#!/usr/bin/env bash
# Verificación silenciosa del CLI (Go). Uso: PKG=./internal/fsx/... verify.sh. Sin red (GOFLAGS=-mod=readonly).
set -uo pipefail
cd "$(git rev-parse --show-toplevel)"; MAX="${MAX_LINES:-30}"; fail=0; pkg="${PKG:-./...}"
export GOFLAGS=-mod=readonly
run() { local label=$1 mode=$2 out; shift 2
  if out=$("$@" 2>&1); then echo "OK   $label"
  else fail=1; echo "FAIL $label"; printf '%s\n' "$out" | grep -v '^[[:space:]]*$' | "$mode" -n "$MAX"; fi; }
fmtcheck() { local o; o=$(gofmt -l $(git ls-files '*.go')); [ -z "$o" ] || { echo "$o"; return 1; }; }
run gofmt head fmtcheck
run vet   head go vet "$pkg"
if command -v staticcheck >/dev/null; then run staticcheck head staticcheck "$pkg"; else echo "SKIP staticcheck"; fi
run test  tail go test -count=1 "$pkg"
[ $fail -eq 0 ] && echo "verify: OK" || echo "verify: FALLO"; exit $fail
