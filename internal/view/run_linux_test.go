package view

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// openPTY returns the master side of a new pseudo-terminal and the path of
// its slave side, sized cols x rows.
func openPTY(t *testing.T, cols, rows int) (*os.File, string) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skip("no pseudo-terminals:", err)
	}
	t.Cleanup(func() { _ = master.Close() })
	ioctl := func(req uintptr, arg unsafe.Pointer) {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), req, uintptr(arg)); e != 0 {
			t.Fatal(e)
		}
	}
	var unlock int32
	var n uint32
	ioctl(syscall.TIOCSPTLCK, unsafe.Pointer(&unlock))
	ioctl(syscall.TIOCGPTN, unsafe.Pointer(&n))
	resize(t, master, cols, rows)
	return master, "/dev/pts/" + strconv.Itoa(int(n))
}

func resize(t *testing.T, master *os.File, cols, rows int) {
	ws := [4]uint16{uint16(rows), uint16(cols), 0, 0}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws))); e != 0 {
		t.Fatal(e)
	}
}

// screen accumulates everything the viewer writes.
type screen struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *screen) drain(f *os.File) {
	b := make([]byte, 4096)
	for {
		n, err := f.Read(b)
		s.mu.Lock()
		s.buf.Write(b[:n])
		s.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// waitFor waits until the output contains want, then forgets the output.
func (s *screen) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		out := s.buf.String()
		if strings.Contains(stripEscapes(out), want) {
			s.buf.Reset()
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q", want)
}

func TestRunOnPTY(t *testing.T) {
	dir := fixture(t)
	master, slave := openPTY(t, 80, 24)
	old := ttyPath
	ttyPath = slave
	t.Cleanup(func() { ttyPath = old })
	t.Setenv("VISUAL", "true") // the "editor" exits at once
	t.Setenv("EDITOR", "")
	scr := &screen{}
	go scr.drain(master)

	var opened []string
	done := make(chan error, 1)
	go func() {
		done <- Run(Options{
			Load: loader(filepath.Join(dir, "README.md")), Start: filepath.Join(dir, "README.md"), Mouse: true,
			Open: func(s string) error { opened = append(opened, s); return nil },
		})
	}()
	scr.waitFor(t, "Tab links · ? help")
	send := func(s string) { _, _ = master.WriteString(s) }
	send("\t")
	scr.waitFor(t, "→ one.md")
	send("\r")
	scr.waitFor(t, "README.md › one.md")
	send("\x7f")
	scr.waitFor(t, "→ one.md")
	send("e") // runs $VISUAL, then reloads
	scr.waitFor(t, "→ one.md")

	resize(t, master, 60, 20)
	_ = syscall.Kill(os.Getpid(), syscall.SIGWINCH)
	scr.waitFor(t, "Enter open")

	send("q")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("viewer did not quit")
	}
	scr.waitFor(t, "") // restored: left the alternate screen
}

func TestRunWatch(t *testing.T) {
	dir := fixture(t)
	master, slave := openPTY(t, 80, 24)
	old := ttyPath
	ttyPath = slave
	t.Cleanup(func() { ttyPath = old })
	scr := &screen{}
	go scr.drain(master)
	one := filepath.Join(dir, "one.md")
	done := make(chan error, 1)
	go func() {
		done <- Run(Options{Load: loader(one), Start: one, Watch: true, Poll: 5 * time.Millisecond})
	}()
	scr.waitFor(t, "Back to")
	time.Sleep(20 * time.Millisecond) // let the first poll record the file
	if err := os.WriteFile(one, []byte("# One\n\nlive edit appeared\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scr.waitFor(t, "live edit appeared")
	_, _ = master.WriteString("q")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("viewer did not quit")
	}
}

func TestRunErrors(t *testing.T) {
	old := ttyPath
	t.Cleanup(func() { ttyPath = old })

	ttyPath = filepath.Join(t.TempDir(), "missing")
	if Run(Options{}) == nil {
		t.Error("missing terminal")
	}
	ttyPath = os.DevNull
	if Run(Options{}) == nil {
		t.Error("not a terminal")
	}
	master, slave := openPTY(t, 80, 24)
	go (&screen{}).drain(master)
	ttyPath = slave
	err := Run(Options{Load: func(string, int) (*Page, error) { return nil, os.ErrNotExist }})
	if err == nil {
		t.Error("load error")
	}
	resize(t, master, 10, 2)
	if Run(Options{}) == nil {
		t.Error("tiny terminal")
	}
}

func TestRunSignal(t *testing.T) {
	dir := fixture(t)
	master, slave := openPTY(t, 80, 24)
	old := ttyPath
	ttyPath = slave
	t.Cleanup(func() { ttyPath = old })
	scr := &screen{}
	go scr.drain(master)
	done := make(chan error, 1)
	go func() { done <- Run(Options{Load: loader(filepath.Join(dir, "one.md"))}) }()
	scr.waitFor(t, "help")
	_ = syscall.Kill(os.Getpid(), syscall.SIGHUP)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SIGHUP did not stop the viewer")
	}
}

func TestOpenExternal(t *testing.T) {
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	if err := openExternal("https://example.com"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing opener: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bin, "xdg-open"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := openExternal("https://example.com"); err != nil {
		t.Errorf("opener: %v", err)
	}
}

func TestRunEditor(t *testing.T) {
	tty, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tty.Close() }()
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "true -x")
	if err := runEditor("a.md", tty); err != nil {
		t.Errorf("EDITOR: %v", err)
	}
	t.Setenv("EDITOR", "")
	t.Setenv("PATH", t.TempDir())
	if err := runEditor("a.md", tty); err == nil {
		t.Error("vi fallback should fail with an empty PATH")
	}
}
