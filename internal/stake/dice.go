package stake

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// DiceBet is one dice roll request.
type DiceBet struct {
	Amount   float64
	Chance   float64
	Bethigh  bool
	Currency string
}

// DiceResult is the mapped diceRoll response.
type DiceResult struct {
	ID     string
	Nonce  int64
	Amount float64
	Payout float64
	// Profit is payout - amount, like the bundle's handleResult.
	Profit float64
	Roll   float64 // state.result
	Target float64
}

// round2 rounds to 2 decimals — the bundle's Math.round((v+EPS)*100)/100.
func round2(v float64) float64 {
	return math.Round((v+numberEpsilon)*100) / 100
}

const numberEpsilon = 2.220446049250313e-16

// randomString mirrors the bundle's randomString(n): alphanumeric.
const alphanum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphanum[rand.Intn(len(alphanum))]
	}
	return string(b)
}

// PlaceDiceBet runs the DiceRoll mutation. Transient failures (slow down,
// seed rotation, network) are retried here so the engine sees either a
// result or a terminal error — same contract as the wolfbet placer.
func (c *Client) PlaceDiceBet(ctx context.Context, bet DiceBet) (DiceResult, error) {
	chance := round2(bet.Chance)
	target := chance
	if bet.Bethigh {
		target = 100 - chance
	}
	target = round2(target)
	condition := "below"
	if bet.Bethigh {
		condition = "above"
	}

	query := `mutation DiceRoll($amount: Float!, $target: Float!, $condition: CasinoGameDiceConditionEnum!, $currency: CurrencyEnum!, $identifier: String!) {
  diceRoll(amount: $amount target: $target condition: $condition currency: $currency identifier: $identifier) {
    ...CasinoBet
    state {
      ...CasinoGameDice
    }
  }
}

fragment CasinoBet on CasinoBet {
  id
  nonce
  amount
  payout
}

fragment CasinoGameDice on CasinoGameDice {
  result
}`

	attempts := 0
	for {
		if err := ctx.Err(); err != nil {
			return DiceResult{}, err
		}
		attempts++
		if c.MaxRetries > 0 && attempts > c.MaxRetries {
			return DiceResult{}, ErrNetwork
		}

		identifier := randomString(20)
		resp, err := c.post(ctx, graphqlRequest{
			OperationName: "DiceRoll",
			Variables: map[string]any{
				"amount":     bet.Amount,
				"currency":   strings.ToLower(bet.Currency),
				"identifier": identifier,
				"condition":  condition,
				"target":     target,
			},
			Query: query,
		})
		if err != nil {
			// ErrNetwork and ErrSlowDown are transient — retry.
			if errors.Is(err, ErrNetwork) {
				continue
			}
			return DiceResult{}, err
		}

		if len(resp.Errors) > 0 {
			msg := resp.Errors[0].Message
			switch msg {
			case "Please slow down":
				select {
				case <-ctx.Done():
					return DiceResult{}, ctx.Err()
				case <-time.After(time.Second):
				}
				continue
			case "an unknown error 294779 has happened.", "Please choose a new seed pair":
				if err := c.RotateSeedPair(ctx, randomString(10)); err != nil {
					return DiceResult{}, err
				}
				continue
			default:
				return DiceResult{}, &APIError{Message: msg}
			}
		}

		var payload struct {
			DiceRoll struct {
				ID     json.Number `json:"id"`
				Nonce  int64       `json:"nonce"`
				Amount float64     `json:"amount"`
				Payout float64     `json:"payout"`
				State  struct {
					Result float64 `json:"result"`
				} `json:"state"`
			} `json:"diceRoll"`
		}
		if err := json.Unmarshal(resp.Data, &payload); err != nil {
			return DiceResult{}, err
		}
		roll := payload.DiceRoll
		id, _ := roll.ID.Int64()
		return DiceResult{
			ID:     strconv.FormatInt(id, 10),
			Nonce:  roll.Nonce,
			Amount: roll.Amount,
			Payout: roll.Payout,
			Profit: roll.Payout - roll.Amount,
			Roll:   roll.State.Result,
			Target: target,
		}, nil
	}
}
