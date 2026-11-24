# Live preview: kmd -f re-renders notes.md each time it changes, while
# notes-edits.sh edits a copy of it in the background.
Output docs/images/follow.gif
Source docs/images/settings.tape
Set Height 420
Set Framerate 12

Hide
Type `cd "$HOME" && cp "$OLDPWD/docs/images/notes.md" . && clear` Enter
Type `"$OLDPWD/docs/images/notes-edits.sh" notes.md >/dev/null 2>&1 & clear` Enter
Show

Type "kmd -f notes.md" Enter
# longer than the edits in notes-edits.sh
Sleep 7s
Ctrl+C
Sleep 300ms
