#!/usr/bin/env bash
# complexity.sh GOCOGNIT LIMIT [BASE] — fail when a function added or made
# more complex since BASE has a cognitive complexity above LIMIT. GOCOGNIT
# is the gocognit binary (a path, or a name on PATH). BASE defaults to the
# merge base with origin/main or main, whichever is newer.
#
# Existing complex functions are tolerated until a change makes them worse:
# the check is a ratchet, so it never asks to rewrite code a change did not
# touch. A function is identified by its package, name (with receiver) and
# file, so moving or renaming a complex function counts as touching it.
# Only tracked Go files are checked, tests excluded.
#
# Without a base to compare with (a shallow clone, no main branch) nothing
# can be checked: it says so, and fails under STRICT=1 (CI).
set -euo pipefail
[ $# -ge 2 ] && [ $# -le 3 ] || { echo "usage: complexity.sh GOCOGNIT LIMIT [BASE]" >&2; exit 2; }
gocognit=$(command -v "$1") || { echo "complexity: $1 not found" >&2; exit 2; }
case $gocognit in /*) ;; *) gocognit=$PWD/$gocognit ;; esac
limit=$2
cd "$(dirname "$0")/.."

base=${3:-}
if [ -z "$base" ]; then
  # the newer of the two merge bases: a local main is often behind origin
  for ref in origin/main main; do
    b=$(git merge-base HEAD "$ref" 2>/dev/null) || continue
    if [ -z "$base" ] || git merge-base --is-ancestor "$base" "$b"; then
      base=$b
    fi
  done
fi
if [ -z "$base" ]; then
  echo "complexity: no main branch to compare with (shallow clone?): not checked" >&2
  [ "${STRICT:-}" = 1 ] && exit 1
  exit 0
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# "package function file<TAB>score" for the given files, without positions
# (they move when code above them changes)
scores() { # dir, files on stdin
  local files
  files=$(grep -v '_test\.go$' || true)
  [ -n "$files" ] || return 0
  # its note about recursion it cannot resolve without type information
  # is expected (-exact-recursion is ~100 times slower); errors still show
  # shellcheck disable=SC2086 # one file per word: tracked paths have no spaces
  (cd "$1" && "$gocognit" $files 2> >(grep -v '^gocognit: note:' >&2)) |
    awk '{ s = $1; f = $NF; sub(/:[0-9]+:[0-9]+$/, "", f); $1 = ""; $NF = ""; sub(/^ /, ""); print $0 f "\t" s }' |
    sort
}

mkdir -p "$tmp/base"
git archive "$base" -- '*.go' | tar -x -C "$tmp/base"
(cd "$tmp/base" && find . -name '*.go' | sed 's|^\./||') | scores "$tmp/base" >"$tmp/before"
git ls-files '*.go' | scores . >"$tmp/after"

# functions above the limit that are new, or whose score grew
awk -F'\t' -v limit="$limit" '
  FILENAME == ARGV[1] { before[$1] = $2; next } # (NR == FNR fails on an empty base)
  $2 > limit && (!($1 in before) || $2 > before[$1]) {
    was = ($1 in before) ? " (was " before[$1] ")" : " (new)"
    printf "  %s: cognitive complexity %d%s, limit %d\n", $1, $2, was, limit
    bad = 1
  }
  END { exit bad }
' "$tmp/before" "$tmp/after" >&2 || {
  echo "complexity: split these functions (see CONTRIBUTING.md, \"Style\")" >&2
  exit 1
}
