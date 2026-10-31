package lua

import (
	"os"
	"testing"

	"wdb-go/internal/state"
)

// The original martingale template, copied verbatim (byte-identical,
// verified by diff) from the reference repo. Goal #1 of the spec: this
// script runs without editing a single character.
const scriptPath = "testdata/dice-lua.txt"

func loadVerbatimScript(t *testing.T, b *Bridge) {
	t.Helper()
	raw, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", scriptPath, err)
	}
	if err := b.LoadUserScript(string(raw)); err != nil {
		t.Fatalf("verbatim script failed to load: %v", err)
	}
}

func contractState() *state.BotState {
	st := state.CreateInitialState()
	st.Balance = 100
	st.StartBalance = 100
	st.Currency = "trx"
	return st
}

func pow2(n int) float64 {
	out := 1.0
	for i := 0; i < n; i++ {
		out *= 2
	}
	return out
}

func nearlyEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	scale := b
	if scale < 1 {
		scale = 1
	}
	return d/scale < 1e-9
}

// TestContractDiceLuaVerbatim runs the real template for 100 iterations
// following the original per-bet order (engine index.ts):
//  1. top-level assignments collected right after load — bet 1 uses the
//     script's top-level nextbet, dobet is NOT called yet
//  2. for bet N (N>1): dobet() runs after bet N-1 lands, then
//     previousbet = nextbet and lastBet.amount = nextbet are snapshotted
//     BEFORE the bet is placed
//
// Martingale expectations: nextbet doubles on a loss, resets to basebet
// on a win.
func TestContractDiceLuaVerbatim(t *testing.T) {
	const basebet = 0.00000001

	st := contractState()
	b := New(st, Callbacks{}, "wolfbet.com")
	t.Cleanup(b.Close)
	loadVerbatimScript(t, b)

	// step 1: top-level nextbet/chance/bethigh land in state before bet 1
	ret := b.CollectReturns()
	ApplyReturns(st, ret)
	if st.Nextbet != basebet {
		t.Fatalf("top-level nextbet = %v, want %v", st.Nextbet, basebet)
	}
	if st.Chance != 49.5 {
		t.Fatalf("top-level chance = %v, want 49.5", st.Chance)
	}
	if !st.Bethigh {
		t.Fatal("top-level bethigh = false, want true")
	}

	firstBetPlaced := false
	expectedNextbet := basebet

	for i := 0; i < 100; i++ {
		if firstBetPlaced {
			if err := b.RunDobet(); err != nil {
				t.Fatalf("dobet for bet %d: %v", i, err)
			}
			ret := b.CollectReturns()
			ApplyReturns(st, ret)
			if !nearlyEqual(st.Nextbet, expectedNextbet) {
				t.Fatalf("bet %d: nextbet = %v, want %v", i, st.Nextbet, expectedNextbet)
			}
		}

		// snapshot before placing (original sendBet port, index.ts:694)
		st.Previousbet = st.Nextbet
		st.LastBet.Amount = st.Nextbet
		firstBetPlaced = true
		betAmount := st.Nextbet

		// 9 losses then a win, repeating
		won := i%10 == 9
		wonValue := 0.0
		roll := 5.0
		if won {
			wonValue = 2 * betAmount
			roll = 95.0
		}
		state.ApplyBetResult(st, state.BetRoll{
			Roll:       roll,
			RollResult: won,
			WonValue:   wonValue,
			BetValue:   betAmount,
			ID:         i,
		})

		if won {
			expectedNextbet = basebet
		} else {
			expectedNextbet = betAmount * 2
		}
	}

	if st.Bets != 100 {
		t.Errorf("bets = %d, want 100", st.Bets)
	}
	if st.Wins != 10 {
		t.Errorf("wins = %d, want 10", st.Wins)
	}
	if st.Losses != 90 {
		t.Errorf("losses = %d, want 90", st.Losses)
	}
	// 10 cycles of 1+2+...+512 = 1023 * basebet wagered per 9-loss run is
	// lost, each win pays back 2*512*basebet... just check wagered matches
	// the sum of all placed bets exactly.
	wantWagered := 0.0
	for cycle := 0; cycle < 10; cycle++ {
		bet := basebet
		for j := 0; j < 9; j++ {
			wantWagered += bet
			bet *= 2
		}
		wantWagered += bet // the winning bet (512*basebet)
	}
	if !nearlyEqual(st.Wagered, wantWagered) {
		t.Errorf("wagered = %v, want %v", st.Wagered, wantWagered)
	}
}

// TestContractScriptNameVerbatim: the script's --scriptname comment sets
// nothing (it is a comment); GetScriptName returns "".
func TestContractScriptNameVerbatim(t *testing.T) {
	b := New(contractState(), Callbacks{}, "wolfbet.com")
	t.Cleanup(b.Close)
	loadVerbatimScript(t, b)
	if got := b.GetScriptName(); got != "" {
		t.Errorf("scriptname = %q, want empty (it is a comment in the template)", got)
	}
}
