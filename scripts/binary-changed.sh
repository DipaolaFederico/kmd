#!/usr/bin/env bash
# binary-changed.sh BASE HEAD — build kmd at two git revisions with identical,
# reproducible settings and report whether the binaries differ. Exit status:
# 0 changed, 1 identical, 2 error. Changes that only touch tests, docs, CI or
# scripts leave the binary byte-for-byte identical, and do not deserve a
# release.
#   scripts/binary-changed.sh v0.2.0 HEAD
set -euo pipefail

base=$1 head=$2
tmp=$(mktemp -d)
cleanup() {
  git worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
  git worktree remove --force "$tmp/head" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

build() { # rev dir out
  git worktree add -q --detach "$2" "$1"
  # -buildvcs=false and a fixed version: the commit id and the version must
  # not count as a change
  (cd "$2" && CGO_ENABLED=0 go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X github.com/DipaolaFederico/kmd/internal/cli.Version=compare" -o "$3" .)
}
build "$base" "$tmp/base" "$tmp/kmd.base" || exit 2
build "$head" "$tmp/head" "$tmp/kmd.head" || exit 2

a=$(sha256sum <"$tmp/kmd.base") b=$(sha256sum <"$tmp/kmd.head")
if [ "$a" = "$b" ]; then
  echo "binary unchanged between $base and $head"
  exit 1
fi
echo "binary changed between $base and $head"
