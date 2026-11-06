#!/usr/bin/env bash
# Validate permission allowlist JSON files ({"writer":[...],"controller":[...],"orchestrator":[...]}).
# Every entry must match the rule form, and no entry may start with a prefix its role is denied.
# Usage: scripts/allowlist-check.sh <allowlist.json>...
set -u

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <allowlist.json>..." >&2
  exit 2
fi
command -v jq >/dev/null 2>&1 || { echo "allowlist-check: jq is required" >&2; exit 2; }

RULE_RE='^[A-Za-z]+\([^()*]+(:\*)?\)$'

DESTRUCTIVE=(
  "git push --force" "git push -f" "git reset --hard" "git clean -fdx" "git clean -fd"
  "git branch -D" "rm -rf /" "rm -rf ~" "rm -rf .."
)
DENY_writer=("git push" "gh pr create" "gh pr merge" "ao pr merge" "ao spawn" "ao decision record" "${DESTRUCTIVE[@]}")
DENY_controller=("gh pr merge" "ao pr merge" "ao spawn" "ao decision record" "ao session handoff-confirm" "${DESTRUCTIVE[@]}")
DENY_orchestrator=("${DESTRUCTIVE[@]}")

status=0
for file in "$@"; do
  if ! jq -e 'type == "object"' "$file" >/dev/null 2>&1; then
    echo "$file: not a JSON object" >&2
    status=1
    continue
  fi
  for role in writer controller orchestrator; do
    deny_name="DENY_$role[@]"
    denied=("${!deny_name}")
    while IFS= read -r entry; do
      if ! [[ "$entry" =~ $RULE_RE ]]; then
        echo "$file: $role: entry fails the rule form: $entry" >&2
        status=1
        continue
      fi
      cmd="${entry#*(}"
      cmd="${cmd%)}"
      cmd="${cmd%:\*}"
      if [[ "$entry" == Bash\(* ]]; then
        for d in "${denied[@]}"; do
          if [[ "$cmd" == "$d"* ]]; then
            echo "$file: $role: entry starts with denied prefix '$d': $entry" >&2
            status=1
          fi
        done
      fi
    done < <(jq -r --arg r "$role" '.[$r] // [] | .[] | tostring' "$file")
  done
done
exit "$status"
