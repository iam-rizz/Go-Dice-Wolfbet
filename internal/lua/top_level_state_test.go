package lua

import (
	"os"
	"strings"
	"testing"

	"wdb-go/internal/state"
)

const topLevelScriptPath = "testdata/top-level-state.lua"

func topLevelScript(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(topLevelScriptPath)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	return string(raw)
}

// TestTopLevelStateRequiresPreLoadInjection is the regression anchor for
// the "1/5jam ls 37" style scripts: they read `balance` at top level.
// Before the InjectState-before-load fix, loading such a script failed
// with "cannot perform div operation" — the CLI only injected state inside
// RunDobet, after bet 1.
//
// It asserts the OLD (broken) sequence stays broken at the bridge level,
// so the caller MUST inject state before load (as cmd/wdbgo/main.go now
// does).
func TestTopLevelStateRequiresPreLoadInjection(t *testing.T) {
	st := contractState()
	b := New(st, Callbacks{}, "wolfbet.com")
	t.Cleanup(b.Close)

	err := b.LoadUserScript(topLevelScript(t))
	if err == nil {
		t.Fatal("expected load to fail without pre-load state injection")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("expected a nil-vs-number load error, got: %v", err)
	}
}

// TestTopLevelReadsState is the fixed sequence (cmd/wdbgo/main.go):
// InjectState → LoadUserScript → CollectReturns → RunDobet. The script's
// top-level `base = math.max(balance / basediv, MINBET)` must resolve, and
// the soft-martingale progression must be deterministic (1.25^4 after four
// losses).
func TestTopLevelReadsState(t *testing.T) {
	raw := topLevelScript(t)

	st := contractState() // balance = 100, startBalance = 100, currency = trx
	b := New(st, Callbacks{}, "wolfbet.com")
	t.Cleanup(b.Close)

	if err := b.InjectState(); err != nil {
		t.Fatalf("inject state: %v", err)
	}
	if err := b.LoadUserScript(raw); err != nil {
		t.Fatalf("script failed to load: %v", err)
	}

	ret := b.CollectReturns()
	ApplyReturns(st, ret)
	base := st.Balance / (5 / 0.000015)
	if !nearlyEqual(st.Nextbet, base) {
		t.Fatalf("top-level nextbet = %v, want %v", st.Nextbet, base)
	}
	if st.Chance != 22 {
		t.Fatalf("top-level chance = %v, want 22", st.Chance)
	}

	firstBetPlaced := false
	for i := 0; i < 5; i++ {
		if firstBetPlaced {
			if err := b.RunDobet(); err != nil {
				t.Fatalf("dobet for bet %d: %v", i, err)
			}
			ApplyReturns(st, b.CollectReturns())
		}

		st.Previousbet = st.Nextbet
		st.LastBet.Amount = st.Nextbet
		firstBetPlaced = true

		state.ApplyBetResult(st, state.BetRoll{
			Roll:       5,
			RollResult: false,
			WonValue:   0,
			BetValue:   st.LastBet.Amount,
			ID:         i,
		})
	}

	expected := base * 1.25 * 1.25 * 1.25 * 1.25
	if !nearlyEqual(st.Nextbet, expected) {
		t.Fatalf("after 4 losses nextbet = %v, want %v", st.Nextbet, expected)
	}
}
