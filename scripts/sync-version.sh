#!/usr/bin/env bash
# sync-version.sh [VERSION DATE] — bring the files that carry the version (RPM
# spec, man page, AppStream metainfo) up to date with the releases: stamp,
# oldest first, every vX.Y.Z tag newer than the spec's version (dated by its
# commit), then VERSION if given (the release being made). The metainfo thus
# gets one <release> per version even when an earlier version pull request
# was never merged. Needs the tags (fetch-depth: 0). Used by the release
# workflow, both for the tagged commit and for the pull request to main.
set -euo pipefail
[ $# -eq 0 ] || [ $# -eq 2 ] || { echo "usage: sync-version.sh [VERSION DATE]" >&2; exit 2; }
cd "$(dirname "$0")/.."

cur=$(sed -n -E 's/^Version:[[:space:]]+//p' packaging/kmd.spec)
newer() { # $1 > $2
  [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | tail -1)" = "$1" ]
}
if [ $# -eq 2 ] && newer "$cur" "$1"; then
  echo "sync-version: the spec already has $cur, newer than $1" >&2
  exit 1
fi

# strict vX.Y.Z tags only, like the release workflow; those after the spec's
tags=$(git tag -l 'v*' | sed -n -E 's/^v((0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))$/\1/p' |
  { cat; echo "$cur"; } | sort -V -u | awk -v c="$cur" 'f; $0 == c { f = 1 }')
for v in $tags; do
  if [ $# -eq 2 ] && ! newer "$1" "$v"; then continue; fi # VERSION: stamped below
  scripts/stamp-version.sh "$v" "$(git log -1 --format=%cs "v$v^{commit}")"
done
if [ $# -eq 2 ]; then scripts/stamp-version.sh "$1" "$2"; fi
