package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wdb-go/internal/state"
	"wdb-go/internal/wolfbet"
)

// mockScript records when dobet runs and mutates nextbet like a
// martingale would.
type mockScript struct {
	calls     int
	lastState *state.BotState
}

func (m *mockScript) RunDobet() error {
	m.calls++
	if m.lastState.Win {
		m.lastState.Nextbet = 0.00000001
	} else {
		m.lastState.Nextbet = m.lastState.Previousbet * 2
	}
	return nil
}

// mockPlacer returns canned outcomes / errors in order.
type mockPlacer struct {
	outcomes []BetOutcome
	errs     []error
	n        int
	requests []BetRequest
}

func (m *mockPlacer) PlaceBet(_ context.Context, req BetRequest) (BetOutcome, error) {
	m.requests = append(m.requests, req)
	i := m.n
	m.n++
	if i < len(m.errs) && m.errs[i] != nil {
		return BetOutcome{}, m.errs[i]
	}
	if i < len(m.outcomes) {
		return m.outcomes[i], nil
	}
	// default: alternating win/loss at 2x
	won := i%2 == 0
	profit := -req.Amount
	if won {
		profit = req.Amount
	}
	return BetOutcome{
		Roll: 50, Profit: profit, Won: won, ID: "bet", Amount: req.Amount,
		Nonce: int64(i + 1), RateLimitRemaining: 100 - i,
	}, nil
}

func newTestLoop(p BetPlacer, s Script, opts Options) (*Loop, *recorder) {
	rec := &recorder{}
	st := state.CreateInitialState()
	st.Balance = 100
	st.StartBalance = 100
	st.Nextbet = 1
	st.Currency = "trx"
	if ms, ok := s.(*mockScript); ok {
		ms.lastState = st
	}
	return &Loop{
		State:     st,
		Placer:    p,
		Script:    s,
		Callbacks: rec.callbacks(),
		Options:   opts,
	}, rec
}

type recorder struct {
	logs   []string
	stats  int
	stops  []string
	errors []error
}

func (r *recorder) callbacks() LoopCallbacks {
	return LoopCallbacks{
		OnLog:   func(msg string) { r.logs = append(r.logs, msg) },
		OnStats: func(*state.BotState) { r.stats++ },
		OnStop:  func(reason string) { r.stops = append(r.stops, reason) },
		OnError: func(err error) { r.errors = append(r.errors, err) },
	}
}

func TestLoopHappyPath(t *testing.T) {
	script := &mockScript{}
	loop, rec := newTestLoop(&mockPlacer{}, script, Options{MaxBets: 10, Backoff: time.Millisecond})
	loop.Run(context.Background())

	if loop.State.Bets == 0 {
		t.Fatal("no bets placed")
	}
	// dobet runs once per bet AFTER the first: calls = bets - 1
	if script.calls != loop.State.Bets-1 {
		t.Errorf("script calls = %d, bets = %d (want calls = bets-1)", script.calls, loop.State.Bets)
	}
	if rec.stats != loop.State.Bets {
		t.Errorf("onStats calls = %d, want %d", rec.stats, loop.State.Bets)
	}
	if len(rec.stops) != 1 {
		t.Fatalf("stops = %v, want exactly one", rec.stops)
	}
}

func TestLoopFirstBetSkipsDobet(t *testing.T) {
	script := &mockScript{}
	loop, _ := newTestLoop(&mockPlacer{outcomes: make([]BetOutcome, 1)}, script, Options{MaxBets: 1, Backoff: time.Millisecond})
	// single outcome at index 0
	loop.Run(context.Background())
	if script.calls != 0 {
		t.Errorf("dobet ran %d times before first bet, want 0", script.calls)
	}
	if loop.State.Bets != 1 {
		t.Errorf("bets = %d, want 1", loop.State.Bets)
	}
}

func TestLoopSnapshotBeforeBet(t *testing.T) {
	p := &mockPlacer{}
	loop, _ := newTestLoop(p, &mockScript{}, Options{MaxBets: 1, Backoff: time.Millisecond})
	loop.Run(context.Background())
	if p.requests[0].Amount != 1 {
		t.Errorf("first bet amount = %v, want 1 (top-level nextbet)", p.requests[0].Amount)
	}
	if loop.State.Previousbet != 1 {
		t.Errorf("previousbet = %v, want 1 (snapshot before place)", loop.State.Previousbet)
	}
}

