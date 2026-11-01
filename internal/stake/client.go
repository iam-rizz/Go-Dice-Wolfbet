// Package stake implements the stake.com API contract extracted from the
// reference bundle bots_src/stable/stake-dice.js.
//
// Unlike wolfbet (REST), stake speaks GraphQL: every call is a POST to
// /_api/graphql with {operationName, variables, query}. Auth is the
// browser `session` cookie value sent as the x-access-token header.
package stake

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Terminal error classes — the engine maps these to retry/stop behavior
// exactly like the original bundle's sendBet catch block:
//   - ErrSlowDown:     "Please slow down" → retry after 1s
//   - ErrSeedRotate:   "an unknown error 294779" / "Please choose a new
//     seed pair" → rotate the seed pair, then retry the same bet
//   - ErrNetwork:      request-level failure → retry immediately
//   - ErrInsufficient: 400 → stop
//   - ErrAuth:         401/403 → stop + refresh instructions
var (
	ErrSlowDown     = errors.New("stake: Please slow down")
	ErrSeedRotate   = errors.New("stake: seed pair must be rotated")
	ErrNetwork      = errors.New("stake: network error, auto retry")
	ErrInsufficient = errors.New("stake: insufficient balance")
	ErrAuth         = errors.New("stake: unauthorized — session cookie expired, extract a new one from the browser (document.cookie → session)")
)

// APIError carries a non-mapped API failure (unknown GraphQL error, 5xx).
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("stake: api error %d: %s", e.Status, e.Message)
}

// graphqlRequest is the wire shape of every stake GraphQL call.
type graphqlRequest struct {
	OperationName string         `json:"operationName,omitempty"`
	Variables     map[string]any `json:"variables,omitempty"`
	Query         string         `json:"query"`
}

// graphqlResponse is the envelope: either data or errors.
type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Client talks to stake.com with the browser-auth header.
type Client struct {
	BaseURL string // e.g. https://stake.com
	Session string // the `session` cookie value
	HTTP    *http.Client
	// MaxRetries caps the sendBet retry loop for transient errors
	// (network / slow down / seed rotation). 0 = unlimited.
	MaxRetries int
}

// New builds a client with a sane default timeout.
func New(baseURL, session string) *Client {
	return &Client{
		BaseURL: baseURL,
		Session: session,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// post sends one GraphQL document and decodes the envelope.
func (c *Client) post(ctx context.Context, req graphqlRequest) (*graphqlResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/_api/graphql", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-access-token", c.Session)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, ErrNetwork
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrNetwork
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrAuth
	case http.StatusBadRequest:
		return nil, ErrInsufficient
	}

	var out graphqlResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(data))}
	}
	return &out, nil
}

// Balances returns every wallet balance as currency(lowercase) → amount.
func (c *Client) Balances(ctx context.Context) (map[string]float64, error) {
	resp, err := c.post(ctx, graphqlRequest{
		OperationName: "UserBalances",
		Query: `query UserBalances {
  user {
    balances {
      available {
        amount
        currency
      }
    }
  }
}`,
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Errors) > 0 {
		return nil, &APIError{Message: resp.Errors[0].Message}
	}
	var payload struct {
		User struct {
			Balances []struct {
				Available struct {
					Amount   float64 `json:"amount"`
					Currency string  `json:"currency"`
				} `json:"available"`
			} `json:"balances"`
		} `json:"user"`
	}
	if err := json.Unmarshal(resp.Data, &payload); err != nil {
		return nil, err
	}
	out := make(map[string]float64, len(payload.User.Balances))
	for _, b := range payload.User.Balances {
		out[strings.ToLower(b.Available.Currency)] = b.Available.Amount
	}
	return out, nil
}

// BalanceFor is one entry of Balances (currency lowercase).
func BalanceFor(balances map[string]float64, currency string) (float64, bool) {
	v, ok := balances[strings.ToLower(currency)]
	return v, ok
}

// RotateSeedPair rotates the server seed pair — the bundle's resetseed().
func (c *Client) RotateSeedPair(ctx context.Context, seed string) error {
	resp, err := c.post(ctx, graphqlRequest{
		OperationName: "RotateSeedPair",
		Variables:     map[string]any{"seed": seed},
		Query: `mutation RotateSeedPair($seed: String!) {
  rotateSeedPair(seed: $seed) {
    clientSeed {
      user {
        activeClientSeed {
          seed
        }
      }
    }
  }
}`,
	})
	if err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return &APIError{Message: resp.Errors[0].Message}
	}
	return nil
}
