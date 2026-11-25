#!/usr/bin/env bash
# release-bump.sh — read commit messages from stdin, NUL-separated (as from
# `git log -z --format=%B`), and print the release increment they call for
# under Conventional Commits (https://www.conventionalcommits.org):
#
#   type!: …  or a "BREAKING CHANGE:" footer   → major
#   feat: …                                    → minor
#   fix: … · perf: … · revert: …               → patch
#   build, chore, ci, docs, refactor, style, test → nothing (prints nothing)
#
# The largest increment wins. Messages that are not Conventional Commits are
# reported on stderr and ignored. Carriage returns are dropped: a pull
# request description typed in a browser has CRLF line ends.
#   git log -z --format=%B v0.2.0..main | scripts/release-bump.sh
set -euo pipefail

rank() { case $1 in major) echo 3 ;; minor) echo 2 ;; patch) echo 1 ;; *) echo 0 ;; esac; }
re='^([a-z]+)(\([a-z0-9._/-]+\))?(!)?: [^ ].*$'
best=""
while IFS= read -r -d '' msg || [ -n "$msg" ]; do
  msg=${msg//$'\r'/}
  subject=${msg%%$'\n'*}
  [ -z "$subject" ] && continue
  if [[ ! $subject =~ $re ]]; then
    echo "release-bump: not a Conventional Commit, ignored: $subject" >&2
    continue
  fi
  type=${BASH_REMATCH[1]} bang=${BASH_REMATCH[3]}
  want=""
  case $type in
    feat) want=minor ;;
    fix | perf | revert) want=patch ;;
    build | chore | ci | docs | refactor | style | test) ;;
    *)
      echo "release-bump: unknown type '$type', ignored: $subject" >&2
      msg=""
      continue
      ;;
  esac
  # breaking: "type!:" or a BREAKING CHANGE footer — in the last paragraph
  # only, so a line quoted in the middle of a squash body does not count
  footer=$(awk 'BEGIN{RS=""} {p=$0} END{print p}' <<<"$msg")
  if [ -n "$bang" ] || { [ "$footer" != "$subject" ] && grep -qE '^BREAKING[ -]CHANGE: ' <<<"$footer"; }; then
    want=major
  fi
  [ "$(rank "$want")" -gt "$(rank "$best")" ] && best=$want
  msg=""
done
echo "$best"
