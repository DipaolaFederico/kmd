#!/usr/bin/env bash
# test-scripts.sh — tests for the helper scripts (run by `make ci`).
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

# build-version.sh, in a scratch repository: the spec's version, the commits
# since it was stamped, the commit, -dirty; the tag on a release tag
bv=$(mktemp -d)
mkdir -p "$bv/scripts" "$bv/packaging"
cp scripts/build-version.sh "$bv/scripts/"
printf 'Name: kmd\nVersion:                1.2.3\n' > "$bv/packaging/kmd.spec"
eq v1.2.3 "$("$bv/scripts/build-version.sh")" "build-version outside git (source tarball)"
g() { git -C "$bv" -c user.name=t -c user.email=t@t -c commit.gpgsign=false -c tag.gpgsign=false "$@" >/dev/null; }
g init -q
eq v1.2.3 "$("$bv/scripts/build-version.sh")" "build-version before the first commit"
g add -A && g commit -qm 'chore(release): v1.2.3'
g commit -q --allow-empty -m 'feat: a'
echo 'Release: 2' >> "$bv/packaging/kmd.spec" # a packaging change, not a release
g commit -qam 'build: b'
eq "v1.2.3-2-g$(git -C "$bv" rev-parse --short HEAD)" "$("$bv/scripts/build-version.sh")" "build-version on main"
echo '# x' >> "$bv/packaging/kmd.spec"
eq "v1.2.3-2-g$(git -C "$bv" rev-parse --short HEAD)-dirty" "$("$bv/scripts/build-version.sh")" "build-version dirty"
g checkout -q -- packaging/kmd.spec
g tag v1.3.0
eq v1.3.0 "$("$bv/scripts/build-version.sh")" "build-version on a release tag"
rm -rf "$bv"

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

# stamp-changelog.sh (in its own git repository): the version gets the
# entries unreleased in the released commit; later ones stay unreleased
cl=$(mktemp -d)
trap 'rm -rf "$tmp" "$cl"' EXIT
mkdir -p "$cl/scripts"
cp scripts/stamp-changelog.sh "$cl/scripts/"
changelog() { # unreleased entries..., as a Keep a Changelog file at 9.8.7
  printf '# Changelog\n\n## [Unreleased]\n\n'
  for e in "$@"; do printf '%s\n' "$e"; done
  printf '\n## [9.8.7] - 2030-01-02\n\n### Added\n\n- Old.\n\n'
  printf '[Unreleased]: https://github.com/o/kmd/compare/v9.8.7...HEAD\n'
  printf '[9.8.7]: https://github.com/o/kmd/releases/tag/v9.8.7\n'
}
(
  cd "$cl"
  export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
  changelog '### Added' '' '- A,' '  wrapped.' '' '### Fixed' '' '- [B].' '' '[B]: https://e.com' '' '- D.' >CHANGELOG.md
  git init -q && git add -A && git commit -qm released
  changelog '### Added' '' '- A,' '  wrapped.' '' '### Fixed' '' '- [B].' '' '[B]: https://e.com' '' '- D.' '- C, later.' >CHANGELOG.md
  scripts/stamp-changelog.sh 9.9.0 2030-01-09 HEAD >/dev/null
  scripts/stamp-changelog.sh 9.9.0 2030-01-09 HEAD >/dev/null # idempotent
)
eq "## [Unreleased]||### Fixed||- C, later.||## [9.9.0] - 2030-01-09||### Added||- A,|  wrapped.||### Fixed||- [B].||[B]: https://e.com||- D.||## [9.8.7] - 2030-01-02" \
  "$(sed -n '/^## \[Unreleased\]/,/^## \[9.8.7\]/p' "$cl/CHANGELOG.md" | paste -sd'|')" "changelog: sections"
