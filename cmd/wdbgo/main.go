// wdbgo — headless wolfbet dice bot with Lua user scripts.
//
// Wiring: config → wolfbet client / dry-run sim → lua bridge → engine
// loop → bubbletea TUI. The loop runs in a goroutine and pushes stats /
// log messages into the TUI; user keys (q, s) flow back through
// callbacks.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"

	"wdb-go/internal/config"
	"wdb-go/internal/dryrun"
	"wdb-go/internal/engine"
	"wdb-go/internal/lua"
	"wdb-go/internal/state"
	"wdb-go/internal/tui"
	"wdb-go/internal/wolfbet"
)

// dryRunPaperBalance is used when dry-run runs without credentials
// (balance/min-max cannot be read from the API then).
const dryRunPaperBalance = 1000

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "wdbgo:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		return err
	}
	rawScript, err := os.ReadFile(cfg.Script)
	if err != nil {
		return fmt.Errorf("script file: %w", err)
	}

	// ctrl-c and SIGTERM cancel the loop cleanly (no bet flies after)
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	st := state.CreateInitialState()
	st.Currency = strings.ToLower(cfg.Currency)
	st.Coin = strings.ToUpper(cfg.Currency)

	// ---- placer: live wolfbet or dry-run sim ----
	var client *wolfbet.Client
	var placer engine.BetPlacer
	var startBalance float64 = dryRunPaperBalance

	hasCreds := cfg.Token != "" && cfg.UserHash != ""
	if hasCreds {
		client = wolfbet.New(cfg.BaseURL, cfg.Token, cfg.UserHash)
		// Fetch the real wallet balance in live mode too — the TUI and the
		// script's top-level reads must show the actual currency balance,
		// not the paper default. Fall back to paper balance only if the call
		// fails.
		bal, err := fetchBalance(ctx, client, st.Currency)
		if err == nil {
			startBalance = bal
		} else {
			fmt.Fprintf(os.Stderr, "wdbgo: could not read %s balance (%v), using paper balance %v\n",
				st.Currency, err, dryRunPaperBalance)
		}
	}
	if cfg.DryRun {
		placer = dryrun.New(cfg.Seed, startBalance)
	} else {
		placer = &wolfPlacer{client: client}
	}

	st.Balance = startBalance
	st.StartBalance = startBalance
	st.Minbalance = startBalance
	st.Maxbalance = startBalance
	if hasCreds {
		if settings, err := client.Currencies(ctx); err == nil {
			if set, ok := settings[st.Currency]; ok {
				st.Minbet = set.AmountMinimum.Float()
				st.Maxbet = set.AmountMaximum.Float()
			}
		}
	}

	// ---- TUI model (callbacks wired before the program starts) ----
	var loop *engine.Loop
	mode := "live"
	if cfg.DryRun {
		mode = "dry-run"
	}

	model := tui.New("", mode, st.Currency, cfg.StopOnWin)
	model.OnQuit = func() { stopSignals() }
	model.OnStopOnWinToggle = func(v bool) {
		if loop != nil {
			loop.Options.StopOnWin = v
		}
	}
	program := tea.NewProgram(model)

	// ---- lua bridge with the 16 contract callbacks ----
	bridge := lua.New(st, bridgeCallbacks(st, client, func(msg string) {
		program.Send(tui.LogMsg(msg))
	}, func(reason string) {
		if loop != nil {
			loop.Stop(reason)
		}
	}), "wolfbet.com")
	defer bridge.Close()

	// Inject state BEFORE loading the script so top-level reads of
	// balance/chance/etc. work — the original web bot also runs the script
	// with all Lua globals already set (bridge.go only injects inside
	// RunDobet otherwise, which is too late for top-level assignment).
	if err := bridge.InjectState(); err != nil {
		return fmt.Errorf("script failed to load: %w", err)
	}
	if err := bridge.LoadUserScript(string(rawScript)); err != nil {
		return fmt.Errorf("script failed to load: %w", err)
	}
	model.ScriptName = bridge.GetScriptName()

	// top-level assignments land in state BEFORE bet 1 (original sequencing)
	lua.ApplyReturns(st, bridge.CollectReturns())

	// ---- pre-start confirmation ----------------------------------------
	// Show what is about to run and ask before betting real money.
	if !cfg.DryRun {
		if err := confirmBeforeStart(st, cfg.Script, cfg.MaxBets, cfg.DelayMs); err != nil {
			return err
		}
	}

	script := &scriptRunner{bridge: bridge, st: st}

	startedAt := time.Now()
	var stopReason string
	loop = &engine.Loop{
		State:  st,
		Placer: placer,
		Script: script,
		Options: engine.Options{
			StopOnWin:  cfg.StopOnWin,
			StopOnLoss: cfg.StopOnLoss,
			MaxBets:    cfg.MaxBets,
			Delay:      time.Duration(cfg.DelayMs) * time.Millisecond,
		},
		Callbacks: engine.LoopCallbacks{
			OnLog: func(msg string) { program.Send(tui.LogMsg(msg)) },
			OnStats: func(s *state.BotState) {
				elapsed := time.Since(startedAt)
				speed := 0.0
				if secs := elapsed.Seconds(); secs > 0 {
					speed = float64(s.Bets) / secs
				}
				program.Send(tui.StatsMsg{
					State:     s,
					RateLimit: loop.RateLimit(),
					Speed:     speed,
					Elapsed:   elapsed,
				})
			},
			OnStop: func(reason string) {
				stopReason = reason
				program.Send(tui.StopMsg{Reason: reason})
			},
			OnError: func(err error) {
				program.Send(tui.ErrorMsg{Err: err.Error()})
			},
		},
	}

	go func() {
		loop.Run(ctx)
		program.Send(tui.QuitMsg{})
	}()

	if _, err := program.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}

	printSummary(st, stopReason)
	return nil
}