func TestLoopStateMathWinLose(t *testing.T) {
	// forced win at 2x: profit = +amount
	p := &mockPlacer{outcomes: []BetOutcome{
		{Roll: 10, Profit: 1, Won: true, Amount: 1, ID: "a", Nonce: 1},
		{Roll: 90, Profit: -1, Won: false, Amount: 1, ID: "b", Nonce: 2},
	}}
	loop, _ := newTestLoop(p, nil, Options{MaxBets: 2, Backoff: time.Millisecond})
	loop.Run(context.Background())

	st := loop.State
	if st.Bets != 2 || st.Wins != 1 || st.Losses != 1 {
		t.Errorf("bets/wins/losses = %d/%d/%d, want 2/1/1", st.Bets, st.Wins, st.Losses)
	}
	if st.Currentprofit != -1 {
		t.Errorf("currentprofit = %v, want -1", st.Currentprofit)
	}
	if st.Profit != 0 {
		t.Errorf("profit = %v, want 0", st.Profit)
	}
	if st.Balance != 100 {
		t.Errorf("balance = %v, want 100", st.Balance)
	}
	if st.LastBet.ID != "b" {
		t.Errorf("lastBet.id = %v, want b", st.LastBet.ID)
	}
}

func TestLoopBusyRetriesImmediatelySamePayload(t *testing.T) {
	p := &mockPlacer{
		errs: []error{wolfbet.ErrBusy},
	}
	loop, _ := newTestLoop(p, nil, Options{MaxBets: 1, Backoff: time.Hour})
	start := time.Now()
	loop.Run(context.Background())

	if loop.State.Bets != 1 {
		t.Fatalf("bets = %d, want 1 (busy retry counts once)", loop.State.Bets)
	}
	if len(p.requests) != 2 {
		t.Fatalf("requests = %d, want 2 (one retry)", len(p.requests))
	}
	if p.requests[0].Amount != p.requests[1].Amount {
		t.Errorf("retry sent different payload: %v vs %v", p.requests[0], p.requests[1])
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("busy retry waited %v, want immediate", elapsed)
	}
}

func TestLoopRateLimitBacksOffThenRetriesSamePayload(t *testing.T) {
	p := &mockPlacer{
		errs: []error{wolfbet.ErrRateLimited},
	}
	loop, rec := newTestLoop(p, nil, Options{MaxBets: 1, Backoff: 50 * time.Millisecond})
	start := time.Now()
	loop.Run(context.Background())

	if loop.State.Bets != 1 {
		t.Fatalf("bets = %d, want 1", loop.State.Bets)
	}
	if len(p.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(p.requests))
	}
	if p.requests[0].Amount != p.requests[1].Amount {
		t.Error("retry after 429 must resend the same payload (no double bet)")
	}
	if elapsed := time.Since(start); elapsed < 45*time.Millisecond {
		t.Errorf("backoff elapsed %v, want >= 50ms", elapsed)
	}
	if len(rec.logs) == 0 || !strings.Contains(rec.logs[0], "3s") {
		t.Errorf("logs = %v, want rate limit message", rec.logs)
	}
}

func TestLoopNetworkErrorBacksOff(t *testing.T) {
	p := &mockPlacer{
		errs: []error{wolfbet.ErrNetwork},
	}
	loop, _ := newTestLoop(p, nil, Options{MaxBets: 1, Backoff: time.Millisecond})
	loop.Run(context.Background())
	if loop.State.Bets != 1 {
		t.Errorf("bets = %d, want 1 (network error retries)", loop.State.Bets)
	}
}

func TestLoopFatalErrorStops(t *testing.T) {
	p := &mockPlacer{
		errs: []error{wolfbet.ErrInsufficient},
	}
	loop, rec := newTestLoop(p, nil, Options{Backoff: time.Millisecond})
	loop.Run(context.Background())

	if loop.State.Bets != 0 {
		t.Errorf("bets = %d, want 0", loop.State.Bets)
	}
	if len(rec.errors) != 1 {
		t.Fatalf("errors = %v, want one", rec.errors)
	}
	if !errors.Is(rec.errors[0], wolfbet.ErrInsufficient) {
		t.Errorf("error = %v, want ErrInsufficient", rec.errors[0])
	}
	if len(rec.stops) == 0 || rec.stops[0] != "error" {
		t.Errorf("stops = %v, want [error]", rec.stops)
	}
}

func TestLoopValidationErrorStops(t *testing.T) {
	p := &mockPlacer{
		errs: []error{&wolfbet.ValidationError{Fields: map[string]string{"amount": "The amount field is required."}}},
	}
	loop, rec := newTestLoop(p, nil, Options{Backoff: time.Millisecond})
	loop.Run(context.Background())
	if loop.State.Bets != 0 {
		t.Errorf("bets = %d, want 0", loop.State.Bets)
	}
	found := false
	for _, msg := range rec.logs {
		if strings.Contains(msg, "amount") {
			found = true
		}
	}
	if !found {
		t.Errorf("validation field error not logged: %v", rec.logs)
	}
}

