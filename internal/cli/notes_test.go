//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DipaolaFederico/kmd/internal/view"
)

func TestNotesDir(t *testing.T) {
	run := t.TempDir()
	dir, err := notesDir(envMap(map[string]string{"XDG_RUNTIME_DIR": run}))
	if err != nil || dir != filepath.Join(run, "kmd") {
		t.Fatalf("dir %q: %v", dir, err)
	}
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("mode: %v %v", st.Mode(), err)
	}
	if _, err := notesDir(envMap(map[string]string{"XDG_RUNTIME_DIR": run})); err != nil {
		t.Errorf("existing private dir refused: %v", err)
	}
	// a relative XDG_RUNTIME_DIR is ignored (temporary directory instead)
	t.Setenv("TMPDIR", t.TempDir())
	if dir, err := notesDir(envMap(map[string]string{"XDG_RUNTIME_DIR": "rel"})); err != nil || !strings.HasPrefix(filepath.Base(dir), "kmd-") {
		t.Errorf("fallback %q: %v", dir, err)
	}
	// a directory others can open, or a link to one, is refused
	open := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(open, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o770); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{open, link, filepath.Join(t.TempDir(), "missing", "x")} {
		if err := privateDir(d); err == nil {
			t.Errorf("%s accepted", d)
		}
	}
}

func TestPublishSlides(t *testing.T) {
	runDir := t.TempDir()
	env := Env{Getenv: envMap(map[string]string{"XDG_RUNTIME_DIR": runDir})}
	var opt view.Options
	done := publishSlides(env, "deck.md", &opt)
	state := stateFile(filepath.Join(runDir, "kmd"), "deck.md")
	opt.OnSlide(3)
	if n, err := readSlide(state); n != 3 || err != nil {
		t.Errorf("published: %d %v", n, err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	opt.OnSlide(3) // the same slide again (a step, a reload): not written
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("same slide written again: %v", err)
	}
	opt.OnSlide(4)
	done()
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("state left: %v", err)
	}
	// standard input has no notes window and touches nothing
	var none view.Options
	publishSlides(Env{Getenv: envMap(map[string]string{"XDG_RUNTIME_DIR": filepath.Join(runDir, "missing")})}, "-", &none)()
	if none.OnSlide != nil {
		t.Error("stdin published")
	}
}

func TestSlideState(t *testing.T) {
	dir := t.TempDir()
	path := stateFile(dir, "deck.md")
	if abs, _ := filepath.Abs("deck.md"); path != stateFile(dir, abs) || path == stateFile(dir, "other.md") {
		t.Errorf("state file names: %s", path)
	}
	if _, err := readSlide(path); err == nil {
		t.Error("missing state read")
	}
	if err := writeSlide(path, 7); err != nil {
		t.Fatal(err)
	}
	if n, err := readSlide(path); n != 7 || err != nil {
		t.Errorf("read %d %v", n, err)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
	if m, _ := filepath.Glob(filepath.Join(dir, ".slide-*")); len(m) != 0 {
		t.Errorf("temporary files left: %v", m)
	}
	for _, content := range []string{"x\n", "-1\n", strings.Repeat("9", 100)} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := readSlide(path); err == nil {
			t.Errorf("%q accepted", content)
		}
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readSlide(link); err == nil {
		t.Error("symlink followed")
	}
	if err := writeSlide(filepath.Join(dir, "missing", "s"), 1); err == nil {
		t.Error("write into a missing directory")
	}
}

func TestNotesWindow(t *testing.T) {
	deck := writeFile(t, "deck.md", "# One\n\n<!-- notes: say **hello** -->\n\n---\n\n## Two\n\nno notes here\n")
	runDir := t.TempDir()
	var got view.Options
	mod := func(e *Env) {
		e.StdoutTTY, e.Cols, e.Rows = true, 80, 24
		e.Getenv = envMap(map[string]string{"HOME": t.TempDir(), "XDG_RUNTIME_DIR": runDir})
		e.View = func(o view.Options) error { got = o; return nil }
	}
	if r := run(t, mod, "--slides", deck); r.code != 0 || got.OnSlide == nil {
		t.Fatalf("slides: %+v", r)
	}
	state := stateFile(filepath.Join(runDir, "kmd"), deck)
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Errorf("state left after the presentation: %v", err)
	}
	if err := writeSlide(state, 1); err != nil {
		t.Fatal(err)
	}

	if r := run(t, mod, "--notes", deck); r.code != 0 || got.Changed == nil || got.Poll != notesPoll {
		t.Fatalf("notes: %+v", r)
	}
	page := func() string {
		p, err := got.Load(deck, 60)
		if err != nil {
			t.Fatal(err)
		}
		return stripTest(strings.Join(p.Lines, "\n"))
	}
	if s := page(); !strings.Contains(s, "Slide 2 / 2") || !strings.Contains(s, "No notes.") || strings.Contains(s, "Next") {
		t.Errorf("slide 2: %s", s)
	}
	if got.Changed() {
		t.Error("changed without a new slide")
	}
	if err := writeSlide(state, 0); err != nil {
		t.Fatal(err)
	}
	if !got.Changed() {
		t.Error("new slide not noticed")
	}
	if s := page(); !strings.Contains(s, "Slide 1 / 2") || !strings.Contains(s, "say hello") || !strings.Contains(s, "Next: Two") {
		t.Errorf("slide 1: %s", s)
	}
	if err := writeSlide(state, 1); err != nil {
		t.Fatal(err)
	}
	got.Changed()
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	// the presentation ended: the notes stay on its last slide
	if s := page(); !got.Changed() || !strings.Contains(page(), "Slide 2 / 2 (waiting for kmd --slides)") {
		t.Errorf("presentation ended: %s", s)
	}
	if _, err := got.Load("/other.md", 60); err == nil {
		t.Error("another document opened in the notes")
	}

	for _, args := range [][]string{{"--notes"}, {"--notes", "-"}, {"--notes", deck, deck}, {"--notes", "--slides", deck}, {"--notes", "--html", deck}} {
		if r := run(t, mod, args...); r.code != 2 {
			t.Errorf("%v: %+v", args, r)
		}
	}
	if r := run(t, mod, "--notes", "-f", deck); r.code != 0 {
		t.Errorf("-f (the notes always follow the file): %+v", r)
	}
	if r := run(t, nil, "--notes", deck); r.code != 2 || !strings.Contains(r.errOut, "--notes needs a terminal") {
		t.Errorf("not a terminal: %+v", r)
	}
}
