#!/usr/bin/env bash
#
# wdb-go — update script
# Pulls latest from the configured remote, rebuilds the binary and re-runs
# the test suite so you always run the current build.
#
# Usage:
#   ./update.sh          # git pull + go test + build
#   ./update.sh --quiet  # skip tests, only pull + build
#
set -euo pipefail

cd "$(dirname "$0")"

step() { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }
info() { printf '   \033[90m%s\033[0m\n' "$*"; }
die()  { printf '\033[1;31m!! %s\033[0m\n' "$*" >&2; exit 1; }

REMOTE=$(git remote get-url origin 2>/dev/null || true)
if [ -z "$REMOTE" ]; then
  die "Belum ada remote 'origin'. Jadikan dulu: git remote add origin <url>"
fi

step "Cek perubahan lokal"
if ! git diff --quiet; then
  note="Ada perubahan lokal yang belum di-commit:"
  git status --short | sed 's/^/    /'
  info "(tidak masalah untuk pull, tapi perhatikan konflik)"
fi

step "Pull dari origin"
git pull --ff-only origin 2>&1 || die "git pull gagal (remote/konflik)."

step "Cek toolchain"
command -v go >/dev/null 2>&1 || die "Go tidak terdeteksi. Install https://go.dev/dl/"

step "Build binary"
go build -o wdbgo ./cmd/wdbgo
info "OK -> ./wdbgo"

if [ "${1:-}" != "--quiet" ]; then
  step "Jalankan test"
  go test ./...
  info "Test selesai"
fi

step "Update selesai"
info "Jalankan live:  ./wdbgo --config wdbgo.toml.local --script <script.lua> --currency free"
