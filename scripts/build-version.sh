#!/usr/bin/env bash
# build-version.sh — print the version of a local build (make build,
# make install) in git describe's form: the tag on a release tag, else
# vX.Y.Z-N-gHASH, where X.Y.Z is the last release (stamped in
# packaging/kmd.spec by every release) and N the commits since it was;
# -dirty with uncommitted changes, and just vX.Y.Z outside a git repository
# (a release's source tarball) or before the first commit.
#
# git describe alone reports an old version on main: a release tag points
# to a commit that stamps the version files on top of the released one, and
# that commit reaches main only squashed into the release pull request.
set -euo pipefail
cd "$(dirname "$0")/.."

spec=packaging/kmd.spec
ver=$(sed -n 's/^Version: *//p' "$spec")
if ! git rev-parse -q --verify HEAD >/dev/null 2>&1; then
  echo "v$ver"
  exit
fi
dirty=
git diff --quiet HEAD -- 2>/dev/null || dirty=-dirty
if tag=$(git describe --tags --exact-match --match 'v[0-9]*' 2>/dev/null); then
  echo "$tag$dirty"
  exit
fi
hash=$(git rev-parse --short HEAD)
stamp=$(git log -1 --format=%H -G '^Version:' -- "$spec") # other edits to the spec do not count
if [ -z "$stamp" ]; then # the spec is not committed yet
  echo "v$ver-g$hash$dirty"
  exit
fi
echo "v$ver-$(git rev-list --count "$stamp..HEAD")-g$hash$dirty"
