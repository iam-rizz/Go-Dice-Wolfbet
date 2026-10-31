# Spec — Bot Betting Go: Wolfbet Dice

**Created**: 2026-10-31
**Status**: draft
**Author**: owner (pemilik platform XWdice)
**Epic**: none (jalur eksperimen engine Go, terpisah dari platform)

---

## Problem

Bot betting sekarang hanya bisa jalan **di dalam browser** lewat extension (bundle JS di-serve dari `apps/bot`). Untuk menguji strategi dan mengevaluasi Go sebagai jalur engine masa depan, dibutuhkan bot **headless terminal** yang tidak bergantung pada Chrome: ringan, single binary, jalan di VPS. Target uji pertama: **wolfbet.com, game dice**. Syarat keras: script user Lua lama harus jalan **tanpa modifikasi** (kontrak 40 state variable + 16 bridge function dari sistem asli).

## Goal

CLI Go yang membaca file `.lua` user, menjalankan kontrak script asli di `gopher-lua`, dan memasang bet dice nyata (atau simulasi dry-run) di wolfbet.com, dengan statistik live di terminal (TUI).

**Definisi sukses (terukur):**
1. Script martingale asli (`apps/bot/src/engine/templates-data/dice-lua.txt` di repo referensi) jalan **verbatim** — tanpa edit satu karakter.
2. Satu sesi bet nyata lulus di wolfbet dengan bet minimum: 20 bet berturut-turut tanpa error, statistik (bets/wins/profit/streak) cocok dengan perhitungan manual.
3. Rate limit 429 dari wolfbet ditangani otomatis (backoff 3 detik) — bot tidak crash, tidak dobel-bet.
4. Mode dry-run memproduksi distribusi win yang wajar (≈ house edge dice wolfbet) untuk 10.000 bet.

## User Stories

> **As a** pemilik platform, **I want** menjalankan bot dice headless di terminal/VPS dengan script Lua yang sama persis seperti bot browser, **so that** saya bisa uji strategi dan menilai jalur engine Go tanpa membuka browser dan tanpa mengubah ekosistem script user.

> **As a** pemilik platform, **I want** mode dry-run paper-trading, **so that** saya bisa menguji script baru tanpa mempertaruhkan saldo nyata.

## Requirements

### Must-have (MVP)

1. **Engine Lua (gopher-lua)** dengan kontrak asli:
   - 40+ state variable (`chance`, `nextbet`, `bethigh`, `balance`, `profit`, `winstreak`, `lastBet` dual-case `amount`/`Amount`, …) — daftar lengkap di `apps/bot/src/engine/state.ts` (repo referensi `/root/wedebe`).
   - 16 bridge function: `start`, `stop`, `resume`, `checkbalance`, `resetstats`, `resetseed`, `resetchart`, `resethistory`, `resetlog`, `resetall`, `ching`, `log`, `invest` (no-op), `vault`/`unvault` (log "Not supported"), `resetpartialprofit`.
   - Rantai kompatibilitas sebelum load script: `!=`→`~=`, `!`→`not `, ekspansi `+=`/`-=`/`*=`/`/=` — regex verbatim dari `lua-bridge.ts:94`.
   - Sebelum tiap `dobet()`: inject seluruh state sebagai global Lua; setelahnya: `collectReturns` (baca balik `nextbet`, `chance`, `bethigh`, `betSide`, koersi string numerik).
   - `sleep(n)` = sleep **asli** (bukan busy-wait — bundle wolf aslinya async Promise sleep); `clock = os.clock`; `table.getn` shim; global `scriptname`, `casino`.
2. **Adapter wolfbet dice**:
   - Auth: header `authorization: Bearer <token>`, `x-requested-with: XMLHttpRequest`, `x-client-type: Web-Application`, `x-hash-api: <user_hash>`.
   - GET `/api/v2/user/balances` (coin + saldo), GET `/api/v1/site/currencies` (min/max bet), GET `/api/v1/game/seed/refresh` (resetseed), POST `/api/v1/bet/place`.
   - Payload dice: `target = bethigh ? 99.99 - chance : chance` (2 desimal), `multiplier = 99/chance` (4 desimal), `{amount: toFixed(8) STRING, auto: 1, bet_value: target, currency: lowercase, game: "dice", multiplier, rule: over|under}`.
   - Win: `bethigh ? roll > target : roll < target` dari `bet.result_value`.
   - Error handling asli: body error `"You can play only one bet at a time."` → retry langsung; 429/undefined status → log + retry 3s; 400 → stop (saldo kurang); 422 → field errors + stop; network error → retry 3s. Header `x-ratelimit-remaining` ditampilkan di TUI.
