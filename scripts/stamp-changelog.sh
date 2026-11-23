#!/usr/bin/env bash
# stamp-changelog.sh VERSION DATE [REF] — turn CHANGELOG.md's [Unreleased]
# entries into a "## [VERSION] - DATE" section and update the compare links
# at the bottom (Keep a Changelog). Called by scripts/sync-version.sh.
#
# The section gets the entries that were unreleased in REF's CHANGELOG.md
# (or REF's own [VERSION] section, for a release tag), and only those leave
# [Unreleased]: the version pull request stamps main's current file, which
# may already have entries for changes merged after the released commit.
# Without REF, the file's own [Unreleased] entries are used. Doing nothing
# when the section exists makes it idempotent. A section ends at the next
# "## " heading or at the version links ([Unreleased]: / [X.Y.Z]:), so
# entries may use other reference links.
set -euo pipefail

[ $# -eq 2 ] || [ $# -eq 3 ] || { echo "usage: stamp-changelog.sh VERSION DATE [REF]" >&2; exit 2; }
ver=$1 date=$2 ref=${3:-}
[[ $ver =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "stamp-changelog: bad version '$ver'" >&2; exit 2; }
[[ $date =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || { echo "stamp-changelog: bad date '$date'" >&2; exit 2; }
root=$(cd "$(dirname "$0")/.." && pwd)
log=$root/CHANGELOG.md

[ -f "$log" ] || exit 0
grep -q "^## \[${ver//./\\.}\]" "$log" && exit 0
grep -qx '## \[Unreleased\]' "$log" || { echo "stamp-changelog: CHANGELOG.md has no '## [Unreleased]'" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
if [ -n "$ref" ]; then
  git -C "$root" rev-parse -q --verify "$ref^{commit}" >/dev/null || { echo "stamp-changelog: no commit '$ref'" >&2; exit 1; }
  # a released commit from before the changelog existed has no entries
  if git -C "$root" cat-file -e "$ref:CHANGELOG.md" 2>/dev/null; then
    git -C "$root" show "$ref:CHANGELOG.md" >"$tmp/src"
  else
    : >"$tmp/src"
  fi
else
  cp "$log" "$tmp/src"
fi

# shellcheck disable=SC2016 # awk program
prog='
# An entry is a "- " or "* " line plus its continuation lines, keyed by its
# "### " subsection, so a later entry that happens to share a line with a
# released one is never removed.
function parse(text, keys, n,   lines, m, i, l, sub_, cur) {
  n = 0; sub_ = ""; cur = ""
  m = split(text, lines, "\n")
  for (i = 1; i <= m; i++) {
    l = lines[i]
    if (l ~ /^### /) { if (cur != "") keys[++n] = sub_ SUBSEP cur; cur = ""; sub_ = substr(l, 5); continue }
    if (l ~ /^[-*] / || l ~ /^[ \t]*$/) { if (cur != "") keys[++n] = sub_ SUBSEP cur; cur = "" }
    if (l ~ /^[ \t]*$/) continue
    cur = cur == "" ? l : cur "\n" l
  }
  if (cur != "") keys[++n] = sub_ SUBSEP cur
  return n
}
function trim(s) { sub(/^\n+/, "", s); sub(/\n+$/, "", s); return s }
function emit(   n, k, i, j, s, e, subs, ns, seen, keep) {
  # what stays unreleased, grouped by subsection in first-seen order
  n = parse(ubuf, k)
  for (i = 1; i <= n; i++) {
    if (k[i] in released) continue
    split(k[i], e, SUBSEP)
    if (!(e[1] in seen)) { seen[e[1]] = 1; subs[++ns] = e[1] }
    keep[e[1]] = keep[e[1]] e[2] "\n"
  }
  print ""
  for (j = 1; j <= ns; j++) {
    if (subs[j] != "") { print "### " subs[j]; print "" }
    printf "%s\n", keep[subs[j]]
  }
  print "## [" ver "] - " date
  print ""
  if (src != "") { print src; print "" }
  done = 1
}
FNR == 1 { file++ }
file == 1 {
  if ($0 ~ /^## /) { cur = index($0, "## [" ver "]") == 1 ? "ver" : ($0 == "## [Unreleased]" ? "unrel" : ""); next }
  if ($0 ~ /^\[(Unreleased|[0-9]+\.[0-9]+\.[0-9]+)\]: /) { cur = ""; next }
  if (cur != "") body[cur] = body[cur] $0 "\n"
  next
}
file == 2 && FNR == 1 {
  src = trim(("ver" in body) ? body["ver"] : body["unrel"])
  n = parse(src, k)
  for (i = 1; i <= n; i++) released[k[i]] = 1
}
state == 1 && ($0 ~ /^## / || $0 ~ /^\[(Unreleased|[0-9]+\.[0-9]+\.[0-9]+)\]: /) { emit(); state = 2 }
state == 1 { ubuf = ubuf $0 "\n"; next }
$0 == "## [Unreleased]" && !state { print; state = 1; next }
/^\[Unreleased\]: .*\/compare\/v[0-9.]+\.\.\.HEAD$/ {
  base = $0; sub(/^\[Unreleased\]: /, "", base); sub(/\/compare\/.*/, "", base)
  prev = $0; sub(/.*\/compare\/v/, "", prev); sub(/\.\.\.HEAD$/, "", prev)
  print "[Unreleased]: " base "/compare/v" ver "...HEAD"
  print "[" ver "]: " base "/compare/v" prev "...v" ver
  next
}
{ print }
END { if (state == 1 && !done) emit() }
'
awk -v ver="$ver" -v date="$date" "$prog" "$tmp/src" "$log" >"$tmp/out"
cat "$tmp/out" >"$log"
grep -q "^## \[${ver//./\\.}\] - $date\$" "$log" || { echo "stamp-changelog: stamping failed" >&2; exit 1; }
echo "changelog: $ver ($date)"
