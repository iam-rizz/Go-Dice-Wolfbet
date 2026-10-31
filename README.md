# wdb-go — headless dice bot (Go) untuk wolfbet.com

Bot dice (siap untuk limbo/keno/plinko di fase 2) yang jalan headless di
terminal/VPS — menjalankan **script Lua** dengan kontrak 40+ state variable +
16 bridge function. Script dice klasik (martingale, soft-martingale, strategi
vault/drawdown) jalan **tanpa modifikasi**.

> Kontrak Lua: `chance`, `nextbet`, `bethigh`, `balance`, `profit`,
> `winstreak`, `lastBet` (dual-case), `log()`, `stop()`, `checkbalance()`,
> `resetseed()`, `vault()`, `resetpartialprofit()`, dll.

---

## Install / Build

Butuh Go 1.22+:

```bash
# langkah cepat (build + cek):
./setup.sh
```

Atau manual:

```bash
go build -o wdbgo ./cmd/wdbgo
```

---

## Cara pakai

```bash
# live (memakai config):
./wdbgo --config wdbgo.toml.local --script scripts/example-martingale.lua --currency trx

# dry-run (paper trading, tanpa kredensial):
./wdbgo --dry-run --script scripts/example-martingale.lua --currency trx

# dengan kondisi stop:
./wdbgo --config wdbgo.toml.local --script scripts/example-martingale.lua --max-bets 1000 --stop-on-win
```

### Config file (`wdbgo.toml.local`)

Salin template lalu isi:

```bash
cp wdbgo.toml.example wdbgo.toml.local
```

```toml
token = "..."          # dari DevTools (lihat bawah)
user_hash = "..."      # dari DevTools (lihat bawah)
currency = "trx"       # coin: free, trx, doge, dll
```

Precedence nilai: **flag > env > TOML > default**.
Jangan commit `wdbgo.toml.local` (sudah di `.gitignore`).

### Mengambil token & user_hash (sekali per login)

1. Login ke wolfbet.com di Chrome, buka DevTools (F12 → Console).
2. Token:
   ```js
   JSON.parse(localStorage.getItem("token")).value
   ```
3. user_hash:
   ```js
   intercomSettings.user_hash
   ```
4. Isi ke `wdbgo.toml.local`.

Kalau dapat 401 — token kedaluwarsa, ambil ulang.

---

## CLI lengkap

```
wdbgo [flags]

  --config string      path TOML config (token, user_hash, currency)
  --token string       bearer token      (env WDBGO_TOKEN)
  --user-hash string   x-hash-api value  (env WDBGO_USER_HASH)
  --currency string    coin: free/trx/doge/dll (default "trx")
  --script string      path ke script .lua (wajib)
  --dry-run            simulasi tanpa bet sungguhan
  --seed int           RNG seed dry-run (default 42)
  --stop-on-win        berhenti setelah menang berikutnya
  --stop-on-loss       berhenti setelah kalah berikutnya
  --max-bets int       berhenti setelah N bet (0 = tanpa batas)
  --base-url string    base URL (default https://wolfbet.com)
```

### `--seed`

Hanya berlaku untuk **dry-run**: seed yang sama → urutan roll sama → hasil
bisa diulang (reproduksibel untuk tes strategi). **Tidak berpengaruh di mode
live** — bet sungguhan memakai roll server wolfbet.

```bash
./wdbgo --dry-run --seed 7 --script scripts/example-martingale.lua   # deterministik
```

---

## Script Lua

**Kontrak sama dengan bot browser**:

- Global: `chance`, `nextbet`, `bethigh`, `balance`, `profit`, `winstreak`,
  `previousbet`, `currentprofit`, `lastBet` (`amount`/`Amount`), `currency`,
  `startBalance`, `minbalance`, `maxbalance`, `wagered`, `target`, dll.
- Fungsi: `log()`, `stop()`, `checkbalance()`, `resetseed()`, `resetall()`,
  `resetstats()`, `resetpartialprofit()`, `vault()`, `unvault()`,
  `resetchart()`, `resethistory()`, `resetlog()`, `start()`, `resume()`,
  `ching()`, `invest()`.
- Entry point: `function dobet()` dipanggil setelah tiap bet (bet 1 memakai
  nilai top-level `nextbet`).
- Kompatibilitas syntax lama otomatis: `!=` → `~=`, `!` → `not`,
  `+=`/`-=`/`*=`/`/=` diekspansi.

