# Panduan Stake: dapat session & jalankan bot

Stake.com **tidak punya API key**. Bot butuh **cookie `session`** dari
browser Anda setelah login. Panduan ini menjelaskan cara mendapatkannya —
termasuk kenapa login otomatis dari server sering gagal.

---

## Kenapa login otomatis (username/password) sering gagal

Stake memblokir **IP datacenter/VPS** (termasuk server tempat bot ini
berjalan). Cloudflare challenge-nya bisa dilewati browser asli, tapi
lapisan aplikasi stake tetap menolak dengan **Error 403** untuk IP
datacenter — bahkan homepage-nya saja sudah 403.

Sudah diuji dari server ini: Chrome headless, `headless=new`,
anti-detection flags, dan Chrome "headed" di bawah Xvfb — **semua 403**.

Artinya: kalau bot jalan di VPS/datacenter, **login otomatis tidak akan
bekerhasil**. Ambil session manual (Opsi 1 di bawah).

---

## Opsi 1 — Ambil session manual (paling pasti, ~30 detik) ✅ REKOMENDASI

### Langkah 1: Login di browser Anda

Buka https://stake.com dan login seperti biasa (di komputer/VPS Anda,
bukan server yang diblokir).

### Langkah 2: Buka DevTools

Tekan **F12** (atau klik kanan → Inspect).

### Langkah 3: Cari cookie `session`

Pilih tab **Application** (di Firefox: **Storage**) → panel kiri:
**Cookies** → `https://stake.com`.

Cari baris bernama **`session`**. Nilainya panjang (biasanya diawali
tanda `-`, misal `-abcd1234...`).

Klik nilainya, salin seluruhnya (Ctrl+C).

> **Cara cepat lewat Console:** jalankan
> ```js
> document.cookie.split('; ').find(c => c.startsWith('session='))
> ```
> lalu salin bagian setelah `session=`.

### Langkah 4: Simpan ke bot

Di mesin tempat bot berjalan:

```bash
./wdbgo-login --manual
```

Tempel nilai cookie saat diminta, tekan Enter. Bot menyimpannya ke
`~/.wdbgo/session` (permission 0600, aman).

Atau langsung taruh di config:

```bash
# wdbgo.toml.local
session = "nilai-cookie-panjang-ini"
```

### Langkah 5: Jalankan bot

```bash
# dry-run dulu (aman)
./wdbgo --dry-run --casino stake --script <script.lua> --currency trx

# live
./wdbgo --casino stake --script <script.lua> --currency trx --delay 200
```

Bot otomatis membaca `~/.wdbgo/session` kalau `--session` tidak
diberikan.

---

## Opsi 2 — Login otomatis (hanya di IP yang tidak diblokir)

Kalau Anda menjalankan bot di **komputer sendiri** atau VPS ber-IP
residensial:

```bash
./wdbgo-login --setup    # masukkan email & password (disimpan 0600)
./wdbgo-login            # buka Chromium, login otomatis, ambil cookie
```

Kalau akun Anda pakai 2FA, bot akan meminta kode OTP di terminal.

**Catatan:** dari server datacenter, ini akan gagal dengan 403 — pakai
Opsi 1.

---

## Kalau session expired (error 401)

Cookie `session` bisa kedaluwarsa (logout, timeout, atau ganti password).
Gejalanya: bot berhenti dengan pesan `stake: unauthorized`.

Solusi: ulangi **Opsi 1 Langkah 3–4** (ambil cookie baru dari browser).

Cek status session:

```bash
./wdbgo-login --status
```

Hapus session (misal sebelum ganti akun):

```bash
./wdbgo-login --logout
```

---

## Ringkasan perintah

| Perintah | Fungsi |
|---|---|
| `./wdbgo-login --manual` | Tempel cookie session dari browser |
| `./wdbgo-login --setup` | Simpan email & password (untuk Opsi 2) |
| `./wdbgo-login` | Login otomatis via Chromium |
| `./wdbgo-login --status` | Cek session tersimpan |
| `./wdbgo-login --logout` | Hapus session |

---

## Keamanan

- Session disimpan di `~/.wdbgo/session` dengan permission **0600**
  (hanya pemilik file yang bisa baca).
- Password (Opsi 2) disimpan di `~/.wdbgo/credentials`, juga **0600**.
- File ini **tidak pernah** di-commit ke git (`.gitignore`).
- Sesi login browser Anda tidak terpengaruh — bot hanya membaca cookie.
