//go:build linux

package term

import (
	"os"
	"syscall"
	"testing"
	"unsafe"
)

// openPty opens a pseudo-terminal master; its window size can be set and
// read back without a controlling terminal.
func openPty(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no pty available: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func setWinsize(t *testing.T, f *os.File, ws winsize) {
	t.Helper()
	// #nosec G103 -- TIOCSWINSZ requires a pointer to a winsize struct.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		t.Skipf("TIOCSWINSZ: %v", errno)
	}
}

func TestSizeAndPixelSizePty(t *testing.T) {
	f := openPty(t)

	setWinsize(t, f, winsize{row: 24, col: 80})
	if c, r, ok := Size(f.Fd()); !ok || c != 80 || r != 24 {
		t.Fatalf("Size = %d,%d,%v; want 80,24,true", c, r, ok)
	}
	if _, _, ok := PixelSize(f.Fd()); ok {
		t.Fatal("PixelSize ok with zero pixel fields")
	}

	setWinsize(t, f, winsize{row: 24, col: 80, x: 800, y: 480})
	if w, h, ok := PixelSize(f.Fd()); !ok || w != 800 || h != 480 {
		t.Fatalf("PixelSize = %d,%d,%v; want 800,480,true", w, h, ok)
	}

	setWinsize(t, f, winsize{})
	if _, _, ok := Size(f.Fd()); ok {
		t.Fatal("Size ok with zero columns")
	}
}
