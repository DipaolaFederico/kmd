# The viewer: follow a link to another document and back, jump with the
# table of contents, search.
Output docs/images/viewer.gif
Source docs/images/settings.tape
Set Height 540
Set Framerate 10

Type "kmd docs/images/guide/README.md" Sleep 300ms Enter
Sleep 1.5s
Tab Sleep 700ms
Tab Sleep 700ms
Enter Sleep 2s
Backspace Sleep 1.2s
Type "t" Sleep 1s
Down Sleep 400ms Down Sleep 400ms Down Sleep 800ms
Enter Sleep 1.5s
Type "/" Sleep 300ms Type "ground" Sleep 300ms Enter Sleep 1.2s
Type "n" Sleep 1.5s
Type "q"
Sleep 300ms
