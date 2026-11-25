package cli

import (
	"errors"
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/view"
)

// Speaker notes: kmd --slides publishes the slide on screen in a state
// file, in a directory only the user can open, and kmd --notes, in another
// terminal, follows it and shows that slide's <!-- notes: … -->.

// notesPoll is how often the notes window checks the slide on screen.
const notesPoll = 100 * time.Millisecond

// notesDir returns the private directory of the state files, creating it:
// $XDG_RUNTIME_DIR/kmd, or kmd-<uid> in the temporary directory.
func notesDir(getenv func(string) string) (string, error) {
	dir := filepath.Join(os.TempDir(), "kmd-"+strconv.Itoa(os.Getuid()))
	if x := getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(x) {
		dir = filepath.Join(x, "kmd")
	}
	return dir, privateDir(dir)
}

// stateFile names the state file of the deck at path.
func stateFile(dir, path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(path))
	return filepath.Join(dir, "slide-"+strconv.FormatUint(h.Sum64(), 16))
}

// writeSlide records the slide on screen, replacing the state file at once
// so a reader never sees half of it.
func writeSlide(path string, slide int) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".slide-*") // mode 0600
	if err != nil {
		return err
	}
	_, err = f.WriteString(strconv.Itoa(slide) + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		_ = os.Remove(f.Name())
	}
	return err
}

// readSlide returns the slide recorded in the state file at path.
func readSlide(path string) (int, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	if !st.Mode().IsRegular() {
		return 0, errors.New("not a regular file")
	}
	// non-blocking: a FIFO put there must not hang the notes window
	// #nosec G304 -- path is stateFile's, in the directory privateDir checked
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 32))
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || n < 0 {
		return 0, errors.New("bad state file")
	}
	return n, nil
}

// publishSlides makes the presentation of the deck at name tell its notes
// window the slide on screen; the returned function removes the state.
// Without a private directory there is no notes window, and nothing else
// changes.
func publishSlides(env Env, name string, opt *view.Options) (done func()) {
	if name == "-" {
		return func() {}
	}
	dir, err := notesDir(env.Getenv)
	if err != nil {
		return func() {}
	}
	path, last := stateFile(dir, name), -1
	opt.OnSlide = func(slide int) {
		if slide != last && writeSlide(path, slide) == nil {
			last = slide
		}
	}
	return func() { _ = os.Remove(path) }
}

// runNotes shows the speaker notes of the slide the presentation of the
// same deck has on screen (--notes), following it.
func runNotes(env Env, o options, s settings) error {
	name := o.files[0]
	dir, err := notesDir(env.Getenv)
	if err != nil {
		return err
	}
	st := &notesState{path: stateFile(dir, name)}
	st.changed()
	in := newSources(name)
	load := func(path string, cols int) (*view.Page, error) {
		if path != name {
			return nil, errors.New("documents do not open in the notes")
		}
		vs := viewerSettings(s, max(cols, 1))
		slides, _, base, err := readDeck(env, name, o.section, vs, in)
		if err != nil {
			return nil, err
		}
		vs.opts = vs.docOptions(name, base)
		return renderedPage(render.Render(notesPage(slides, st.slide, st.live, s.mdOpt), vs.opts)), nil
	}
	poll := env.Poll
	if poll <= 0 {
		poll = notesPoll
	}
	sel, match, bar := render.ViewerStyles(s.opts.Theme, s.opts.Color, s.opts.TrueColor)
	opt := view.Options{Load: load, Start: name, Mouse: s.mouse, Watch: true, Poll: poll, Changed: st.changed,
		Select: sel, Match: match, Bar: bar}
	if env.View != nil {
		return env.View(opt)
	}
	return view.Run(opt)
}

// notesState follows the slide a presentation has on screen.
type notesState struct {
	path  string // the state file
	slide int    // the last slide seen (0 before any)
	live  bool   // a presentation is publishing it
}

// changed reads the state file and reports whether the slide or the
// presentation's presence changed. When the presentation ends, the notes
// stay on its last slide.
func (n *notesState) changed() bool {
	slide, err := readSlide(n.path)
	live := err == nil
	if !live {
		slide = n.slide
	}
	if slide == n.slide && live == n.live {
		return false
	}
	n.slide, n.live = slide, live
	return true
}

// notesPage builds the notes window's document for slide i of slides: its
// number, its notes and the title of the next one. Without a presentation
// running (live false) it shows the first slide's.
func notesPage(slides []*md.Node, i int, live bool, opt md.Options) *md.Node {
	i = min(max(i, 0), len(slides)-1)
	doc := &md.Node{Type: md.Document}
	title := "Slide " + strconv.Itoa(i+1) + " / " + strconv.Itoa(len(slides))
	if !live {
		title += " (waiting for kmd --slides)"
	}
	doc.AppendChild(textBlock(&md.Node{Type: md.Heading, Level: 2}, title))
	if notes := md.Notes(slides[i]); notes != "" {
		for _, c := range md.Parse(notes, opt).Children() {
			doc.AppendChild(c)
		}
	} else {
		doc.AppendChild(textBlock(&md.Node{Type: md.Paragraph}, "No notes."))
	}
	if i+1 < len(slides) {
		doc.AppendChild(&md.Node{Type: md.ThematicBreak})
		doc.AppendChild(textBlock(&md.Node{Type: md.Paragraph}, "Next: "+slideTitle(slides[i+1], i+2)))
	}
	return doc
}

// textBlock gives block n the plain text s (never parsed as Markdown).
func textBlock(n *md.Node, s string) *md.Node {
	n.AppendChild(&md.Node{Type: md.Text, Literal: s})
	return n
}

// slideTitle names a slide by its first heading, or by its number n.
func slideTitle(slide *md.Node, n int) string {
	for c := slide.FirstChild; c != nil; c = c.Next {
		if c.Type == md.Heading {
			return md.PlainText(c)
		}
	}
	return "slide " + strconv.Itoa(n)
}
