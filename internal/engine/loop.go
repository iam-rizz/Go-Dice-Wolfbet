package engine

import (
	"context"
	"errors"
	"time"

	"wdb-go/internal/state"
	"wdb-go/internal/wolfbet"
)

// DefaultBackoff is the 3-second retry the wolf bundle uses after a 429
// ("Wolf blocks fast betting, auto retry after 3s") and network errors.
const DefaultBackoff = 3 * time.Second

// LoopCallbacks surface loop events to the TUI/log without the loop
// knowing about them.
type LoopCallbacks struct {
	OnLog   func(msg string)
	OnStats func(s *state.BotState)
	OnStop  func(reason string)
	OnError func(err error)
}

// Options are the CLI stop conditions.
type Options struct {
	StopOnWin  bool
	StopOnLoss bool
	MaxBets    int // 0 = unlimited
	// Backoff after rate-limit/network errors; overridable in tests.
	Backoff time.Duration
	// Delay is a fixed pause between bets to stay under the casino rate
	// limit (0 = back-to-back, the wolf bundle default). Retries after a
	// 429 use Backoff instead of Delay.
	Delay time.Duration
}

// Loop drives the betting iteration: dobet → snapshot → place →
// handleResult. No fixed interval — back-to-back like the wolf bundle,
// bounded by the casino's rate limiter.
type Loop struct {
	State     *state.BotState
	Placer    BetPlacer
	Script    Script
	Callbacks LoopCallbacks
	Options   Options

	run             bool
	firstBetPlaced  bool
	needRetry       bool
	lastRateLimit   int
	betsPlacedCount int
}

// Run loops until a stop condition, error, or context cancellation.
// Returns the final state.
func (l *Loop) Run(ctx context.Context) *state.BotState {
	l.run = true
	l.State.Run = true
	if l.Options.Backoff == 0 {
		l.Options.Backoff = DefaultBackoff
	}

	for l.run {
		select {
		case <-ctx.Done():
			l.stop("interrupted")
			return l.State
		default:
		}

		if !l.needRetry {
			// dobet for bet N runs after bet N-1 lands — bet 1 uses the
			// top-level nextbet (engine index.ts sequencing)
			if l.firstBetPlaced && l.Script != nil {
				if err := l.Script.RunDobet(); err != nil {
					l.stopErr("script error: "+err.Error(), err)
					return l.State
				}
			}
			// snapshot BEFORE placing (original sendBet port): wagered,
			// previousbet and session maxima track the bet actually made
			l.State.Previousbet = l.State.Nextbet
			l.State.LastBet.Amount = l.State.Nextbet
			l.firstBetPlaced = true

			// throttle: pause between bets to stay under the casino rate
			// limit (skipped on retries, which use Backoff instead)
			if l.Options.Delay > 0 {
				if !l.sleepDelay(ctx) {
					return l.State
				}
			}
		}

		outcome, err := l.Placer.PlaceBet(ctx, BetRequest{
			Amount:   l.State.LastBet.Amount,
			Chance:   l.State.Chance,
			Bethigh:  l.State.Bethigh,
			Currency: l.State.Currency,
		})
		if err != nil {
			switch {
			case errors.Is(err, wolfbet.ErrBusy):
				// one bet at a time: resend immediately, same payload
				l.needRetry = true
				continue
			case errors.Is(err, wolfbet.ErrRateLimited):
				l.needRetry = true
				l.log("Wolf blocks fast betting, auto retry after 3s")
				if !l.sleepBackoff(ctx) {
					return l.State
				}
				continue
			case errors.Is(err, wolfbet.ErrNetwork):
				l.needRetry = true
				l.log("Network error, auto retry after 3s")
				if !l.sleepBackoff(ctx) {
					return l.State
				}
				continue
			default:
				l.stopErr(err.Error(), err)
				return l.State
			}
		}
		l.needRetry = false
		l.lastRateLimit = outcome.RateLimitRemaining
		l.betsPlacedCount++

		// map wolfbet profit onto the state layer's (won-bet)*rate shape
		state.ApplyBetResult(l.State, state.BetRoll{
			Roll:       outcome.Roll,
			RollResult: outcome.Won,
			WonValue:   outcome.Amount + outcome.Profit,
			BetValue:   outcome.Amount,
			ID:         outcome.ID,
			Nonce:      noncePtr(outcome.Nonce),
		})

		if l.Callbacks.OnStats != nil {
			l.Callbacks.OnStats(l.State)
		}

		if l.Options.StopOnWin && l.State.Win {
			l.stop("stop-on-win")
			return l.State
		}
		if l.Options.StopOnLoss && !l.State.Win {
			l.stop("stop-on-loss")
			return l.State
		}
		if l.Options.MaxBets > 0 && l.State.Bets >= l.Options.MaxBets {
			l.stop("max-bets")
			return l.State
		}
	}
	return l.State
}

// RateLimit exposes the last seen x-ratelimit-remaining for the TUI.
func (l *Loop) RateLimit() int {
	return l.lastRateLimit
}

// Stop halts the loop (Ctrl-C / 'q' / stop() from a script callback).
func (l *Loop) Stop(reason string) {
	l.stop(reason)
}

func (l *Loop) stop(reason string) {
	if !l.run {
		return
	}
	l.run = false
	l.State.Run = false
	if l.Callbacks.OnStop != nil {
		l.Callbacks.OnStop(reason)
	}
}

func (l *Loop) stopErr(msg string, err error) {
	l.log(msg)
	if l.Callbacks.OnError != nil {
		l.Callbacks.OnError(err)
	}
	l.stop("error")
}

func (l *Loop) log(msg string) {
	if l.Callbacks.OnLog != nil {
		l.Callbacks.OnLog(msg)
	}
}

// sleepBackoff waits the backoff duration; false means the context was
// cancelled (no bet flies after that).
func (l *Loop) sleepBackoff(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		l.stop("interrupted")
		return false
	case <-time.After(l.Options.Backoff):
		return true
	}
}

// sleepDelay waits the inter-bet throttle duration; false means the context
// was cancelled.
func (l *Loop) sleepDelay(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		l.stop("interrupted")
		return false
	case <-time.After(l.Options.Delay):
		return true
	}
}

func noncePtr(n int64) *int64 {
	return &n
}
