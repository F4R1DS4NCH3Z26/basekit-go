#!/usr/bin/env bash
# Verificación silenciosa del CLI (Go). Uso: PKG=./internal/fsx/... [STATICCHECK=<comando>] verify.sh. No modifica go.mod ni go.sum (GOFLAGS=-mod=readonly).
set -uo pipefail
cd "$(git rev-parse --show-toplevel)"; MAX="${MAX_LINES:-30}"; fail=0; pkg="${PKG:-./...}"
export GOFLAGS=-mod=readonly
run() { local label=$1 mode=$2 out; shift 2
  if out=$("$@" 2>&1); then echo "OK   $label"
  else fail=1; echo "FAIL $label"; printf '%s\n' "$out" | grep -v '^[[:space:]]*$' | "$mode" -n "$MAX"; fi; }
# gofmt over the package directories of PKG: it honors the scope and also sees new files not yet tracked.
fmtcheck() { local d o=""; while IFS= read -r d; do o+=$(gofmt -l "$d"); done < <(go list -f '{{.Dir}}' "$pkg")
  [ -z "$o" ] || { echo "$o"; return 1; }; }
run gofmt head fmtcheck
run vet   head go vet "$pkg"
# STATICCHECK is the pinned command `make verify` passes (`go run …@v0.8.2`); run directly, it falls back to the one on PATH.
# shellcheck disable=SC2086
if [ -n "${STATICCHECK:-}" ]; then run staticcheck head $STATICCHECK "$pkg"
elif command -v staticcheck >/dev/null; then run staticcheck head staticcheck "$pkg"
else echo "SKIP staticcheck (make verify usa la versión fijada)"; fi
run test  tail go test -count=1 "$pkg"
[ $fail -eq 0 ] && echo "verify: OK" || echo "verify: FALLO"; exit $fail
