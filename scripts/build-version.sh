#!/usr/bin/env bash
# build-version.sh — print the version of a local build (make build,
# make install), in git describe's form:
#   vX.Y.Z             on a release tag, outside a git repository (a
#                      release's source tarball) or before the first commit
#   vX.Y.Z-N-gHASH     otherwise: X.Y.Z is the last release, stamped in
#                      packaging/kmd.spec by every release, and N the commits
#                      since it was (0 in a shallow clone)
#   …-dirty            with uncommitted changes
# It fails (make then uses "dev") when the result is not such a version: a
# tag name can hold quotes and other shell syntax that would end up in the
# go build command line, or control characters that would reach --version.
#
# git describe alone reports an old version on main: a release tag points
# to a commit that stamps the version files on top of the released one, and
# that commit reaches main only squashed into the release pull request.
set -euo pipefail
cd "$(dirname "$0")/.."

spec=packaging/kmd.spec
ver=v$(sed -n 's/^Version: *//p' "$spec")
# kmd's own repository only: a source tarball unpacked inside another
# repository must not take that one's commits or tags
if [ "$(git rev-parse --show-toplevel 2>/dev/null)" != "$(pwd -P)" ] ||
  ! git rev-parse -q --verify HEAD >/dev/null; then
  v=$ver
else
  dirty=
  git diff --quiet HEAD -- || dirty=-dirty
  if tag=$(git describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null); then
    v=$tag$dirty
  else
    stamp=$(git log -1 --no-show-signature --format=%H -G '^Version:' -- "$spec") # other spec edits do not count
    v=$ver-$(git rev-list --count "$stamp..HEAD")-g$(git rev-parse --short HEAD)$dirty
  fi
fi
if [[ ! $v =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9]+-g[0-9a-f]+)?(-dirty)?$ ]]; then
  echo "build-version: '$v' is not a version" >&2
  exit 1
fi
echo "$v"
