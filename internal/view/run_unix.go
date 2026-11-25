//go:build linux || darwin || freebsd || netbsd || openbsd

package view

import (
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/DipaolaFederico/kmd/internal/term"
)

// Supported reports whether the viewer can run on this platform.
const Supported = true

const (
	enterScreen = "\x1b[?1049h\x1b[?25l\x1b[H\x1b[2J"
	leaveScreen = "\x1b[?25h\x1b[?1049l"
	mouseOn     = "\x1b[?1000h\x1b[?1006h"
	mouseOff    = "\x1b[?1000l\x1b[?1006l"
)

// ttyPath is the terminal the viewer runs on (a pty in tests).
var ttyPath = "/dev/tty"

// tty is the controlling terminal in raw mode.
type tty struct {
	f       *os.File
	restore func() error
	mouse   bool
	err     error  // the terminal could not be set up again (after the editor)
	clear   string // removes graphics images the screen switch may keep (kitty)
}

func (t *tty) enter() error {
	restore, err := term.MakeRaw(t.f.Fd())
	if err != nil {
		return err
	}
	t.restore = restore
	s := enterScreen
	if t.mouse {
		s += mouseOn
	}
	if _, err := t.f.WriteString(s); err != nil {
		t.leave() // never leave the terminal raw
		return err
	}
	return nil
}

// suspend stops kmd like Ctrl-Z in a shell, with the terminal restored,
// and sets it up again when the shell resumes it.
func (t *tty) suspend(sigs chan os.Signal) error {
	t.leave()
	signal.Reset(syscall.SIGTSTP) // let the stop signal take its default action
	_ = syscall.Kill(0, syscall.SIGTSTP)
	signal.Notify(sigs, syscall.SIGTSTP)
	return t.enter()
}

func (t *tty) leave() {
	s := t.clear + leaveScreen
	if t.mouse {
		s = mouseOff + s
	}
	_, _ = t.f.WriteString(s)
	if t.restore != nil {
		_ = t.restore()
		t.restore = nil
	}
}

// Run shows the viewer on the controlling terminal (/dev/tty, so it also
// works when the document came from stdin) until the user quits.
func Run(opt Options) error {
	f, err := os.OpenFile(ttyPath, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	cols, rows, ok := term.Size(f.Fd())
	if !ok || rows < 3 || cols < 20 {
		return errors.New("terminal too small")
	}
	t := &tty{f: f, mouse: opt.Mouse, clear: opt.ClearImages}
	if opt.Edit == nil {
		opt.Edit = func(path string) error {
			t.leave()
			defer func() { t.err = t.enter() }()
			return runEditor(path, f)
		}
	}
	if opt.Open == nil {
		opt.Open = openExternal
	}
	if opt.Copy == nil {
		opt.Copy = func(text string) error {
			if c := clipboardCommand(os.Getenv, exec.LookPath, runtime.GOOS); c != nil {
				return runClipboard(c, text)
			}
			_, err := f.WriteString(osc52(text))
			return err
		}
	}
	m, err := newModel(opt, cols, rows)
	if err != nil {
		return err
	}
	if err := t.enter(); err != nil {
		return err
	}
	defer t.leave()
	defer m.stopPicker()

	s := &session{m: m, t: t, sigs: make(chan os.Signal, 1), want: make(chan struct{}), input: make(chan []byte)}
	signal.Notify(s.sigs, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT,
		syscall.SIGTSTP, os.Interrupt)
	defer signal.Stop(s.sigs)
	s.guardLoads()

	// One read at a time, requested by the loop: while the editor runs no
	// read is pending, so kmd never steals its keystrokes.
	go func() {
		buf := make([]byte, 4096)
		for range s.want {
			n, err := f.Read(buf)
			if err != nil {
				close(s.input)
				return
			}
			s.input <- append([]byte(nil), buf[:n]...)
		}
	}()
	defer close(s.want)

	tick, stop := m.ticker()
	defer stop()
	return s.loop(tick)
}

// session is the viewer running on a terminal: the model and the events
// that drive it.
type session struct {
	m     *model
	t     *tty
	sigs  chan os.Signal
	want  chan struct{} // asks the reader for the next input
	input chan []byte   // what the terminal sent
	keys  keyParser

	pending bool             // a read was asked for and has not arrived
	settle  <-chan time.Time // pending resize: re-render once it settles
	late    os.Signal        // a resize or stop that came during a load
	quit    bool             // a quit key or a signal ended kmd during a load
}

// loop draws the screen and applies events until the user quits.
func (s *session) loop(tick <-chan time.Time) error {
	f := s.t.f
	var out strings.Builder
	redraw := true
	for !s.m.quit && !s.quit {
		if redraw {
			out.Reset()
			s.m.draw(&out)
			if _, err := f.WriteString(out.String()); err != nil {
				return err
			}
		}
		s.request()
		redraw = false
		more := true
		var err error
		select {
		case fn := <-s.m.bg:
			s.m.applyBackground(fn)
			redraw = s.m.picking // results of the picker's work: only it shows them
		case <-tick:
			redraw = s.m.tick()
		case <-s.settle:
			s.settle = nil
			redraw = s.resize()
		case b, ok := <-s.input:
			if !ok {
				return nil // the terminal is gone
			}
			s.pending, redraw = false, true
			err = s.handleInput(b)
		case sig := <-s.sigs:
			redraw, more, err = s.signal(sig)
		}
		if sig := s.late; more && err == nil && sig != nil {
			s.late = nil
			var r bool
			r, more, err = s.signal(sig)
			redraw = redraw || r
		}
		if !more || err != nil {
			return err
		}
	}
	return nil
}

// request asks the reader for the next input, unless it was asked already.
func (s *session) request() {
	if !s.pending {
		s.want <- struct{}{}
		s.pending = true
	}
}

// resize renders the page again if the terminal's size changed.
func (s *session) resize() bool {
	c, r, ok := term.Size(s.t.f.Fd())
	if !ok || c == s.m.cols && r == s.m.rows {
		return false
	}
	s.m.cols, s.m.rows = c, max(r, 2)
	s.m.reload()
	_, _ = s.t.f.WriteString("\x1b[2J")
	return true
}

// handleInput applies the keys the terminal sent.
func (s *session) handleInput(b []byte) error {
	for _, k := range s.keys.parse(b) {
		if k.name == "ctrl-z" {
			if err := s.t.suspend(s.sigs); err != nil {
				return err
			}
			s.m.drawn = drawnImages{} // a new screen: draw the images again
			continue
		}
		s.m.handle(k)
		if s.t.err != nil {
			return s.t.err
		}
		if s.quit || s.pending {
			// a load (await) asked for the next input: drop the rest, as
			// keys typed during a load are, so that the editor (e) never
			// runs with a read pending that would steal its keystrokes
			break
		}
	}
	return nil
}

// signal handles a signal; more is false when the viewer must end.
func (s *session) signal(sig os.Signal) (redraw, more bool, err error) {
	switch sig {
	case syscall.SIGTSTP: // kill -TSTP from outside
		if err := s.t.suspend(s.sigs); err != nil {
			return false, false, err
		}
		s.m.drawn = drawnImages{} // a new screen: draw the images again
		return true, true, nil
	case syscall.SIGWINCH:
		// dragging a window sends a burst of SIGWINCH: re-render (which
		// can take a while on huge documents) once, after it settles
		s.settle = time.After(80 * time.Millisecond)
		return false, true, nil
	}
	return false, false, nil
}

// errQuit is the error of a load abandoned because the user quit.
var errQuit = errors.New("quit")

// guardLoads makes the model's loads run off the event loop (await).
func (s *session) guardLoads() {
	if load := s.m.opt.Load; load != nil {
		s.m.opt.Load = func(path string, cols int) (*Page, error) {
			return background(s, func() (*Page, error) { return load(path, cols) })
		}
	}
	if slides := s.m.opt.Slides; slides != nil {
		s.m.opt.Slides = func(cols, rows int) (*Deck, error) {
			return background(s, func() (*Deck, error) { return slides(cols, rows) })
		}
	}
}

// background runs load in a goroutine and awaits it. A panic in load is
// raised again here, on the loop, whose deferred calls restore the
// terminal.
func background[T any](s *session, load func() (T, error)) (T, error) {
	var v T
	var err error
	var panicked any
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { panicked = recover() }()
		v, err = load()
	}()
	if !s.await(done) {
		var zero T
		return zero, errQuit // the load goes on, unread, until kmd exits
	}
	if panicked != nil {
		panic(panicked)
	}
	return v, err
}

