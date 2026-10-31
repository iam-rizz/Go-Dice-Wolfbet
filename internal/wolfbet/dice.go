package wolfbet

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
)

// JS uses Math.round((x + Number.EPSILON) * 100) / 100 for the chance and
// target rounding — the epsilon nudge breaks ties away from zero.
const jsEpsilon = 2.220446049250313e-16

func round2(v float64) float64 {
	return math.Round((v+jsEpsilon)*100) / 100
}

func round4(v float64) float64 {
	return math.Round((v+jsEpsilon)*10000) / 10000
}

// fix8 mirrors JS Number(x).toFixed(8) — a STRING with exactly 8 decimals.
func fix8(v float64) string {
	return strconv.FormatFloat(v, 'f', 8, 64)
}

// dicePayload is the POST /api/v1/bet/place body, field-for-field the
// bundle's JSON.stringify object.
type dicePayload struct {
	Amount     string  `json:"amount"` // toFixed(8) STRING — the API requires it
	Auto       int     `json:"auto"`
	BetValue   float64 `json:"bet_value"`
	Currency   string  `json:"currency"`
	Game       string  `json:"game"`
	Multiplier float64 `json:"multiplier"`
	Rule       string  `json:"rule"`
}

// DiceBet is the engine-side bet request.
type DiceBet struct {
	Amount   float64 // nextbet
	Chance   float64
	Bethigh  bool
	Currency string
}

// DiceResult is the mapped bet response.
type DiceResult struct {
	Roll               float64 // bet.result_value
	Profit             float64 // bet.profit (coin currency, no USD conversion)
	Won                bool    // bethigh ? roll > target : roll < target
	Target             float64
	Nonce              int64  // bet.nonce
	ID                 string // bet.hash
	Amount             float64
	RateLimitRemaining int // x-ratelimit-remaining header
}

// BuildDicePayload computes the exact wire payload for a dice bet.
func BuildDicePayload(b DiceBet) dicePayload {
	chance := round2(b.Chance)
	target := chance
	if b.Bethigh {
		target = 99.99 - chance
	}
	target = round2(target)
	return dicePayload{
		Amount:     fix8(b.Amount),
		Auto:       1,
		BetValue:   target,
		Currency:   lower(b.Currency),
		Game:       "dice",
		Multiplier: round4(99 / chance),
		Rule:       map[bool]string{true: "over", false: "under"}[b.Bethigh],
	}
}

func lower(s string) string {
	out := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out[i] = r
	}
	return string(out)
}

// betResponse is the POST /api/v1/bet/place success body: {bet: {...}}.
type betResponse struct {
	Bet struct {
		Profit      FlexibleFloat `json:"profit"`
		ResultValue FlexibleFloat `json:"result_value"`
		Nonce       FlexibleFloat `json:"nonce"`
		Hash        string        `json:"hash"`
	} `json:"bet"`
}

// PlaceDiceBet sends one dice bet and maps the result. Error semantics
// follow the original sendBet: ErrBusy retries immediately, ErrRateLimited
// and ErrNetwork retry after 3s, everything else stops the loop.
func (c *Client) PlaceDiceBet(ctx context.Context, bet DiceBet) (DiceResult, error) {
	payload := BuildDicePayload(bet)
	data, headers, err := c.do(ctx, http.MethodPost, "/api/v1/bet/place", payload)
	if err != nil {
		return DiceResult{}, err
	}

	var body betResponse
	if err := json.Unmarshal(data, &body); err != nil {
		return DiceResult{}, &APIError{Status: 200, Message: "bet/place: " + err.Error()}
	}

	roll := body.Bet.ResultValue.Float()
	// win rule (bundle handleStats, dice branch):
	// bethigh ? result > target : result < target
	won := roll < payload.BetValue
	if bet.Bethigh {
		won = roll > payload.BetValue
	}

	remaining := 120 // bundle default when the header is absent
	if v := headers.Get("x-ratelimit-remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			remaining = n
		}
	}

	return DiceResult{
		Roll:               roll,
		Profit:             body.Bet.Profit.Float(),
		Won:                won,
		Target:             payload.BetValue,
		Nonce:              int64(body.Bet.Nonce.Float()),
		ID:                 body.Bet.Hash,
		Amount:             bet.Amount,
		RateLimitRemaining: remaining,
	}, nil
}