// wolfPlacer adapts the wolfbet client to the engine's BetPlacer.
type wolfPlacer struct {
	client *wolfbet.Client
}

func (w *wolfPlacer) PlaceBet(ctx context.Context, req engine.BetRequest) (engine.BetOutcome, error) {
	res, err := w.client.PlaceDiceBet(ctx, wolfbet.DiceBet{
		Amount:   req.Amount,
		Chance:   req.Chance,
		Bethigh:  req.Bethigh,
		Currency: req.Currency,
	})
	if err != nil {
		return engine.BetOutcome{}, err
	}
	return engine.BetOutcome{
		Roll:               res.Roll,
		Profit:             res.Profit,
		Won:                res.Won,
		Target:             res.Target,
		Nonce:              res.Nonce,
		ID:                 res.ID,
		Amount:             res.Amount,
		RateLimitRemaining: res.RateLimitRemaining,
	}, nil
}

// scriptRunner adapts the lua bridge to the engine's Script interface.
type scriptRunner struct {
	bridge *lua.Bridge
	st     *state.BotState
}

func (s *scriptRunner) RunDobet() error {
	if err := s.bridge.RunDobet(); err != nil {
		return err
	}
	lua.ApplyReturns(s.st, s.bridge.CollectReturns())
	return nil
}

