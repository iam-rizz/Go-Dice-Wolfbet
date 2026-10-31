# Plan — Bot Betting Go: Wolfbet Dice

**Spec**: .planning/specs/wolfbet-dice-bot.md
**Epic**: none
**Created**: 2026-10-31
**Status**: draft

---

## Stack

Go CLI (bukan kotlin/nextjs — repo baru). Go 1.22+ (toolchain di-install ke `/usr/local/go` sebagai task 0), module `wdb-go`, dependencies: `github.com/yuin/gopher-lua`, `github.com/charmbracelet/bubbletea` + `lipgloss`. Tidak ada database. Kontrak referensi di-port dari `/root/wedebe/apps/bot/src/engine/` (bukan di-copy — bedahasa), adapter dari `/root/wedebe/apps/bot/bots_src/stable/wolf-dice.js`.

## Architecture

```
main.go (CLI flags/config)
   │
   ▼
engine.Loop ──► lua.Bridge ──► gopher-lua VM (script user)
   │  ▲            │ inject state / collect returns
   │  └────────────┘
   ▼
wolfbet.Client (HTTP: auth, balances, bet/place, seed/refresh)
   │
   └─ dryrun.Dice (implements interface BetPlacer — swap untuk dry-run)
state.BotState (shared: di-mutate loop, dibaca bridge & TUI)
tui.App (bubbletea — baca state + log ring, kirim commands)
```

Layering: `cmd` → `engine` → (`lua`, `wolfbet`/`dryrun`, `state`) → `tui` hanya render. `wolfbet.Client` dan `dryrun.Dice` meng-implement interface `BetPlacer` yang sama — dry-run = drop-in replacement tanpa menyentuh loop.

### Components

| Component | Tipe | Purpose |
|---|---|---|
| `config` | package | Flags + TOML + env (token, user_hash, currency, script, dry-run, stop conditions) |
| `state` | package | `BotState` (40 var + lastBet dual-case) + `ApplyBetResult` + `ExposeToLua` — port dari `state.ts` |
| `lua` | package | Bridge gopher-lua: kompat chain, inject state, `dobet()`, `CollectReturns`, 16 bridge funcs, sleep/clock/getn |
| `wolfbet` | package | HTTP client: header auth, 4 endpoint, payload math dice, mapping error (429/400/422/one-bet/401) |
| `dryrun` | package | Simulator dice seeded (implements `BetPlacer`) |
| `engine` | package | Loop betting: PlayBet→SendBet→HandleResult→HandleStats + backoff + stop + SIGINT |
| `tui` | package | Model bubbletea: statistik 2 kolom + log ring + keys |
| `cmd/wdbgo` | main | Wiring: config → client/dryrun → engine → TUI |

### File Locations

| File | Location | Purpose |
|---|---|---|
| `go.mod`, `go.sum` | root | Module + deps |
| `main.go` | `cmd/wdbgo/` | Entry, flag parsing, wiring |
| `config.go` + `config_test.go` | `internal/config/` | Load flags/TOML/env, validasi |
| `state.go` + `state_test.go` | `internal/state/` | BotState + ApplyBetResult + ExposeToLua |
| `compat.go` | `internal/lua/` | applyLuaCompatReplacements (`!=`→`~=`, dll) |
| `bridge.go` + `bridge_test.go` | `internal/lua/` | VM, inject, dobet, collectReturns, primitives |
| `bridgefuncs.go` + `bridgefuncs_test.go` | `internal/lua/` | 16 bridge function + sleep/clock/getn/casino |
| `contract_test.go` | `internal/lua/` | Script asli verbatim (dice-lua.txt) 100 iterasi |
| `placer.go` | `internal/engine/` | Interface `BetPlacer` + tipe BetRequest/BetResult |
| `client.go` + `client_test.go` | `internal/wolfbet/` | HTTP client + auth + balances/currencies/seed |
| `dice.go` + `dice_test.go` | `internal/wolfbet/` | Payload math + parse result + error mapping |
| `sim.go` + `sim_test.go` | `internal/dryrun/` | RNG seeded + payout dice |
| `loop.go` + `loop_test.go` | `internal/engine/` | Loop + backoff + stop conditions |
| `app.go` + `app_test.go` | `internal/tui/` | Model bubbletea + log ring |
| `README.md` | root (ubah) | Cara pakai + cara ambil token/user_hash |

### Files to Change

| File | What Changes | Why |
|---|---|---|
| `README.md` | Tambah usage, config, smoke manual | Deliverable harus bisa dipakai |

## Task Breakdown

### Phase 0: Environment

| # | Task | Files |
|---|---|---|
| 0 | Install Go toolchain (`/usr/local/go`, PATH), `go mod init wdb-go`, deps `go get` | `go.mod`, `go.sum` |

### Phase 1: Foundation — state math (TDD)

| # | Task | Files |
|---|---|---|
| 1 | Tulis `state_test.go` dulu (port ekspektasi `state.test.ts`: streak, min/max, dual-case lastBet, wagered), lalu implementasi `state.go` sampai hijau | `internal/state/{state.go,state_test.go}` |
| 2 | `ExposeToLua` — generate deklarasi Lua literal (scalars + lastBet nested) + test snapshot vs format asli | `internal/state/{state.go,state_test.go}` |

