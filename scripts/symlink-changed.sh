#!/usr/bin/env bash
# symlink-changed.sh RANGE — exit 0 when a change in RANGE (as for
# `git diff`) leaves a symbolic link at some path, 1 when none does, 2 on
# error. CI then runs in full: a file turned into a symlink could pull
# runner files into a release. The modes come from git's raw diff,
# NUL-separated, so no file name, whatever characters it holds, is split
# or skipped.
#   scripts/symlink-changed.sh "$base...$head"
set -euo pipefail

raw=$(git diff --no-renames --raw -z "$1" | tr '\0' '\n') || exit 2
# records are ":oldmode newmode oldsha newsha status", then the path: a path
# that looks like a record can only add a false "yes", never hide a link
if grep -qE '^:[0-7]{6} 120000 ' <<<"$raw"; then
  exit 0
fi
exit 1
