#!/usr/bin/env bash
# Valida un stack de plantillas/ sobre un proyecto generado. Uso: check.sh <stack>
set -uo pipefail
cd "$(git rev-parse --show-toplevel)"
stack="${1:-}"
[ -n "$stack" ] && [ -f "plantillas/stacks/$stack/stack.json" ] || {
  echo "Uso: check.sh <stack> ($(ls plantillas/stacks | tr '\n' ' '))"; exit 2; }
# Every backend or fullstack stack offers postgres + redis; a frontend stack has no database (S1b), so it gets none.
kind=$(sed -n 's/.*"kind": *"\([a-z]*\)".*/\1/p' "plantillas/stacks/$stack/stack.json" | head -1)
if [ "$kind" = frontend ]; then db=none; infra=(--db none); else db=postgres; infra=(--db postgres --redis); fi
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
fail=0
ok()  { echo "OK   $1"; }
bad() { fail=1; echo "FAIL $1"; [ -n "${2:-}" ] && printf '%s\n' "$2" | grep -v '^[[:space:]]*$' | head -n 20; }

go build -o "$tmp/basekit" ./cmd/basekit || { echo "FAIL build"; exit 1; }
p="$tmp/$stack-check"
out=$("$tmp/basekit" init "$p" --yes --no-external-skills --stack "$stack" "${infra[@]}" 2>&1) \
  && ok "init --stack $stack ${infra[*]}" || { bad "init" "$out"; exit 1; }
cd "$p"

errs=""; while IFS= read -r -d '' f; do e=$(bash -n "$f" 2>&1) || errs+="$f: $e"$'\n'; done < <(find . -name '*.sh' -not -path './.git/*' -print0)
[ -z "$errs" ] && ok "bash -n de los scripts" || bad "bash -n" "$errs"

# Acceptance test 1 of docs/guias/02_especificacion_cli.md §13: `make help` lists at least 25 targets.
n=$(make help 2>/dev/null | grep -c '^[[:space:]]\+[^[:space:]]')
[ "$n" -ge 25 ] && ok "make help ($n objetivos)" || bad "make help ($n objetivos, se esperan ≥ 25)"

out=$(make env 2>&1 && make env ENV=prod 2>&1) && ok "make env (dev y prod)" || bad "make env" "$out"
for args in "dev" "prod-up" "migrate" "verify SCOPE=src"; do
  out=$(make -n $args 2>&1) && ok "make -n $args" || bad "make -n $args" "$out"
done

if docker info >/dev/null 2>&1; then
  for env in dev prod; do
    ef=.env; [ "$env" = prod ] && ef=.env.prod
    files=(-f compose.yaml -f "compose.$env.yaml"); for o in compose.db.yaml compose.redis.yaml; do [ -f "$o" ] && files+=(-f "$o"); done
    out=$(docker compose --env-file "$ef" "${files[@]}" --profile tools config -q 2>&1) \
      && ok "docker compose config ($env + overlays)" || bad "docker compose config ($env)" "$out"
  done
else
  echo "SKIP docker compose config (Docker no disponible)"
fi

out=$(bash .agents/skills/zone-check/scripts/check_zones.sh --owners 2>&1)
printf '%s' "$out" | grep -q ' 0 sin dueño' && ok "zone-check --owners" || bad "zone-check --owners" "$out"

[ $fail -eq 0 ] && echo "template-check $stack: OK" || echo "template-check $stack: FALLO"; exit $fail
