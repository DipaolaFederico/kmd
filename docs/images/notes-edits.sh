#!/usr/bin/env bash
# notes-edits.sh FILE — stands in for an editor in tapes/follow.tape: adds
# to FILE in three saves. The waits add up to 5.5 s, which must stay under
# the tape's Sleep so the last save is recorded.
set -eu
file=$1
sleep 2
echo '- [x] Pick a date' >>"$file"
sleep 1.5
cat >>"$file" <<'MD'
- [ ] Book the room

> [!NOTE]
> The **live preview** keeps up as you type.
MD
sleep 2
cat >>"$file" <<'MD'

| Who | Task   |
|-----|--------|
| Ana | slides |
| Ben | demo   |
MD