eq "[Unreleased]: https://github.com/o/kmd/compare/v9.9.0...HEAD|[9.9.0]: https://github.com/o/kmd/compare/v9.8.7...v9.9.0|[9.8.7]: https://github.com/o/kmd/releases/tag/v9.8.7" \
  "$(grep -E '^\[(Unreleased|9)' "$cl/CHANGELOG.md" | paste -sd'|')" "changelog: links"
(cd "$cl" && scripts/stamp-changelog.sh 9.9.1 2030-01-10 nosuchref >/dev/null 2>&1) && eq rejected accepted "changelog: bad ref"
if "$cl/scripts/stamp-changelog.sh" 9.9 2030-01-09 >/dev/null 2>&1; then eq rejected accepted "changelog bad version"; fi
rm "$cl/CHANGELOG.md"
"$cl/scripts/stamp-changelog.sh" 9.9.1 2030-01-10 >/dev/null || eq skipped failed "changelog: no CHANGELOG.md"

# sync-version.sh (the copy above, now at 9.8.7, as a git repository with
# releases the files missed)
cp scripts/sync-version.sh scripts/stamp-changelog.sh "$tmp/scripts/"
changelog >"$tmp/CHANGELOG.md"
meta=$tmp/packaging/io.github.dipaolafederico.kmd.metainfo.xml
(
  cd "$tmp"
  export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
  git init -q && git add -A && GIT_COMMITTER_DATE=2030-01-02T12:00:00Z git commit -qm a
  git tag v9.8.7
  for t in 9.8.8:2030-01-05 9.9.0:2030-01-09 10.0.0-rc1:2030-01-10; do
    GIT_COMMITTER_DATE=${t#*:}T12:00:00Z git commit -q --allow-empty -m "${t%%:*}"
    git tag "v${t%%:*}"
  done
  changelog '- New.' >CHANGELOG.md
  scripts/sync-version.sh 9.10.0 2030-02-03 >/dev/null
  scripts/sync-version.sh 9.10.0 2030-02-03 >/dev/null # idempotent
)
eq "## [Unreleased]|## [9.10.0] - 2030-02-03|- New.|## [9.9.0] - 2030-01-09|## [9.8.8] - 2030-01-05|## [9.8.7] - 2030-01-02" \
  "$(grep -E '^(## |- New)' "$tmp/CHANGELOG.md" | paste -sd'|')" "sync: changelog history"
eq "Version:                9.10.0" "$(grep '^Version:' "$tmp/packaging/kmd.spec")" "sync: spec"
eq '"kmd 9.10.0"' "$(head -1 "$tmp/docs/kmd.1" | grep -o '"kmd [^"]*"')" "sync: man page"
eq "9.10.0 2030-02-03,9.9.0 2030-01-09,9.8.8 2030-01-05,9.8.7 2030-01-02," \
  "$(sed -n -E 's/.*<release version="([^"]*)" date="([^"]*)".*/\1 \2/p' "$meta" | head -4 | tr '\n' ,)" "sync: metainfo history"
if (cd "$tmp" && scripts/sync-version.sh 9.9.1 2030-02-04 >/dev/null 2>&1); then eq rejected accepted "sync older version"; fi

# CHANGELOG.md: Keep a Changelog layout, its links up to the spec's version
cur=$(sed -n -E 's/^Version:[[:space:]]+//p' packaging/kmd.spec)
grep -qx '## \[Unreleased\]' CHANGELOG.md || eq present missing "CHANGELOG.md: ## [Unreleased]"
grep -qx "\[Unreleased\]: https://github.com/DipaolaFederico/kmd/compare/v$cur...HEAD" CHANGELOG.md ||
  eq present missing "CHANGELOG.md: [Unreleased] link from v$cur"
grep -q "^## \[$cur\] - " CHANGELOG.md || eq present missing "CHANGELOG.md: ## [$cur] section"
for v in $(sed -n -E 's/^## \[([0-9]+\.[0-9]+\.[0-9]+)\] - [0-9]{4}-[0-9]{2}-[0-9]{2}$/\1/p' CHANGELOG.md); do
  grep -q "^\[$v\]: https://github.com/DipaolaFederico/kmd/" CHANGELOG.md || eq present missing "CHANGELOG.md: [$v] link"
done
bad=$(grep -E '^## ' CHANGELOG.md | grep -v -x -E -e '## \[Unreleased\]' -e '## \[[0-9]+\.[0-9]+\.[0-9]+\] - [0-9]{4}-[0-9]{2}-[0-9]{2}' || true)
eq "" "$bad" "CHANGELOG.md: version headings"

# gen-emoji.py: the committed emoji table matches its Unicode-name source
if command -v python3 >/dev/null && command -v gofmt >/dev/null; then
  # a Python whose Unicode database is too old exits 3: skip the check
  if python3 -I scripts/gen-emoji.py >"$tmp/emoji_raw.go"; then
    gofmt <"$tmp/emoji_raw.go" >"$tmp/emoji_table.go"
    cmp -s "$tmp/emoji_table.go" internal/md/emoji_table.go ||
      eq current stale "internal/md/emoji_table.go (run: python3 -I scripts/gen-emoji.py | gofmt > internal/md/emoji_table.go)"
  elif [ $? -ne 3 ]; then
    eq ok failed "scripts/gen-emoji.py"
  fi
fi

# complexity.sh, in a scratch repository with a stub gocognit that prints
# the scores of the base tree or of the working tree
cx=$(mktemp -d)
mkdir -p "$cx/repo/scripts"
cp scripts/complexity.sh "$cx/repo/scripts/"
cat >"$cx/gocognit" <<'STUB'
#!/usr/bin/env bash
case $PWD in */base) cat "$STUB/before" ;; *) cat "$STUB/after" ;; esac
STUB
chmod +x "$cx/gocognit"
(
  cd "$cx/repo"
  git init -q -b main . && git config user.email t@t && git config user.name t
  printf 'package a\n' >a.go && git add a.go scripts && git commit -qm base
  git switch -qc work
)
export STUB=$cx
cx_run() { # before after → output and status of complexity.sh
  printf '%b' "$1" >"$cx/before"
  printf '%b' "$2" >"$cx/after"
  "$cx/repo/scripts/complexity.sh" "$cx/gocognit" 30 2>&1 && echo "rc=0" || echo "rc=$?"
}
out=$(cx_run '' '45 a (*t) New a.go:3:1\n')
eq "  a (*t) New a.go: cognitive complexity 45 (new), limit 30|rc=1" "$(echo "$out" | grep -v '^complexity:' | paste -sd'|')" "complexity: new function"
out=$(cx_run '40 a Old a.go:3:1\n' '41 a Old a.go:9:1\n')
eq "  a Old a.go: cognitive complexity 41 (was 40), limit 30|rc=1" "$(echo "$out" | grep -v '^complexity:' | paste -sd'|')" "complexity: grew (line moved)"
eq rc=0 "$(cx_run '40 a Old a.go:3:1\n' '40 a Old a.go:3:1\n12 a Small a.go:20:1\n')" "complexity: unchanged above the limit"
eq rc=0 "$(cx_run '40 a Old a.go:3:1\n' '25 a Old a.go:3:1\n')" "complexity: now below the limit"
( cd "$cx/repo" && git branch -qm main old )
eq "complexity: no main branch to compare with (shallow clone?): not checked|rc=0" \
  "$(cx_run '' '45 a New a.go:3:1\n' | paste -sd'|')" "complexity: no base"
eq rc=1 "$(STRICT=1 cx_run '' '45 a New a.go:3:1\n' | tail -1)" "complexity: no base, STRICT"
unset STUB
rm -rf "$cx"

if [ "$fails" -gt 0 ]; then
  echo "$fails script test(s) failed"
  exit 1
fi
echo "  scripts: ok"
