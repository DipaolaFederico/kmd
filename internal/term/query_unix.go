//go:build linux || darwin || freebsd || netbsd || openbsd

package term

import (
	"bytes"
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
// background color. ok is false when the terminal does not say, or could
// not be asked; light reports whether the color is light (see isLight).
// A terminal that stays silent is given up on after silent; one that has
// started to answer is given until maxQueryWait.
func LightBackground(path string, silent time.Duration) (light, ok bool) {
	reply, _ := queryBackground(path, silent) // a partial reply may still hold the color
	r, g, b, ok := parseBackground(reply)
	return ok && isLight(r, g, b), ok
}

// queryBackground writes the background query to the terminal at path and
// returns what it answered, up to maxReply bytes, for parseBackground (see
// readReply for how long it waits). The terminal mode is restored before it
// returns, and also when a signal ends the process meanwhile.
func queryBackground(path string, silent time.Duration) ([]byte, error) {
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
	// watch for signals before the mode changes, so none can leave it raw
	var mu sync.Mutex
	var restore func() error
	undo := func() {
		mu.Lock()
		defer mu.Unlock()
		if restore != nil {
			_ = restore()
			restore = nil
		}
	}
	defer restoreOnSignal(undo)()
	defer undo()
	vtime := tenths(min(silent, maxQueryWait)) // reads return after this long without input
	mu.Lock()
	restore, err = makeRaw(fd, 0, vtime)
	mu.Unlock()
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(backgroundQuery); err != nil {
		return nil, err
	}
	return readReply(fd, vtime, silent)
}

// maxQueryWait bounds the wait for the terminal's answer once it has
// begun: stopping halfway would leave the rest of it in the terminal's
// input, where the shell or the viewer would read it as typed keys.
const maxQueryWait = time.Second

// readReply reads from a terminal in raw mode, whose read timeout is vtime
// tenths of a second, until the DA1 reply ends the answer or maxReply bytes
// arrived. It gives up when no answer began (no ESC arrived) within silent,
// or the answer is still incomplete after maxQueryWait. The timeout has a
// precision of 0.1 s, so either may run up to 50 ms over.
func readReply(fd uintptr, vtime uint8, silent time.Duration) ([]byte, error) {
	start := time.Now()
	buf := make([]byte, maxReply)
	n := 0
	for n < len(buf) && !da1Done(buf[:n]) {
		wait := min(silent, maxQueryWait)
		if bytes.IndexByte(buf[:n], 0x1b) >= 0 { // a key typed meanwhile is no answer
			wait = maxQueryWait
		}
		left := wait - time.Since(start)
		if left < 50*time.Millisecond { // a read would end past wait+50ms
			return buf[:n], errNoReply
		}
		if t := tenths(left); t != vtime {
			if err := setReadTimeout(fd, t); err != nil {
				return buf[:n], err
			}
			vtime = t
		}
		m, err := syscall.Read(int(fd), buf[n:]) // #nosec G115 -- file descriptors fit in an int
		switch {
		case errors.Is(err, syscall.EINTR):
			continue
		case err != nil:
			return buf[:n], err
		case m == 0: // the read timeout expired without input
			return buf[:n], errNoReply
		}
		n += m
	}
	return buf[:n], nil
}

// tenths converts d to a termios read timeout: the nearest tenth of a
// second, 1–255.
func tenths(d time.Duration) uint8 {
	return uint8(min(max((d+50*time.Millisecond)/(100*time.Millisecond), 1), 255)) // #nosec G115 -- clamped to 1–255
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
