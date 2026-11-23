#!/usr/bin/env bash
# complexity.sh GOCOGNIT [LIMIT [BASE]] — fail when a function added or
# made more complex since BASE (default: the merge base with main, else
# HEAD) has a cognitive complexity above LIMIT (default 30). Existing
# complex functions are tolerated until they are touched: the check is a
# ratchet, so it never asks to rewrite code a change did not make worse.
# Test files are not checked.
set -euo pipefail
[ $# -ge 1 ] && [ $# -le 3 ] || { echo "usage: complexity.sh GOCOGNIT [LIMIT [BASE]]" >&2; exit 2; }
gocognit=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
limit=${2:-30}
cd "$(dirname "$0")/.."

base=${3:-}
if [ -z "$base" ]; then
  base=$(git merge-base HEAD main 2>/dev/null || git merge-base HEAD origin/main 2>/dev/null || git rev-parse HEAD)
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# scores as "package function file<TAB>score", without line numbers (they move)
scores() { # dir
  (cd "$1" && "$gocognit" -ignore '_test\.go$' . 2>/dev/null) |
    awk '{ s=$1; f=$NF; sub(/:[0-9]+:[0-9]+$/, "", f); $1=""; $NF=""; sub(/^ /, ""); print $0 f "\t" s }' | sort
}

# the base tree, for the Go files that exist there
mkdir -p "$tmp/base"
git archive "$base" | tar -x -C "$tmp/base"
scores "$tmp/base" >"$tmp/before"
scores . >"$tmp/after"

# functions above the limit that are new, or whose score grew
awk -F'\t' -v limit="$limit" '
  NR == FNR { before[$1] = $2; next }
  $2 > limit && (!($1 in before) || $2 > before[$1]) {
    was = ($1 in before) ? " (was " before[$1] ")" : " (new)"
    printf "  %s: cognitive complexity %d%s, limit %d\n", $1, $2, was, limit
    bad = 1
  }
  END { exit bad }
' "$tmp/before" "$tmp/after" || {
  echo "complexity: split these functions (see CONTRIBUTING.md, \"Style\")" >&2
  exit 1
}