3. **Loop betting**: tanpa interval tetap (back-to-back seperti aslinya), dengan backoff rate-limit. `stopOnWin`/`stopOnLoss` dari argumen CLI.
4. **Dry-run mode**: RNG simulasi roll (uniform 0–99.99, seeded, deterministik untuk test) + payout dice wolfbet; saldo/min-max tetap dibaca dari API nyata; tidak ada POST bet.
5. **TUI statistik** (bubbletea/lipgloss): balance, profit, bets, wins/losses, streak, current bet, speed (bets/sec), rate-limit remaining, log ring (pesan `log()` dari script).
6. **CLI & config**: flags + file config (TOML): `--token`, `--user-hash`, `--currency`, `--script <file.lua>`, `--dry-run`, `--stop-on-win`, `--stop-on-loss`, `--max-bets`. Token/user_hash juga boleh dari env (`WDBGO_TOKEN`, `WDBGO_USER_HASH`) — **tidak pernah** di-commit.
7. **State math** di-port dari `state.ts` `applyBetResult` (streak, min/max, wagered, percent) — harus identik perilaku.

### Nice-to-have (fase berikutnya, BUKAN MVP)

- Mode JS script via `goja` (kontrak `with(scope)` dari `js-bridge.ts`).
- Adapter game wolfbet lain: limbo, keno, plinko, range-dice (bundle referensi `bots_src/stable/wolf-*.js`).
- Export history JSON (seperti `exportLog()` asli).
- Multi-account paralel.

### Out of scope

- Validasi license / device binding ke platform XWdice (bot ini standalone pribadi; integrasi platform dibahas ulang kalau jadi produk).
- Panel web, extension, marketplace, obfuscation.
- Kasino lain di MVP (hanya wolfbet dice).
- Mode JS script di MVP.

## Data Model

Tanpa database. State in-memory per sesi (`BotState` port dari `state.ts`). Artefak di disk:
- Config TOML (opsional) — token, user_hash, currency default.
- `session-<timestamp>.json` (opsional, nice-to-have): history bet untuk analisis pasca-run.

## API Changes

Tidak ada API yang dibuat. Bot adalah **client** dari API wolfbet (kontrak terdokumentasi di Requirements §2). Tidak menyentuh api/bot XWdice.

## UI Changes (TUI)

Satu layar terminal, dua kolom:
- Kiri: statistik sesi (balance, profit + warna, bets, wins/losses, streak, wagered, speed, rate-limit).
- Kanan: log ring script (`log()` dan pesan sistem, 200 baris terakhir).
- Footer: nama script, mode (live/dry-run), currency, `q` berhenti, `s` stop-on-win toggle.
- Semua state TUI: default / running / stopped / error (pesan + recovery).

## Edge Cases (minimum yang harus ditangani)

1. **429 rate limit** → backoff 3 detik, tidak crash, tidak mengirim bet ganda saat menunggu.
2. **"You can play only one bet at a time."** → retry langsung (bet sebelumnya belum selesai).
3. **400 insufficient balance** → stop bersih + pesan jelas.
4. **422 validation** (nextbet < minbet, chance di luar rentang) → stop + field error ditampilkan.
5. **401/403 token expired atau user_hash salah** → stop + instruksi cara mengambil token/user_hash baru.
6. **Script Lua tanpa `function dobet()`** → error saat load, exit non-zero.
7. **Runtime error di dalam `dobet()`** → stop loop ( perilaku asli), pesan error Lua ditampilkan apa adanya.
8. **Script set `chance` sebagai string** (`chance='49.5'`) → koersi numerik seperti aslinya.
9. **Dry-run**: hasil deterministik (seed tetap) untuk test; `sleep()` di dry-run tetap benar-benar tidur.
10. **Ctrl-C / SIGINT** → stop rapi: tidak ada bet terbang setelah sinyal, statistik akhir dicetak sebelum keluar.

## Testing Criteria

- **Unit — state math**: port ekspektasi dari `state.test.ts` (streak, min/max, dual-case lastBet, wagered).
- **Unit — Lua bridge**: kompat chain (`!=`, `!`, `+=`), collectReturns + koersi string, 16 nama bridge ter-inject, `sleep`, `table.getn`.
- **Unit — kontrak script**: `dice-lua.txt` asli di-load dan 100 iterasi dobet berjalan di mock state.
- **Integration — adapter**: `httptest` server mock wolfbet: happy path, 429→backoff (asersi timing), one-bet-at-a-time retry, 400 stop, 422 stop, 401 stop.
- **Dry-run**: 10.000 bet simulasi, win-rate dalam toleransi house edge (±1%).
- **Manual smoke (live)**: 20 bet minimum di wolfbet, saldo & statistik dicek manual — dicatat di WORKLOG repo ini.
- Go 1.22+; `go test ./...` hijau; `gofmt`/`go vet` bersih.

## Dependencies

- Go 1.22+, module `github.com/…/wdb-go` (nama module final menyusul).
- `github.com/yuin/gopher-lua` (Lua 5.1 — catatan drift vs fengari 5.3: script sederhana aman; dicatat sebagai risiko).
- `github.com/charmbracelet/bubbletea` + `lipgloss` (TUI).
- Akun wolfbet + `token` dan `user_hash` diekstrak manual dari browser (localStorage `token`, `window.intercomSettings.user_hash`).
- Repo referensi `/root/wedebe`: `apps/bot/src/engine/` (kontrak state/bridge), `apps/bot/bots_src/stable/wolf-dice.js` (kontrak adapter), `apps/bot/src/engine/templates-data/dice-lua.txt` (script uji verbatim).
