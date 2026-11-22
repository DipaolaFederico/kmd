#!/usr/bin/env bash
# test-scripts.sh — tests for the release helper scripts (run by `make ci`).
set -euo pipefail
cd "$(dirname "$0")/.."
fails=0
eq() { # want got description
  if [ "$1" = "$2" ]; then return; fi
  echo "FAIL: $3: want '$1', got '$2'"
  fails=$((fails + 1))
}

# next-version.sh
eq 0.2.1 "$(scripts/next-version.sh v0.2.0 patch)" "patch"
eq 0.3.0 "$(scripts/next-version.sh v0.2.0 minor)" "minor"
eq 1.0.0 "$(scripts/next-version.sh 0.9.9 major)" "major"
eq 1.10.0 "$(scripts/next-version.sh v1.9.9 minor)" "carry"
for bad in "v0.2 patch" "v01.2.0 patch" "v0.2.0 huge" "v0.2.0-rc1 patch"; do
  # shellcheck disable=SC2086
  if scripts/next-version.sh $bad >/dev/null 2>&1; then eq rejected accepted "next-version $bad"; fi
done

# release-bump.sh
bump() { printf '%s\0' "$@" | scripts/release-bump.sh 2>/dev/null; }
eq minor "$(bump 'feat: viewer' 'ci: release' 'docs: readme')" "feat wins over ci/docs"
eq patch "$(bump 'fix(md): tabs' 'test: x')" "fix"
eq patch "$(bump 'perf(render): faster')" "perf"
eq patch "$(bump 'revert: feat: viewer')" "revert"
eq "" "$(bump 'ci: x' 'docs(readme): y' 'refactor: z' 'chore: a' 'build: b' 'style: c' 'test: d')" "no release types"
eq major "$(bump 'feat(cli)!: remove --foo')" "bang"
eq major "$(bump "$(printf 'fix: x\n\nBREAKING CHANGE: pager is builtin')")" "breaking footer"
eq minor "$(bump "$(printf 'feat: a\n\nBREAKING CHANGES are fun')")" "not a footer"
eq minor "$(bump 'feat: a' 'fix: b' 'perf: c')" "largest wins"
eq "" "$(bump 'foo!: x')" "unknown type with ! is ignored"
eq patch "$(bump "$(printf 'fix: x\n\n* feat: y\n\nBREAKING CHANGE: quoted\n\nmore text')")" "BREAKING CHANGE not in the last paragraph"
eq major "$(bump "$(printf 'fix: x\n\nbody\n\nRefs: #1\nBREAKING CHANGE: real footer')")" "footer with other trailers"
eq "" "$(bump 'BREAKING CHANGE: subject only')" "not conventional"
eq "" "$(bump 'Update stuff' 'feat:x' 'Feat: x' 'feat : x' 'wip: x')" "not conventional"

# stamp-version.sh (on a copy)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/scripts" "$tmp/packaging" "$tmp/docs"
cp scripts/stamp-version.sh "$tmp/scripts/"
cp packaging/kmd.spec packaging/io.github.dipaolafederico.kmd.metainfo.xml "$tmp/packaging/"
cp docs/kmd.1 "$tmp/docs/"
"$tmp/scripts/stamp-version.sh" 9.8.7 2030-01-02 >/dev/null
"$tmp/scripts/stamp-version.sh" 9.8.7 2030-01-02 >/dev/null # idempotent
eq "Version:                9.8.7" "$(grep '^Version:' "$tmp/packaging/kmd.spec")" "spec"
eq '.TH KMD 1 "2030-01-02" "kmd 9.8.7" "User Commands"' "$(head -1 "$tmp/docs/kmd.1")" "man page"
eq 1 "$(grep -c '<release version="9.8.7" date="2030-01-02"/>' "$tmp/packaging/io.github.dipaolafederico.kmd.metainfo.xml")" "metainfo"
if "$tmp/scripts/stamp-version.sh" 1.2 2030-01-02 >/dev/null 2>&1; then eq rejected accepted "stamp bad version"; fi

if [ "$fails" -gt 0 ]; then
  echo "$fails script test(s) failed"
  exit 1
fi
echo "  release scripts: ok"
