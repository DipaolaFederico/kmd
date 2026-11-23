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
	err     error // the terminal could not be set up again (after the editor)
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
	s := leaveScreen
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
	t := &tty{f: f, mouse: opt.Mouse}
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

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT,
		syscall.SIGTSTP, os.Interrupt)
	defer signal.Stop(sigs)

	// One read at a time, requested by the loop: while the editor runs no
	// read is pending, so kmd never steals its keystrokes.
	want, input := make(chan struct{}), make(chan []byte)
	go func() {
		buf := make([]byte, 4096)
		for range want {
			n, err := f.Read(buf)
			if err != nil {
				close(input)
				return
			}
			input <- append([]byte(nil), buf[:n]...)
		}
	}()
	defer close(want)

	var tick <-chan time.Time
	if opt.Watch {
		poll := opt.Poll
		if poll <= 0 {
			poll = 250 * time.Millisecond
		}
		m.poll() // remember the current version
		t := time.NewTicker(poll)
		defer t.Stop()
		tick = t.C
	}

	var out strings.Builder
	var settle <-chan time.Time // pending resize: re-render once it settles
	redraw, pending := true, false
	for !m.quit {
		if redraw {
			out.Reset()
			m.draw(&out)
			if _, err := f.WriteString(out.String()); err != nil {
				return err
			}
		}
		if !pending {
			want <- struct{}{}
			pending = true
		}
		redraw = false
		select {
		case f := <-m.bg:
			m.applyBackground(f)
			redraw = m.picking // results of the picker's work: only it shows them
		case <-tick:
			redraw = m.poll()
		case <-settle:
			settle = nil
			if c, r, ok := term.Size(f.Fd()); ok && (c != m.cols || r != m.rows) {
				m.cols, m.rows = c, max(r, 2)
				m.reload()
				_, _ = f.WriteString("\x1b[2J")
				redraw = true
			}
		case b, ok := <-input:
			if !ok {
				return nil
			}
			pending, redraw = false, true
			for _, k := range parseKeys(b) {
				if k.name == "ctrl-z" {
					if err := t.suspend(sigs); err != nil {
						return err
					}
					continue
				}
				m.handle(k)
				if t.err != nil {
					return t.err
				}
			}
		case s := <-sigs:
			if s == syscall.SIGTSTP { // kill -TSTP from outside
				if err := t.suspend(sigs); err != nil {
					return err
				}
				redraw = true
				continue
			}
			if s != syscall.SIGWINCH {
				return nil
			}
			// dragging a window sends a burst of SIGWINCH: re-render (which
			// can take a while on huge documents) once, after it settles
			settle = time.After(80 * time.Millisecond)
		}
	}
	return nil
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
