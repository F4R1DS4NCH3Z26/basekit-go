#!/usr/bin/env bash
# Generates the per-tool subagents of this repository from .agents/roles and .agents/zones:
#   .claude/agents/<role>.md    (Claude Code)
#   .opencode/agents/<role>.md  (OpenCode: the zone becomes its edit permission)
# The generated files are never edited by hand: change the role or the zone and run `make agents`.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"
mkdir -p .claude/agents .opencode/agents

# A role that no longer exists leaves no generated adapter behind (only files this script wrote).
for f in .claude/agents/*.md .opencode/agents/*.md; do
  [ -f "$f" ] || continue
  role=$(basename "$f" .md)
  if [ ! -f ".agents/roles/$role.md" ] && grep -q 'Generado por scripts/gen-agents.sh' "$f"; then
    rm -f "$f"; echo "  - $f"
  fi
done

json() { # quotes $1 as a JSON string (also a valid YAML scalar)
  local s=${1//\\/\\\\}; s=${s//\"/\\\"}; printf '"%s"' "$s"
}

for role_file in .agents/roles/*.md; do
  role=$(basename "$role_file" .md)
  zone_file=".agents/zones/$role.txt"
  [ -f "$zone_file" ] || { echo "ERROR: falta $zone_file"; exit 1; }
  desc=$(awk '/^---$/{n++; next} n==1 && /^description:/{sub(/^description:[[:space:]]*/, ""); print; exit}' "$role_file")
  [ -n "$desc" ] || { echo "ERROR: $role_file no tiene description"; exit 1; }
  body=$(awk '/^---$/{n++; next} n>=2' "$role_file")
  header="<!-- Generado por scripts/gen-agents.sh desde .agents/roles/$role.md y .agents/zones/$role.txt: edita esos archivos y ejecuta \`make agents\` -->"

  {
    echo "---"
    echo "name: $role"
    echo "description: $(json "$desc")"
    echo "tools: Read, Edit, Write, Grep, Glob, Bash"
    echo "model: sonnet"
    echo "---"
    echo "$header"
    echo "$body"
  } > ".claude/agents/$role.md"

  {
    echo "---"
    echo "description: $(json "$desc")"
    echo "mode: subagent"
    echo "permission:"
    echo "  edit:"
    echo '    "*": deny'
    while IFS= read -r z || [ -n "$z" ]; do
      z="${z%%#*}"; z="${z%"${z##*[![:space:]]}"}"
      [ -n "$z" ] && echo "    $(json "$z"): allow"
    done < "$zone_file"
    # The last matching rule wins in OpenCode: writers can never touch tests, only the qa-writer.
    [ "$role" = qa-writer ] || echo '    "**/*_test.go": deny'
    echo "  bash:"
    for rule in '"*": ask' '"make verify*": allow' '"make test*": allow' '"make template-check*": allow' \
                '"make zones*": allow' '"go doc*": allow' '"git status*": allow' \
                '"bash .agents/skills/*": allow' '"git push*": deny' '"rm -rf*": deny'; do
      echo "    $rule"
    done
    echo "  webfetch: deny"
    echo "---"
    echo "$header"
    echo "$body"
  } > ".opencode/agents/$role.md"
  echo "  ~ .claude/agents/$role.md · .opencode/agents/$role.md"
done