### Phase 2: Lua bridge (TDD, tergantung Phase 1)

| # | Task | Files |
|---|---|---|
| 3 | Kompat chain: test verbatim regex asli (`!=`→`~=`, `!`→`not`, `+=` dll) lalu implement `compat.go` | `internal/lua/{compat.go,compat_test.go}` |
| 4 | Bridge core: `New(state, callbacks)`, inject state → run `dobet()` → `CollectReturns` (koersi string numerik) + test | `internal/lua/{bridge.go,bridge_test.go}` |
| 5 | Bridge functions: 16 fungsi via callbacks Go + primitives (`sleep` asli, `clock`, `table.getn`, `casino`, `scriptname`) + test nama fungsi ter-inject | `internal/lua/{bridgefuncs.go,bridgefuncs_test.go}` |
| 6 | Kontrak verbatim: load `dice-lua.txt` asli, 100 iterasi dobet dengan mock state — tanpa error, nilai nextbet benar | `internal/lua/contract_test.go` |

### Phase 3: Wolbet client (TDD — PARALEL dengan Phase 2)

| # | Task | Files |
|---|---|---|
| 7 | Client HTTP: header auth, GET balances/currencies/seed-refresh, parse `userBalances`/`settings` + httptest | `internal/wolfbet/{client.go,client_test.go}` |
| 8 | Bet dice: payload math (target 99.99, multiplier 4 desimal, amount string 8 desimal), parse `bet.{profit,result_value,nonce,hash}`, mapping error lengkap (429→ErrRateLimited, 400→ErrInsufficient, 422→field, "one bet at a time"→ErrBusy, 401→ErrAuth) + httptest semua cabang | `internal/wolfbet/{dice.go,dice_test.go}` |

### Phase 4: Engine + dry-run (tergantung Phase 2 & 3)

| # | Task | Files |
|---|---|---|
| 9 | Interface `BetPlacer` + loop: PlayBet (baca var dari VM) → SendBet → HandleResult (state math) → HandleStats → iterasi; stop-on-win/loss/max-bets; error mapping → backoff 3s; SIGINT rapi + test loop dengan mock placer | `internal/engine/{placer.go,loop.go,loop_test.go}` |
| 10 | Dry-run: sim dice seeded (roll uniform 0–99.99, payout 99/chance) implements `BetPlacer` + test distribusi 10k bet (win-rate ±1% dari house edge) | `internal/dryrun/{sim.go,sim_test.go}` |

### Phase 5: TUI + wiring (tergantung Phase 4)

| # | Task | Files |
|---|---|---|
| 11 | TUI: model bubbletea 2 kolom (statistik/log ring 200), keys `q`/`s`, semua state (running/stopped/error) + unit test model | `internal/tui/{app.go,app_test.go}` |
| 12 | `main.go` wiring (config → wolfbet/dryrun → engine → TUI) + README (usage, cara ekstrak token/user_hash) + `go vet`/`gofmt` bersih + build binary | `cmd/wdbgo/main.go`, `README.md` |

### Parallel vs Sequential

| Parallel Group | Tasks | Why |
|---|---|---|
| A (setelah task 0-1) | 3-6 (lua) dan 7-8 (wolfbet) | package terpisah, tidak saling import |

| Sequential | Depends On | Why |
|---|---|---|
| 1, 2 | 0 | butuh go.mod |
| 3-6 | 1, 2 | bridge butuh state + ExposeToLua |
| 9 | 3-8 | loop butuh bridge + placer |
| 10 | 9 | implements interface BetPlacer |
| 11, 12 | 9 | TUI/main butuh loop & state |

## Testing Plan

| Test | Jenis | Trace ke spec |
|---|---|---|
| `state_test.go` | Unit | Goal #2 (statistik benar), Req #7 (state math identik) |
| `compat_test.go` | Unit | Req #1 (kompat chain verbatim) |
| `bridge_test.go`, `bridgefuncs_test.go` | Unit | Req #1 (40 var, 16 funcs, koersi) |
| `contract_test.go` | Unit/e2e-local | Goal #1 (dice-lua.txt verbatim), edge #6-8 |
| `client_test.go`, `dice_test.go` | Integration (httptest) | Req #2, edge #1-5 |
| `loop_test.go` | Unit (mock placer) | Req #3 (backoff, stop), edge #1, #2, #10 |
| `sim_test.go` | Unit statistik | Goal #4 (10k bet, house edge ±1%), Req #4 |
| `app_test.go` | Unit model | Req #5 (TUI state), UI Changes |
| Manual smoke live | Manual | Goal #2 (20 bet nyata) — dicatat ke WORKLOG `wdb-go` |

Semua test dijalankan via `go test ./...`; gate build: `gofmt -l .` kosong + `go vet ./...` bersih + `go build ./...` sukses.

## Risk Notes

- **gopher-lua = Lua 5.1** vs fengari 5.3 — kontrak test (task 6) jadi penjaga; kalau script kompleks gagal, opsi: transformasi source atau VM lain.
- **Auth manual**: token/user_hash bisa kedaluwarsa — error 401 harus memberi instruksi refresh (edge #5).
- **Live smoke** memakai saldo nyata — wajib bet minimum + dicatat.
