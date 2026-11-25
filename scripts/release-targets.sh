#!/usr/bin/env bash
# release-targets.sh — print the GOOS/GOARCH pairs kmd is released for, one
# per line. The one list the release build, CI's cross-compile and
# binary-changed.sh share: a platform added here is built, checked and
# compared everywhere.
set -euo pipefail
printf '%s\n' \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64 \
  freebsd/amd64 \
  windows/amd64
