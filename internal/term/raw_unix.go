//go:build linux || darwin || freebsd || netbsd || openbsd

package term

import (
	"syscall"
	"unsafe"
)

// MakeRaw puts the terminal attached to fd in raw mode (no echo, no line
// buffering, no signal keys, no output post-processing) and returns a
// function that restores the previous state.
func MakeRaw(fd uintptr) (restore func() error, err error) {
	return makeRaw(fd, 1, 0)
}

// makeRaw is MakeRaw with the read conditions of non-canonical mode: a read
// returns once vmin bytes arrived, or, with vmin 0, after vtime tenths of a
// second without input.
func makeRaw(fd uintptr, vmin, vtime uint8) (restore func() error, err error) {
	var old syscall.Termios
	if err := termios(fd, ioctlGet, &old); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME] = vmin, vtime
	if err := termios(fd, ioctlSet, &raw); err != nil {
		return nil, err
	}
	return func() error { return termios(fd, ioctlSet, &old) }, nil
}

// setReadTimeout changes the read timeout of a terminal in non-canonical
// mode to vtime tenths of a second.
func setReadTimeout(fd uintptr, vtime uint8) error {
	var t syscall.Termios
	if err := termios(fd, ioctlGet, &t); err != nil {
		return err
	}
	t.Cc[syscall.VTIME] = vtime
	return termios(fd, ioctlSet, &t)
}

func termios(fd, req uintptr, t *syscall.Termios) error {
	// #nosec G103 -- the termios ioctls require a pointer to a Termios struct.
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(unsafe.Pointer(t))); errno != 0 {
		return errno
	}
	return nil
}
