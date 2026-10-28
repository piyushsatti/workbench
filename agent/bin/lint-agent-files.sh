#!/bin/bash
# lint-agent-files.sh — mechanical checks for AGENTS.md / CLAUDE.md files.
# Report-only: every finding carries a check name,
# a severity (blocker/major/minor/info) and a lane:
#   A = agent can auto-heal   B = needs a human to pick the destination
#   C = scheduled thinning    INFO = recorded, no action defined yet
# Exit 1 if any blocker/major exists.

set -u

HOME_AGENT="$HOME/.agents/AGENTS.md"
HOME_WRAPPER="$HOME/.claude/CLAUDE.md"
CODEX_POINTER="$HOME/.codex/AGENTS.md"
# Root of the source tree to scan; override with AGENT_LINT_ROOT.
ROOT="${AGENT_LINT_ROOT:-$HOME/Source}"
# The workbench's agent/ holds the canonical AGENTS.md that deploys to $HOME_AGENT, not a project file.
WORKBENCH_AGENT="$ROOT/workbench/agent"
# Third-party plugin repos cloned beside the others (workbench plugins.json entries whose GitHub owner is not
# the catalog's) follow their own conventions, so they are not linted.
third_party=()
while IFS= read -r name; do
  [ -n "$name" ] && third_party+=( -o -path "$ROOT/$name" )
done < <(python3 - "$ROOT/workbench/plugins.json" 2>/dev/null <<'PY'
import json, sys
data = json.load(open(sys.argv[1]))
for entry in data.get("plugins", []):
    owner = entry.get("github", "").split("/")[0]
    if data.get("owner") and owner and owner != data["owner"]:
        print(entry["name"])
PY
)

SECRETS='sk-[a-zA-Z0-9_-]{20,}|ghp_[a-zA-Z0-9]{20,}|AKIA[A-Z0-9]{16}|-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----|(password|passwd|secret|token|api[_-]?key)[[:space:]]*[=:][[:space:]]*[^[:space:]]{8,}'

printf '%-72s %-10s %-8s %-4s %s\n' "FILE" "CHECK" "SEVERITY" "LANE" "PRESCRIBED ACTION"
printf '%.0s-' $(seq 1 132); echo

blockers=0; majors=0; minors=0

say() { # file check sev lane msg
  printf '%-72s %-10s %-8s %-4s %s\n' "$1" "$2" "$3" "$4" "$5"
  case "$3" in
    blocker) blockers=$((blockers+1));;
    major)   majors=$((majors+1));;
    minor)   minors=$((minors+1));;
  esac
}

# ---- collect governed files -------------------------------------------------
files=$( {
  ls "$HOME_AGENT" "$HOME_WRAPPER" "$CODEX_POINTER" "$ROOT/AGENTS.md" 2>/dev/null
  find "$ROOT" \
    \( -path "$WORKBENCH_AGENT" ${third_party[@]+"${third_party[@]}"} -o -name node_modules -o -name .git -o -name dist -o -name artifacts \
       -o -name worktrees -o -name .venv \
       -o -name template -o -name archive \
       -o -name "*snapshot*" \) -prune -o \
    \( -name AGENTS.md -o -name CLAUDE.md \) -print 2>/dev/null
} | sort -u )

while IFS= read -r f; do
  [ -n "$f" ] || continue
  rel=${f#"$HOME"/}
  base=$(basename "$f")

  # C6 — symlink ban, anywhere in the wiring
  if [ -L "$f" ]; then
    say "$rel" "symlink" "blocker" "A" "replace with pointer text or synced copy"
    continue
  fi

  # ---- role classification --------------------------------------------------
  role="ROOT"; total=$(wc -l < "$f" | tr -d ' ')
  case "$f" in
    "$HOME_AGENT")   role="HOME";;
    "$HOME_WRAPPER"|"$CODEX_POINTER") role="WIRING";;
  esac
  if [ "$role" = "ROOT" ]; then
    case "$f" in
      "$ROOT"/*) :;;
      *) role="WIRING";;
    esac
  fi
  if [ "$role" = "ROOT" ]; then
    # nested = deep subtree files; a git repo root or project root is ROOT-class
    dir=$(dirname "$f")
    top=$(git -C "$dir" rev-parse --show-toplevel 2>/dev/null)
    if [ -n "$top" ] && [ "$dir" != "$top" ] || [ -n "$(printf %s "$rel" | grep '/src/')" ]; then
      role="NESTED"
    fi
  fi

  # C2 — secret-shaped values
  hits=$(grep -inE "$SECRETS" "$f" 2>/dev/null | head -3)
  [ -n "$hits" ] && say "$rel" "secrets" "blocker" "B" \
    "value-shaped string(s); remove, store the credential in a secrets manager"

  case "$role" in
    WIRING) continue;;   # harness wiring: no budgets, no shim rules
    HOME)
      # no budget is defined for the home canonical file — report only
      say "$rel" "budget" "info" "-" "home canonical ${total} lines; no budget defined"
      continue;;
  esac

  # C1 — budgets: content lines = non-blank, non-pointer-table lines
  content=$(grep -cvE '^[[:space:]]*$|^[[:space:]]*\|' "$f")
  if [ "$role" = "ROOT" ] && [ "$content" -gt 20 ]; then
    say "$rel" "budget" "major" "C" "${content}/20 content lines; move detail into referenced docs"
  elif [ "$role" = "NESTED" ] && [ "$total" -gt 40 ]; then
    say "$rel" "budget" "major" "C" "${total}/40 lines; move detail into referenced docs"
  fi

  if [ "$base" = "AGENTS.md" ]; then
    # C3 — pointer map present at roots
    if [ "$role" = "ROOT" ] && [ "$(grep -c '^[[:space:]]*|' "$f")" -lt 4 ]; then
      say "$rel" "pointer-map" "minor" "A" "generate skeleton table from deferred resources"
    fi
    # C5 — shim presence beside every project AGENTS.md
    if [ ! -f "$(dirname "$f")/CLAUDE.md" ]; then
      say "$rel" "shim-missing" "major" "A" "write sibling CLAUDE.md containing exactly @AGENTS.md"
    fi
  fi

  if [ "$base" = "CLAUDE.md" ]; then
    body=$(tr -d '[:space:]' < "$f")
    if [ "$body" = "@AGENTS.md" ]; then
      : # compliant shim
    elif [ -f "$(dirname "$f")/AGENTS.md" ]; then
      say "$rel" "shim-content" "major" "B" \
        "both files exist: merge this content into AGENTS.md first, then reduce to @AGENTS.md"
    else
      say "$rel" "content-in-shim" "major" "B" "flip: git mv to AGENTS.md, write @AGENTS.md shim"
    fi
  fi

  # C7 — command blocks / install-run invocations belong in README/CONTRIBUTING
  if grep -qE '^[[:space:]]*```' "$f" || \
     grep -qE '(^|[[:space:]])(pnpm|npm|yarn|pip3?) (install|-m pytest)|^## +(Build|Setup|Commands|Installation?) *$' "$f"; then
    say "$rel" "commands" "minor" "C" "move invocation(s) to README/CONTRIBUTING"
  fi
done <<< "$files"

echo
echo "queue: ${blockers} blocker(s), ${majors} major(s), ${minors} minor(s)"
[ "$blockers" -eq 0 ] && [ "$majors" -eq 0 ] || exit 1
