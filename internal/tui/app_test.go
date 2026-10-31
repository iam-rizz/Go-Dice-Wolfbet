package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbletea"

	"wdb-go/internal/state"
)

func keyMsg(s string) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestNewModelDefaults(t *testing.T) {
	m := New("martingale", "dry-run", "trx", false)
	if m.Status != StatusRunning {
		t.Errorf("status = %q, want running", m.Status)
	}
	if m.StopOnWin {
		t.Error("stop-on-win should default off")
	}
	if v := m.View(); !strings.Contains(v, "martingale") {
		t.Errorf("view missing script name:\n%s", v)
	}
}

func TestStatsMsgUpdatesSnapshot(t *testing.T) {
	m := New("s", "live", "trx", false)
	st := state.CreateInitialState()
	st.Balance = 101
	st.Profit = 1
	st.Bets = 10
	st.Wins = 4
	st.Losses = 6
	st.Winstreak = 2
	st.Losestreak = 0
	st.Wagered = 12.5
	st.Nextbet = 0.00000002

	model, _ := m.Update(StatsMsg{State: st, RateLimit: 87, Speed: 12.5})
	m2 := model.(Model)
	if m2.Balance != 101 || m2.Bets != 10 || m2.Wins != 4 || m2.Losses != 6 {
		t.Errorf("stats not applied: %+v", m2)
	}
	if m2.RateLimit != 87 || m2.Speed != 12.5 {
		t.Errorf("rate/speed not applied: %d/%v", m2.RateLimit, m2.Speed)
	}
	if m2.CurrentBet != 0.00000002 {
		t.Errorf("current bet = %v", m2.CurrentBet)
	}
}

func TestStatsMsgInView(t *testing.T) {
	m := New("s", "live", "trx", false)
	st := state.CreateInitialState()
	st.Bets = 42
	st.Balance = 55.5
	model, _ := m.Update(StatsMsg{State: st, RateLimit: 87, Speed: 9.5})
	view := model.(Model).View()
	for _, want := range []string{"42", "55.5", "87", "9.5"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestLogMsgRing(t *testing.T) {
	m := New("s", "live", "trx", false)
	var model tea.Model = m
	for i := 0; i < LogRingSize+50; i++ {
		model, _ = model.Update(LogMsg("line " + strconv.Itoa(i)))
	}
	m2 := model.(Model)
	if len(m2.logs) != LogRingSize {
		t.Fatalf("log ring = %d lines, want %d", len(m2.logs), LogRingSize)
	}
	if m2.logs[0] != "line 50" {
		t.Errorf("oldest retained = %q, want %q (trim to last %d)", m2.logs[0], "line 50", LogRingSize)
	}
	if m2.logs[len(m2.logs)-1] != "line 249" {
		t.Errorf("newest = %q, want %q", m2.logs[len(m2.logs)-1], "line 249")
	}
}

func TestStopAndErrorStates(t *testing.T) {
	m := New("s", "live", "trx", false)
	model, _ := m.Update(StopMsg{Reason: "max-bets"})
	stopped := model.(Model)
	if stopped.Status != StatusStopped || stopped.StopReason != "max-bets" {
		t.Errorf("stop state = %q/%q", stopped.Status, stopped.StopReason)
	}
	if v := stopped.View(); !strings.Contains(v, "stopped") || !strings.Contains(v, "max-bets") {
		t.Errorf("view missing stop reason:\n%s", v)
	}

	model2, _ := m.Update(ErrorMsg{Err: "insufficient balance"})
	errored := model2.(Model)
	if errored.Status != StatusError || errored.Err != "insufficient balance" {
		t.Errorf("error state = %q/%q", errored.Status, errored.Err)
	}
	if v := errored.View(); !strings.Contains(v, "error") || !strings.Contains(v, "insufficient balance") {
		t.Errorf("view missing error:\n%s", v)
	}
}

func TestQuitKey(t *testing.T) {
	quitCalled := false
	m := New("s", "live", "trx", false)
	m.OnQuit = func() { quitCalled = true }
	_, cmd := m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatal("q should produce a quit command")
	}
	if !quitCalled {
		t.Error("OnQuit callback not fired")
	}
}

func TestStopOnWinToggle(t *testing.T) {
	var got bool
	m := New("s", "live", "trx", false)
	m.OnStopOnWinToggle = func(v bool) { got = v }
	model, _ := m.Update(keyMsg("s"))
	m2 := model.(Model)
	if !m2.StopOnWin {
		t.Error("s should turn stop-on-win on")
	}
	if !got {
		t.Error("toggle callback not fired")
	}
	model3, _ := m2.Update(keyMsg("s"))
	if model3.(Model).StopOnWin {
		t.Error("second s should turn stop-on-win off")
	}
}

func TestProfitColoredInView(t *testing.T) {
	m := New("s", "live", "trx", false)
	st := state.CreateInitialState()
	st.Profit = 5
	model, _ := m.Update(StatsMsg{State: st})
	view := model.(Model).View()
	if !strings.Contains(view, "+5") {
		t.Errorf("positive profit should render with +:\n%s", view)
	}
}

func TestFooterShowsKeys(t *testing.T) {
	m := New("s", "dry-run", "doge", true)
	v := m.View()
	for _, want := range []string{"dry-run", "doge", "stop-on-win: on", "[q] quit"} {
		if !strings.Contains(v, want) {
			t.Errorf("footer missing %q:\n%s", want, v)
		}
	}
}
