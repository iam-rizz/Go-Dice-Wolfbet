# wdb-go — headless wolfbet dice bot (Lua scripts, Go runtime)

Bot betting dice untuk [wolfbet.com](https://wolfbet.com) yang jalan di
terminal/VPS — tanpa browser, tanpa Chrome extension. Script Lua user
lama (kontrak 40 state variable + 16 bridge function) jalan **tanpa
modifikasi**.

Spec & plan: `.planning/` (Spartan workflow).

## Install / build

Butuh Go 1.22+:

```bash
go build -o wdbgo ./cmd/wdbgo
```

## Cara pakai

```bash
# live (butuh token + user_hash dari browser):
./wdbgo --script martingale.lua --token "$TOKEN" --user-hash "$HASH" --currency trx

# dry-run (paper trading, tanpa kredensial):
./wdbgo --script martingale.lua --dry-run --currency trx

# dengan kondisi stop:
./wdbgo --script martingale.lua --token ... --user-hash ... --max-bets 1000 --stop-on-win
```

Flag lengkap: `--help`.

### Config file (opsional)

Flag > env > TOML > default. Contoh `wdbgo.toml`:

```toml
token = "..."
user_hash = "..."
currency = "trx"
```

```bash
./wdbgo --script bot.lua --config wdbgo.toml
```

**Jangan commit file ini** (sudah di-ignore: `.env`, `*.toml.local`).

### Mengambil token & user_hash (sekali per login)

1. Login ke wolfbet.com di Chrome, buka DevTools.
2. Token: Console →
   `JSON.parse(localStorage.getItem("token")).value`
3. user_hash: Console →
   `intercomSettings.user_hash`
4. Simpan ke env supaya tidak masuk shell history:
   ```bash
   export WDBGO_TOKEN=... WDBGO_USER_HASH=...
   ```

Kalau muncul error 401, token/user_hash kedaluwarsa — ambil ulang.

## Script Lua

Kontrak sama dengan bot browser: variabel global `chance`, `nextbet`,
`bethigh`, `balance`, `profit`, `winstreak`, `lastBet` (dual-case
`amount`/`Amount`), dll; fungsi `log()`, `stop()`, `checkbalance()`,
`resetseed()`, `sleep()`, `clock`, `table.getn`, `casino`. Entry point:
`function dobet()`. Kompatibilitas syntax lama otomatis: `!=` → `~=`,
`!` → `not`, `+=`/`-=`/`*=`/`/=` diekspansi.

Contoh martingale:

```lua
chance=49.5
bethigh=true
basebet=0.00000001
nextbet=basebet

function dobet()
  if (win) then
    nextbet=basebet
  else
    nextbet=previousbet*2
  end
end
```

## TUI

Dua kolom: statistik sesi (balance, profit, bets, streak, speed,
rate-limit) dan log ring script (200 baris terakhir). Keys: `q` quit,
`s` toggle stop-on-win, ctrl-c quit bersih.

Error handling rate limit: 429 → backoff 3 detik otomatis (payload sama,
tanpa dobel bet); "one bet at a time" → retry langsung; 400/422/401 →
stop dengan pesan jelas.

## Mode dry-run

RNG seeded (default seed 42, flag `--seed`), roll uniform 0.00–99.99,
payout 99/chance — house edge sama dengan kasino (~1%). Saldo awal
dibaca dari API kalau ada kredensial; kalau tidak, paper balance 1000.
Tidak ada POST `/api/v1/bet/place`.

## Testing

```bash
go test ./...        # semua package
go vet ./...
gofmt -l .           # harus kosong
```

Kontrak script dijaga test: `internal/lua/testdata/dice-lua.txt`
(template martingale asli, verbatim) dijalankan 100 iterasi.

## Status

- [x] state math (streak/min-max/wagered) — port `state.ts`
- [x] kompat chain Lua (`!=`, `!`, `+=`...)
- [x] bridge gopher-lua: 16 fungsi + sleep asli + clock + getn
- [x] adapter wolfbet dice (payload + error mapping lengkap)
- [x] engine loop (backoff, stop conditions, SIGINT)
- [x] dry-run simulator (house edge ±)
- [x] TUI bubbletea
- [ ] live smoke test 20 bet di wolfbet (butuh akun: WORKLOG)
- [ ] mode JS (goja) — fase 2
- [ ] game wolfbet lain (limbo, keno, plinko, range-dice) — fase 2