Script yang membaca `balance` di **top-level** (contoh: `base = balance/BASEDIV`)
didukung — state di-inject sebelum script di-load.

Contoh `scripts/example-martingale.lua`:

```lua
chance     = 49.5
bethigh    = true
basebet    = 0.01
nextbet    = basebet

function dobet()
  if win then
    nextbet = basebet
  else
    nextbet = previousbet * 2
  end
end
```

---

## TUI (tampilan)

Dua kolom:

```
╭──────────────────────╮╭─────────────────────╮
│ xWDice — nama script ││ · log() output      │
│ mode / currency      ││ · VAULT: ...        │
│ balance/profit/wager ││ · STOP: ...         │
│ chance/bets/winrate  ││                     │
│ streak / max strk    ││                     │
│ current / speed      ││                     │
╰──────────────────────╯╰─────────────────────╯
```

- Kolom kiri = statistik sesi; kolom kanan = log ring 200 baris.
- `q` quit · `s` toggle stop-on-win · `ctrl-c` berhenti bersih.
- Setelah berhenti, **ringkasan sesi** dicetak (bets, profit, balance
  awal→akhir, max streak, alasan stop).

**Butuh TTY** — di VPS tanpa layar penuh pakai `tmux`:
```bash
tmux new -s bot
./wdbgo --config wdbgo.toml.local --script scripts/example-martingale.lua
```

### Error handling

- `429` terbatasi → backoff 3 detik otomatis (payload sama).
- `"one bet at a time"` → retry langsung.
- `400`/`422`/`401` → stop dengan pesan jelas.
- Coin `free` didukung (string diteruskan apa adanya).

---

## Mode dry-run

- RNG seeded (default `42`, flag `--seed`), roll uniform 0.00–99.99,
  payout `99/chance` — house edge ~1%.
- Balance awal dibaca dari API kalau ada kredensial; kalau tidak, paper
  balance 1000.
- Tidak ada POST `/api/v1/bet/place` — aman.

---

## Testing

```bash
go test ./...        # semua package
go vet ./...
gofmt -l .           # harus kosong
```

Kontrak script dijaga test: `internal/lua/testdata/dice-lua.txt` (template
martingale asli, verbatim) dijalankan 100 iterasi; plus
`internal/lua/testdata/top-level-state.lua` untuk script yang baca state di
top-level.

---

## Setup & update otomatis

```bash
./setup.sh          # build binary
./setup.sh dry      # build + verifikasi dry-run 5 bet
./setup.sh config   # build + buat wdbgo.toml.local dari template

./update.sh         # git pull + test + rebuild (butuh remote origin)
./update.sh --quiet # pull + build saja (skip test)
```

`setup.sh` mengecek Go 1.22+, build, dan (opsional) dry-run.
`update.sh` menarik perubahan remote, menjalankan test, lalu membangun ulang
binary — cocok untuk mengikuti update setelah repo jadi private.

---

## Struktur

```
cmd/wdbgo/        entrypoint (wiring config → bridge → engine → TUI)
internal/state/   40+ state variable + ApplyBetResult (win/loss math)
internal/lua/     bridge gopher-lua (16 fungsi) + compatibility chain
internal/wolfbet/ client HTTP wolfbet (balances, seed, place bet)
internal/engine/  loop + BetPlacer abstraction (live / dry-run)
internal/dryrun/  seeded simulator implementasi BetPlacer
internal/config/  flags + env + TOML
internal/tui/     dashboard bubbletea
scripts/          contoh script Lua user
```

---

## Status

- [x] state math (streak/min-max/wagered) — port `state.ts`
- [x] kompat chain Lua (`!=`, `!`, `+=`...)
- [x] bridge gopher-lua: 16 fungsi + sleep asli + clock + getn
- [x] adapter wolfbet dice (payload + error mapping lengkap)
- [x] engine loop (backoff, stop conditions, SIGINT)
- [x] dry-run simulator (house edge ±)
- [x] TUI bubbletea — panel xWDice (chance, max streak, winrate, speed)
- [x] setup.sh & update.sh
- [ ] live smoke test 20 bet di wolfbet (butuh akun)
- [ ] mode JS (goja) — fase 2
- [ ] game wolfbet lain (limbo, keno, plinko, range-dice) — fase 2