func TestLoopScriptRuntimeErrorStops(t *testing.T) {
	script := errScript{}
	loop, rec := newTestLoop(&mockPlacer{}, script, Options{Backoff: time.Millisecond})
	loop.Run(context.Background())
	if loop.State.Bets != 1 {
		t.Fatalf("bets = %d, want 1 (first bet has no dobet, error hits bet 2)", loop.State.Bets)
	}
	if len(rec.stops) == 0 || rec.stops[0] != "error" {
		t.Errorf("stops = %v, want [error]", rec.stops)
	}
}

type errScript struct{}

func (errScript) RunDobet() error { return errors.New("attempt to index a nil value") }

func TestLoopStopOnWin(t *testing.T) {
	p := &mockPlacer{outcomes: []BetOutcome{
		{Roll: 10, Profit: 1, Won: true, Amount: 1, ID: "a", Nonce: 1},
	}}
	loop, rec := newTestLoop(p, nil, Options{StopOnWin: true, Backoff: time.Millisecond})
	loop.Run(context.Background())
	if loop.State.Bets != 1 {
		t.Errorf("bets = %d, want 1", loop.State.Bets)
	}
	if rec.stops[0] != "stop-on-win" {
		t.Errorf("stop reason = %q, want stop-on-win", rec.stops[0])
	}
}

func TestLoopStopOnLoss(t *testing.T) {
	p := &mockPlacer{outcomes: []BetOutcome{
		{Roll: 90, Profit: -1, Won: false, Amount: 1, ID: "a", Nonce: 1},
	}}
	loop, rec := newTestLoop(p, nil, Options{StopOnLoss: true, Backoff: time.Millisecond})
	loop.Run(context.Background())
	if loop.State.Bets != 1 {
		t.Errorf("bets = %d, want 1", loop.State.Bets)
	}
	if rec.stops[0] != "stop-on-loss" {
		t.Errorf("stop reason = %q, want stop-on-loss", rec.stops[0])
	}
}

func TestLoopMaxBets(t *testing.T) {
	loop, rec := newTestLoop(&mockPlacer{}, nil, Options{MaxBets: 7, Backoff: time.Millisecond})
	loop.Run(context.Background())
	if loop.State.Bets != 7 {
		t.Errorf("bets = %d, want 7", loop.State.Bets)
	}
	if rec.stops[0] != "max-bets" {
		t.Errorf("stop reason = %q, want max-bets", rec.stops[0])
	}
}

func TestLoopContextCancelStopsCleanly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &mockPlacer{}
	loop, _ := newTestLoop(p, nil, Options{Backoff: time.Hour})
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	loop.Run(ctx)
	if time.Since(start) > time.Second {
		t.Error("loop did not stop promptly after cancel")
	}
}

func TestLoopRateLimitExposed(t *testing.T) {
	p := &mockPlacer{outcomes: []BetOutcome{
		{Roll: 10, Profit: 1, Won: true, Amount: 1, ID: "a", Nonce: 1, RateLimitRemaining: 42},
	}}
	loop, _ := newTestLoop(p, nil, Options{StopOnWin: true, Backoff: time.Millisecond})
	loop.Run(context.Background())
	if loop.RateLimit() != 42 {
		t.Errorf("rate limit = %d, want 42", loop.RateLimit())
	}
}

// TestLoopDelayThrottlesBets: with Delay set, N bets must take at least
// (N-1)*Delay (the first bet is not delayed).
func TestLoopDelayThrottlesBets(t *testing.T) {
	const delay = 20 * time.Millisecond
	p := &mockPlacer{}
	loop, _ := newTestLoop(p, nil, Options{MaxBets: 5, Backoff: time.Millisecond, Delay: delay})

	start := time.Now()
	loop.Run(context.Background())
	elapsed := time.Since(start)

	if len(p.requests) != 5 {
		t.Fatalf("placed %d bets, want 5", len(p.requests))
	}
	min := 4 * delay // 5 bets → 4 inter-bet gaps
	if elapsed < min {
		t.Errorf("elapsed %v, want >= %v (delay not applied)", elapsed, min)
	}
}

// TestLoopNoDelayIsFast: with Delay unset the loop runs back-to-back.
func TestLoopNoDelayIsFast(t *testing.T) {
	p := &mockPlacer{}
	loop, _ := newTestLoop(p, nil, Options{MaxBets: 50, Backoff: time.Millisecond})

	start := time.Now()
	loop.Run(context.Background())
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("50 bets without delay took %v, want fast", elapsed)
	}
}
