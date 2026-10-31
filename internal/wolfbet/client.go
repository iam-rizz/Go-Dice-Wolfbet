// Package wolfbet implements the wolfbet.com API contract extracted from
// the reference bundle bots_src/stable/wolf-dice.js.
package wolfbet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Terminal error classes — the engine maps these to retry/stop behavior
// exactly like the original bundle's sendBet catch block:
//   - ErrBusy:         "You can play only one bet at a time." → retry NOW
//   - ErrRateLimited:  429 / empty status → retry after 3s
//   - ErrNetwork:      request-level failure → retry after 3s
//   - ErrInsufficient: 400 → stop
//   - ErrValidation:   422 field errors → stop
//   - ErrAuth:         401/403 → stop + refresh instructions
var (
	ErrBusy         = errors.New("wolfbet: You can play only one bet at a time.")
	ErrRateLimited  = errors.New("wolfbet: rate limited (Wolf blocks fast betting, auto retry after 3s)")
	ErrNetwork      = errors.New("wolfbet: network error, auto retry after 3s")
	ErrInsufficient = errors.New("wolfbet: You have insufficient balance")
	ErrAuth         = errors.New("wolfbet: unauthorized — token/user_hash expired, extract new ones from the browser (localStorage token, window.intercomSettings.user_hash)")
)

// APIError carries a non-mapped API failure (unknown body error, 5xx...).
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wolfbet: api error %d: %s", e.Status, e.Message)
}

// ValidationError is the 422 case: field → message, logged as
// "field: message" per key like the original.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, k+": "+v)
	}
	return "wolfbet: validation failed — " + strings.Join(parts, "; ")
}

// Client talks to wolfbet.com with the browser-auth headers.
type Client struct {
	BaseURL  string // e.g. https://wolfbet.com
	Token    string
	UserHash string
	HTTP     *http.Client
}

// New builds a client. token and userHash come from the browser
// (localStorage "token" JSON .value / window.intercomSettings.user_hash).
func New(baseURL, token, userHash string) *Client {
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Token:    token,
		UserHash: userHash,
		HTTP:     &http.Client{},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return nil, err
	}
	// beforeWork(): axios.defaults.headers
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("X-Client-Type", "Web-Application")
	req.Header.Set("X-Hash-Api", c.UserHash)
	return req, nil
}

// do performs the request and maps the response to the contract errors.
// status 0 / 429 → ErrRateLimited; 400 → ErrInsufficient; 401/403 →
// ErrAuth; 422 → ValidationError; 200 with body.error → ErrBusy or
// APIError. The raw body is returned for endpoint-specific parsing.
func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, http.Header, error) {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, ErrNetwork
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, ErrNetwork
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		var errBody struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &errBody) == nil && errBody.Error != "" {
			if errBody.Error == "You can play only one bet at a time." {
				return data, resp.Header, ErrBusy
			}
			return data, resp.Header, &APIError{Status: resp.StatusCode, Message: errBody.Error}
		}
		return data, resp.Header, nil
	case resp.StatusCode == 429:
		return data, resp.Header, ErrRateLimited
	case resp.StatusCode == 400:
		return data, resp.Header, ErrInsufficient
	case resp.StatusCode == 401 || resp.StatusCode == 403:
		return data, resp.Header, ErrAuth
	case resp.StatusCode == 422:
		fields := map[string]string{}
		var raw map[string]json.RawMessage
		if json.Unmarshal(data, &raw) == nil {
			for k, v := range raw {
				var s string
				if json.Unmarshal(v, &s) == nil {
					fields[k] = s
				} else {
					fields[k] = string(v)
				}
			}
		}
		return data, resp.Header, &ValidationError{Fields: fields}
	default:
		return data, resp.Header, &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}
}

// FlexibleFloat accepts a JSON number or numeric string ("1.23") — the
// bundle wraps every amount in Number(...).
type FlexibleFloat float64

func (f *FlexibleFloat) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = 0
		return nil
	}
	// try string first, then bare number
	var str string
	if err := json.Unmarshal(b, &str); err == nil {
		s = strings.TrimSpace(str)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("wolfbet: %q is not numeric: %w", s, err)
	}
	*f = FlexibleFloat(v)
	return nil
}

func (f FlexibleFloat) Float() float64 { return float64(f) }

// Balance is one entry of GET /api/v2/user/balances → userBalances[].
type Balance struct {
	Currency string        `json:"currency"`
	Amount   FlexibleFloat `json:"amount"`
}

// Balances fetches the coin list + amounts.
func (c *Client) Balances(ctx context.Context) ([]Balance, error) {
	data, _, err := c.do(ctx, http.MethodGet, "/api/v2/user/balances", nil)
	if err != nil {
		return nil, err
	}
	var body struct {
		UserBalances []Balance `json:"userBalances"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, &APIError{Status: 200, Message: "balances: " + err.Error()}
	}
	return body.UserBalances, nil
}

// BalanceFor returns the balance for a coin (case-insensitive, like the
// bundle's `currency == String(coin).toLowerCase()`).
func BalanceFor(balances []Balance, coin string) (float64, bool) {
	lc := strings.ToLower(coin)
	for _, b := range balances {
		if strings.ToLower(b.Currency) == lc {
			return b.Amount.Float(), true
		}
	}
	return 0, false
}

// CasinoSettings is the min/max block of GET /api/v1/site/currencies →
// settings.{CUR}.casino.{amount_minimum, amount_maximum}.
type CasinoSettings struct {
	AmountMinimum FlexibleFloat `json:"amount_minimum"`
	AmountMaximum FlexibleFloat `json:"amount_maximum"`
}

// Currencies fetches per-coin casino limits keyed by lowercase coin code.
func (c *Client) Currencies(ctx context.Context) (map[string]CasinoSettings, error) {
	data, _, err := c.do(ctx, http.MethodGet, "/api/v1/site/currencies", nil)
	if err != nil {
		return nil, err
	}
	var body struct {
		Settings map[string]struct {
			Casino CasinoSettings `json:"casino"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return nil, &APIError{Status: 200, Message: "currencies: " + err.Error()}
	}
	out := make(map[string]CasinoSettings, len(body.Settings))
	for k, v := range body.Settings {
		out[strings.ToLower(k)] = v.Casino
	}
	return out, nil
}

// RefreshSeed resets the server seed (GET /api/v1/game/seed/refresh).
func (c *Client) RefreshSeed(ctx context.Context) error {
	_, _, err := c.do(ctx, http.MethodGet, "/api/v1/game/seed/refresh", nil)
	return err
}
