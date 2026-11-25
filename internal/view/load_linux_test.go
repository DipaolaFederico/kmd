package view

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

// ptyViewer runs the viewer with load on a new pseudo-terminal and returns
// its master side, the screen and the channel of Run's result.
func ptyViewer(t *testing.T, load Loader) (*os.File, *screen, chan error) {
	t.Helper()
	master, slave := openPTY(t, 80, 24)
	old := ttyPath
	ttyPath = slave
	t.Cleanup(func() { ttyPath = old })
	scr := &screen{}
	go scr.drain(master)
	done := make(chan error, 1)
	go func() { done <- Run(Options{Load: load}) }()
	scr.waitFor(t, "help")
	return master, scr, done
}

func waitQuit(t *testing.T, done chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(testlimit.Scale() * 5 * time.Second):
		t.Fatal("the viewer did not quit")
	}
}

// blockingLoader loads start the first time; later loads (from the second
// on, or only the second if once) wait until release is closed.
func blockingLoader(start string, once bool, release <-chan struct{}) (Loader, chan struct{}, *atomic.Int32) {
	real := loader(start)
	started := make(chan struct{}, 1)
	calls := &atomic.Int32{}
	return func(path string, cols int) (*Page, error) {
		if n := calls.Add(1); n == 2 || n > 2 && !once {
			started <- struct{}{}
			<-release
		}
		return real(path, cols)
	}, started, calls
}

// A load that does not return (a document on a hung mount, a FIFO, a huge
// render) used to block the event loop with the terminal raw: q, Ctrl-C
// and SIGTERM did nothing. Loads now run off the loop.
func TestRunQuitDuringLoad(t *testing.T) {
	dir := fixture(t)
	for _, tc := range []struct {
		name string
		quit func(master *os.File)
	}{
		{"q", func(m *os.File) { _, _ = m.WriteString("q") }},
		{"ctrl-c", func(m *os.File) { _, _ = m.WriteString("\x03") }},
		{"SIGTERM", func(*os.File) { _ = syscall.Kill(os.Getpid(), syscall.SIGTERM) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			load, started, _ := blockingLoader(filepath.Join(dir, "one.md"), false, release)
			master, scr, done := ptyViewer(t, load)
			_, _ = master.WriteString("r") // reload: blocks
			<-started
			tc.quit(master)
			waitQuit(t, done)
			scr.waitFor(t, "") // restored
		})
	}
}

// Keys typed during a load, other than those that quit, are dropped; a
// resize that came meanwhile is applied once the load is over.
func TestRunKeysDuringLoad(t *testing.T) {
	dir := fixture(t)
	release := make(chan struct{})
	load, started, calls := blockingLoader(filepath.Join(dir, "README.md"), true, release)
	master, scr, done := ptyViewer(t, load)
	_, _ = master.WriteString("r")
	<-started
	_, _ = master.WriteString("?") // dropped: the help does not open
	resize(t, master, 60, 20)
	_ = syscall.Kill(os.Getpid(), syscall.SIGWINCH)
	time.Sleep(50 * time.Millisecond)
	close(release)
	scr.waitFor(t, "Tab links") // the status bar, not the help
	deadline := time.Now().Add(testlimit.Scale() * 5 * time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() < 3 {
		t.Errorf("the resize during the load was not applied: %d loads", calls.Load())
	}
	_, _ = master.WriteString("q")
	waitQuit(t, done)
}
