//go:build linux || darwin || freebsd || netbsd || openbsd

package term

import (
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// errNoReply reports a terminal that stayed silent until the deadline.
var errNoReply = errors.New("terminal did not answer")

// LightBackground asks the terminal at path (normally /dev/tty) for its
// background colour. ok is false when the terminal does not say, or could
// not be asked; light reports whether the colour is light (see IsLight).
func LightBackground(path string, timeout time.Duration) (light, ok bool) {
	reply, _ := QueryBackground(path, timeout) // a partial reply may still hold the colour
	r, g, b, ok := ParseBackground(reply)
	return ok && IsLight(r, g, b), ok
}

// QueryBackground writes the background query to the terminal at path and
// returns what it answered, up to maxReply bytes, for ParseBackground. It
// stops at the DA1 reply, after a tenth of a second without input, or at
// the first read past timeout, so it never waits longer than timeout plus a
// tenth of a second. The terminal mode is restored before it returns, and
// also when a signal ends the process meanwhile.
func QueryBackground(path string, timeout time.Duration) ([]byte, error) {
	// #nosec G304 -- path is /dev/tty (a pty in tests), never from a document.
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	fd := f.Fd() // also makes reads blocking, so the termios timeout applies
	if !foreground(fd) {
		// changing the mode of the terminal from a background job would
		// stop kmd with SIGTTOU
		return nil, errors.New("not the terminal's foreground process")
	}
	restore, err := makeRaw(fd, 0, 1) // reads return after 0.1 s of silence
	if err != nil {
		return nil, err
	}
	var once sync.Once
	undo := func() { once.Do(func() { _ = restore() }) }
	defer undo()
	defer restoreOnSignal(undo)()
	if _, err := f.WriteString(backgroundQuery); err != nil {
		return nil, err
	}
	return readReply(int(fd), time.Now().Add(timeout)) // #nosec G115 -- file descriptors fit in an int
}

// readReply reads from a terminal in raw mode with a read timeout until a
// DA1 reply ends the answer, the terminal stays silent, the deadline passes
// or maxReply bytes arrived.
func readReply(fd int, deadline time.Time) ([]byte, error) {
	buf := make([]byte, maxReply)
	n := 0
	for n < len(buf) && !da1Done(buf[:n]) {
		if !time.Now().Before(deadline) {
			return buf[:n], errNoReply
		}
		m, err := syscall.Read(fd, buf[n:])
		switch {
		case errors.Is(err, syscall.EINTR):
			continue
		case err != nil:
			return buf[:n], err
		case m <= 0:
			return buf[:n], errNoReply
		}
		n += m
	}
	return buf[:n], nil
}

// foreground reports whether kmd may change the mode of the terminal fd:
// it is not its controlling terminal, or kmd is in its foreground process
// group.
func foreground(fd uintptr) bool {
	var pgrp int32
	// #nosec G103 -- TIOCGPGRP requires a pointer to a pid_t.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGPGRP), uintptr(unsafe.Pointer(&pgrp)))
	return errno != 0 || int(pgrp) == syscall.Getpgrp()
}

// restoreOnSignal runs restore before a terminating signal ends the process
// (the signal is then delivered again with its default action) and returns
// a function that stops watching. Signals the process ignores stay ignored.
func restoreOnSignal(restore func()) (stop func()) {
	sigs := make(chan os.Signal, 1)
	for _, s := range []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT} {
		if !signal.Ignored(s) {
			signal.Notify(sigs, s)
		}
	}
	done := make(chan struct{})
	go func() {
		select {
		case s := <-sigs:
			die(restore, s)
		case <-done:
			select { // one that arrived just before stop
			case s := <-sigs:
				die(restore, s)
			default:
			}
		}
	}()
	return func() {
		signal.Stop(sigs)
		close(done)
	}
}

// die restores the terminal and lets signal s end the process.
func die(restore func(), s os.Signal) {
	restore()
	signal.Reset(s)
	_ = syscall.Kill(os.Getpid(), s.(syscall.Signal))
}
