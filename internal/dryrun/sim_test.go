package dryrun

import (
	"context"
	"testing"

	"wdb-go/internal/engine"
)

func TestSimDeterministic(t *testing.T) {
	a := New(42, 100)
	b := New(42, 100)
	req := engine.BetRequest{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"}
	for i := 0; i < 100; i++ {
		ra, _ := a.PlaceBet(context.Background(), req)
		rb, _ := b.PlaceBet(context.Background(), req)
		if ra.Roll != rb.Roll || ra.Won != rb.Won {
			t.Fatalf("seeded runs diverged at bet %d: %v vs %v", i, ra.Roll, rb.Roll)
		}
	}
}

func TestSimRollRange(t *testing.T) {
	s := New(1, 100)
	req := engine.BetRequest{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"}
	for i := 0; i < 1000; i++ {
		out, _ := s.PlaceBet(context.Background(), req)
		if out.Roll < 0 || out.Roll > 99.99 {
			t.Fatalf("roll out of range: %v", out.Roll)
		}
	}
}

func TestSimPayout(t *testing.T) {
	s := New(7, 100)
	// win at 49.5% pays 2x total → profit = +amount
	req := engine.BetRequest{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"}
	var out engine.BetOutcome
	for i := 0; i < 1000; i++ {
		out, _ = s.PlaceBet(context.Background(), req)
	}
	if out.Won && out.Profit != 1 {
		t.Errorf("win profit = %v, want 1 (amount * (2-1))", out.Profit)
	}
	if !out.Won && out.Profit != -1 {
		t.Errorf("loss profit = %v, want -1", out.Profit)
	}
}

// Goal #4 of the spec: 10,000 simulated bets produce a win rate within
// ±1% of the true chance (49.5%), i.e. the simulator carries the same
// house edge as the casino.
func TestSimHouseEdgeDistribution(t *testing.T) {
	const n = 10000
	s := New(2026, 1000)
	req := engine.BetRequest{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"}

	wins := 0
	wagered := 0.0
	profit := 0.0
	for i := 0; i < n; i++ {
		out, _ := s.PlaceBet(context.Background(), req)
		wagered += req.Amount
		profit += out.Profit
		if out.Won {
			wins++
		}
	}

	winRate := float64(wins) / n
	if winRate < 0.485 || winRate > 0.505 {
		t.Errorf("win rate = %.4f, want within 0.485–0.505 (49.5%% ±1%%)", winRate)
	}

	// EV = -1% of wagered (house edge). Per-bet std ≈ amount, so over
	// 10k bets the total std is ≈ 1% of wagered — allow ±3σ.
	expected := -0.01 * wagered
	if diff := profit - expected; diff < -0.03*wagered || diff > 0.03*wagered {
		t.Errorf("profit = %.2f, want ≈ %.2f (−1%% house edge ±3σ)", profit, expected)
	}
}

func TestSimBalanceTracking(t *testing.T) {
	s := New(3, 100)
	req := engine.BetRequest{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"}
	before := s.Balance()
	out, _ := s.PlaceBet(context.Background(), req)
	if s.Balance() != before+out.Profit {
		t.Errorf("balance = %v, want %v", s.Balance(), before+out.Profit)
	}
	if s.Bets() != 1 {
		t.Errorf("bets = %d, want 1", s.Bets())
	}
}

func TestSimWinRuleOverUnder(t *testing.T) {
	// bethigh with chance 49.5: target 50.49, win when roll > 50.49
	s := New(11, 100)
	under := engine.BetRequest{Amount: 1, Chance: 49.5, Bethigh: false, Currency: "trx"}
	out, _ := s.PlaceBet(context.Background(), under)
	if out.Target != 49.5 {
		t.Errorf("under target = %v, want 49.5", out.Target)
	}
	// win/roll consistency for under
	if out.Won != (out.Roll < 49.5) {
		t.Errorf("under: won=%v roll=%v inconsistent", out.Won, out.Roll)
	}

	over := engine.BetRequest{Amount: 1, Chance: 49.5, Bethigh: true, Currency: "trx"}
	out2, _ := s.PlaceBet(context.Background(), over)
	if out2.Target != 50.49 {
		t.Errorf("over target = %v, want 50.49", out2.Target)
	}
	if out2.Won != (out2.Roll > 50.49) {
		t.Errorf("over: won=%v roll=%v inconsistent", out2.Won, out2.Roll)
	}
}

func TestSimRateLimitUnlimited(t *testing.T) {
	s := New(5, 100)
	out, _ := s.PlaceBet(context.Background(), engine.BetRequest{Amount: 1, Chance: 49.5, Currency: "trx"})
	if out.RateLimitRemaining != 120 {
		t.Errorf("dry-run rate limit = %d, want 120 (unlimited)", out.RateLimitRemaining)
	}
}
