#!/usr/bin/env bash
#
# wdb-go — one-shot setup script
# Builds the binary, creates the config template and verifies dry-run.
#
# Usage:
#   ./setup.sh                # build binary
#   ./setup.sh dry            # build + run a 5-bet dry-run check
#   ./setup.sh config         # build + generate wdbgo.toml.local from template
#
set -euo pipefail

cd "$(dirname "$0")"

step() { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }
info() { printf '   \033[90m%s\033[0m\n' "$*"; }
die()  { printf '\033[1;31m!! %s\033[0m\n' "$*" >&2; exit 1; }

# --- check toolchain -----------------------------------------------------
step "Cek toolchain"
if ! command -v go >/dev/null 2>&1; then
  die "Go belum terinstal. Install dulu: https://go.dev/dl/ (minimal Go 1.22)"
fi
GO_VERSION=$(go version | awk '{print $3}')
info "Terdeteksi ${GO_VERSION}"
if ! go version | grep -qE 'go1\.(2[2-9]|[3-9][0-9])'; then
  die "Butuh Go 1.22+ (package gopher-lua membutuhkan versi tersebut)."
fi

# --- build ---------------------------------------------------------------
step "Build binary"
if ! go build -o wdbgo ./cmd/wdbgo; then
  die "Build gagal. Jalankan: go test ./... untuk lihat error."
fi
info "OK -> ./wdbgo"

# --- optional: config template ------------------------------------------
if [ "${1:-}" = "config" ]; then
  step "Buat config template"
  if [ ! -f wdbgo.toml.local ]; then
    cp wdbgo.toml.example wdbgo.toml.local
    info "wdbgo.toml.local dibuat — isi token & user_hash di file itu"
  else
    info "wdbgo.toml.local sudah ada, dibiarkan"
  fi
fi

# --- optional: dry-run check --------------------------------------------
if [ "${1:-}" = "dry" ]; then
  step "Verifikasi dry-run (5 bet)"
  if [ ! -f wdbgo.toml.local ]; then
    info "(tanpa config, pakai balance kertas 1000 & currency default)"
    ./wdbgo --dry-run --script scripts/example-martingale.lua --max-bets 5 >/dev/null 2>&1 \
      || die "dry-run gagal. Jalankan manual untuk lihat outputnya."
  else
    ./wdbgo --dry-run --config wdbgo.toml.local --script scripts/example-martingale.lua --max-bets 5 >/dev/null 2>&1 \
      || die "dry-run gagal (cek config + jaringan)."
  fi
  info "Dry-run 5 bet selesai tanpa error"
fi

step "Setup selesai."
info "Jalankan live:  ./wdbgo --config wdbgo.toml.local --script <script.lua> --currency free"
info "Dry-run:        ./wdbgo --dry-run --config wdbgo.toml.local --script <script.lua>"
