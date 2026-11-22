#!/usr/bin/env bash
# next-version.sh CURRENT BUMP — print the version after CURRENT (vX.Y.Z or
# X.Y.Z) for BUMP (major | minor | patch), without the leading "v".
#   next-version.sh v0.2.0 minor   →  0.3.0
set -euo pipefail

cur=${1#v}
bump=${2:-}
if [[ ! $cur =~ ^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]]; then
  echo "next-version: '$1' is not a version (X.Y.Z)" >&2
  exit 2
fi
major=${BASH_REMATCH[1]} minor=${BASH_REMATCH[2]} patch=${BASH_REMATCH[3]}
case $bump in
  major) echo "$((major + 1)).0.0" ;;
  minor) echo "$major.$((minor + 1)).0" ;;
  patch) echo "$major.$minor.$((patch + 1))" ;;
  *) echo "next-version: bump must be major, minor or patch (got '$bump')" >&2; exit 2 ;;
esac