// await waits until done is closed, while still answering q, Ctrl-C and
// the signals that end kmd: a load that takes long (a huge document, a
// file on a hung mount) never holds the terminal raw against the user's
// will. Other keys typed meanwhile are dropped; a resize or a stop signal
// is handled once the load is over. false means the viewer must end.
func (s *session) await(done <-chan struct{}) bool {
	for {
		s.request()
		select {
		case <-done:
			return true
		case b, ok := <-s.input:
			s.pending = false
			if !ok || quitKey(s.keys.parse(b)) {
				s.quit = true
				return false
			}
		case sig := <-s.sigs:
			if sig != syscall.SIGWINCH && sig != syscall.SIGTSTP {
				s.quit = true
				return false
			}
			s.late = sig
		}
	}
}

// quitKey reports whether keys hold one that quits.
func quitKey(keys []key) bool {
	for _, k := range keys {
		if isQuit(k.binding()) {
			return true
		}
	}
	return false
}

// runEditor opens path in $VISUAL / $EDITOR (default vi) on the terminal.
func runEditor(path string, tty *os.File) error {
	ed := strings.Fields(os.Getenv("VISUAL"))
	if len(ed) == 0 {
		ed = strings.Fields(os.Getenv("EDITOR"))
	}
	if len(ed) == 0 {
		ed = []string{"vi"}
	}
	// an absolute path cannot be mistaken for an editor option
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	// #nosec G204 G702 -- the editor is chosen by the user ($VISUAL / $EDITOR).
	cmd := exec.Command(ed[0], append(ed[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	return cmd.Run()
}

// openExternal hands target to the desktop's default application. The
// opener runs detached from the terminal and its output is discarded.
func openExternal(target string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	path, err := exec.LookPath(opener)
	if err != nil {
		return errors.New(opener + " not found")
	}
	// #nosec G204 -- fixed program; the target is a single argument, never a shell string.
	cmd := exec.Command(path, target)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// runClipboard sends text to a clipboard program on its standard input.
// The program (wl-copy, xclip…) keeps serving the clipboard in the
// background; its output is discarded and it is detached from the terminal.
func runClipboard(argv []string, text string) error {
	// #nosec G204 -- fixed programs found by clipboardCommand; the text goes to stdin.
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return errors.New(filepath.Base(argv[0]) + " failed")
		}
	case <-time.After(2 * time.Second): // still serving the selection: fine
	}
	return nil
}