// bridgeCallbacks implements the 16 contract functions against the CLI.
// vault/unvault log "Not supported" like the wolf bundle; invest and
// ching are no-ops; reset* reset the state counters.
func bridgeCallbacks(
	st *state.BotState,
	client *wolfbet.Client,
	log func(string),
	stop func(string),
) lua.Callbacks {
	resetStats := func() {
		st.Bets = 0
		st.Wins = 0
		st.Losses = 0
		st.Winstreak = 0
		st.Losestreak = 0
		st.Currentstreak = 0
		st.Maxwinstreak = 0
		st.Maxlosestreak = 0
		st.Profit = 0
		st.Currentprofit = 0
		st.Partialprofit = 0
		st.Wagered = 0
		st.Minprofit = 0
		st.Maxprofit = 0
		st.MaxBetAmount = 0
		st.MaxLosseAmount = 0
	}

	return lua.Callbacks{
		"log": func(args ...any) any {
			if len(args) > 0 {
				if s, ok := args[0].(string); ok {
					log(s)
				}
			}
			return nil
		},
		"stop": func(args ...any) any {
			stop("stop() called")
			return nil
		},
		"checkbalance": func(args ...any) any {
			if client == nil {
				return st.Balance
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			bal, err := fetchBalance(ctx, client, st.Currency)
			if err != nil {
				log("checkbalance failed: " + err.Error())
				return st.Balance
			}
			st.Balance = bal
			return bal
		},
		"resetseed": func(args ...any) any {
			if client == nil {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := client.RefreshSeed(ctx); err != nil {
				log("resetseed failed: " + err.Error())
			}
			return nil
		},
		"resetstats": func(args ...any) any {
			resetStats()
			return nil
		},
		"resetpartialprofit": func(args ...any) any {
			st.Partialprofit = 0
			return nil
		},
		"vault":   func(args ...any) any { log("Not supported"); return nil },
		"unvault": func(args ...any) any { log("Not supported"); return nil },
		// start/resume/resetchart/resethistory/resetlog/resetall/ching/invest:
		// no chart/history to reset in a CLI, no vault to invest into —
		// keep the contract callable without breaking scripts.
		"start":        func(args ...any) any { return nil },
		"resume":       func(args ...any) any { return nil },
		"resetchart":   func(args ...any) any { return nil },
		"resethistory": func(args ...any) any { return nil },
		"resetlog":     func(args ...any) any { return nil },
		"resetall": func(args ...any) any {
			resetStats()
			return nil
		},
		"ching":  func(args ...any) any { return nil },
		"invest": func(args ...any) any { log("Not supported"); return nil },
	}
}

func fetchBalance(ctx context.Context, client *wolfbet.Client, currency string) (float64, error) {
	balances, err := client.Balances(ctx)
	if err != nil {
		return 0, err
	}
	bal, ok := wolfbet.BalanceFor(balances, currency)
	if !ok {
		return 0, fmt.Errorf("no balance entry for %s", currency)
	}
	return bal, nil
}

func printSummary(st *state.BotState, reason string) {
	fmt.Println("──────── session summary ────────")
	fmt.Printf("bets:      %d (wins %d / losses %d)\n", st.Bets, st.Wins, st.Losses)
	fmt.Printf("wagered:   %.8f\n", st.Wagered)
	fmt.Printf("profit:    %.8f\n", st.Profit)
	fmt.Printf("balance:   %.8f → %.8f\n", st.StartBalance, st.Balance)
	fmt.Printf("streaks:   win %d / loss %d (max %d/%d)\n",
		st.Winstreak, st.Losestreak, st.Maxwinstreak, st.Maxlosestreak)
	fmt.Printf("max bet:   %.8f\n", st.MaxBetAmount)
	if reason != "" {
		fmt.Printf("stopped:   %s\n", reason)
	}
	fmt.Println("─────────────────────────────────")
}

// confirmBeforeStart prints what is about to run (script, currency, initial
// balance, first bet) and asks for an explicit yes before placing real bets.
// Returning an error aborts the run. Skipped in dry-run.
func confirmBeforeStart(st *state.BotState, scriptPath string, maxBets, delayMs int) error {
	fmt.Println("──────── konfirmasi sebelum mulai ────────")
	fmt.Printf("mode:     live\n")
	fmt.Printf("script:   %s\n", scriptPath)
	fmt.Printf("currency: %s\n", st.Currency)
	fmt.Printf("balance:  %.8f\n", st.Balance)
	fmt.Printf("chance:   %v\n", st.Chance)
	fmt.Printf("first bet: %.8f\n", st.Nextbet)
	fmt.Printf("bets max:  %d\n", maxBets)
	if delayMs > 0 {
		fmt.Printf("delay:    %d ms antar bet\n", delayMs)
	} else {
		fmt.Printf("delay:    - (back-to-back)\n")
	}
	fmt.Println("script menempel — pastikan kamu sudah menguji dry-run dulu!")
	fmt.Print("Lanjut live betting? [y/N]: ")

	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("dibatalkan pengguna — tidak ada bet yang ditempatkan")
	}
	fmt.Println("──────── mulai ────────")
	return nil
}
