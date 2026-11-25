#!/usr/bin/env bash
# binary-changed.sh BASE HEAD — build kmd at two git revisions for every
# released platform (release-targets.sh), with identical, reproducible
# settings, and report whether any binary differs. Exit status: 0 changed,
# 1 identical, 2 error. Changes that only touch tests, docs, CI or scripts
# leave every binary byte-for-byte identical, and do not deserve a release;
# a change behind runtime.GOOS or a build constraint (macOS, FreeBSD,
# Windows) changes only that platform's binary, and does.
#   scripts/binary-changed.sh v0.2.0 HEAD
set -euo pipefail

base=$1 head=$2
targets=$("$(dirname "$0")/release-targets.sh")
tmp=$(mktemp -d)
cleanup() {
  git worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
  git worktree remove --force "$tmp/head" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

build() { # rev dir out — every target, in parallel; fails if any build does
  git worktree add -q --detach "$2" "$1"
  mkdir -p "$3"
  local t p pids=() rc=0
  for t in $targets; do
    # -buildvcs=false and a fixed version: the commit id and the version
    # must not count as a change
    (cd "$2" && GOOS=${t%/*} GOARCH=${t#*/} CGO_ENABLED=0 go build -trimpath -buildvcs=false \
      -ldflags "-s -w -X github.com/DipaolaFederico/kmd/internal/cli.Version=compare" \
      -o "$3/${t//\//-}" .) &
    pids+=($!)
  done
  for p in "${pids[@]}"; do wait "$p" || rc=1; done
  return $rc
}
build "$base" "$tmp/base" "$tmp/out.base" || exit 2
build "$head" "$tmp/head" "$tmp/out.head" || exit 2

changed=()
for t in $targets; do
  cmp -s "$tmp/out.base/${t//\//-}" "$tmp/out.head/${t//\//-}" || changed+=("$t")
done
if [ ${#changed[@]} -eq 0 ]; then
  echo "binaries unchanged between $base and $head ($(paste -sd' ' <<<"$targets"))"
  exit 1
fi
echo "binary changed between $base and $head: ${changed[*]}"
