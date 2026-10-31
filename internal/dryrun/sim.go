// Package dryrun simulates wolfbet dice locally — a drop-in BetPlacer
// replacement for paper trading. Rolls are uniform over the 10,000
// two-decimal values (0.00–99.99) like the casino's result_value; payout
// is 99/chance so the simulator carries the same ~1% house edge.
package dryrun

import (
	"context"
	"math"
	"math/rand"

	"wdb-go/internal/engine"
)

// Sim is a seeded dice simulator. Seed fixed = deterministic runs.
type Sim struct {
	rng     *rand.Rand
	balance float64
	bets    int
}

// New creates a simulator tracking a paper balance from startBalance
// (read from the real API, per the spec: no POST bet in dry-run).
func New(seed int64, startBalance float64) *Sim {
	return &Sim{
		rng:     rand.New(rand.NewSource(seed)),
		balance: startBalance,
	}
}

// Balance returns the current paper balance.
func (s *Sim) Balance() float64 { return s.balance }

// Bets returns the number of simulated bets.
func (s *Sim) Bets() int { return s.bets }

// round2 matches the wolfbet result_value granularity.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// PlaceBet simulates one dice bet with the same math as the live
// adapter: chance round2, target = bethigh ? 99.99-chance : chance,
// multiplier 99/chance, win = bethigh ? roll > target : roll < target.
func (s *Sim) PlaceBet(_ context.Context, req engine.BetRequest) (engine.BetOutcome, error) {
	chance := round2(req.Chance)
	if chance <= 0 || chance >= 100 {
		chance = 49.5
	}
	target := chance
	if req.Bethigh {
		target = round2(99.99 - chance)
	}
	multiplier := math.Round((99/chance)*10000) / 10000

	roll := float64(s.rng.Intn(10000)) / 100.0
	won := roll < target
	if req.Bethigh {
		won = roll > target
	}

	profit := -req.Amount
	if won {
		profit = req.Amount * (multiplier - 1)
	}

	s.balance += profit
	s.bets++

	return engine.BetOutcome{
		Roll:               roll,
		Profit:             profit,
		Won:                won,
		Target:             target,
		Nonce:              int64(s.bets),
		ID:                 "dry",
		Amount:             req.Amount,
		RateLimitRemaining: 120, // unlimited in dry-run
	}, nil
}
