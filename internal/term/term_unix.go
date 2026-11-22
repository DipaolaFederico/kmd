//go:build linux || darwin || freebsd || netbsd || openbsd

// Package term queries terminal properties using only the standard library.
package term

import (
	"syscall"
	"unsafe"
)

type winsize struct{ row, col, x, y uint16 }

// getWinsize issues TIOCGWINSZ on fd. ok is false when fd is not a terminal.
func getWinsize(fd uintptr) (ws winsize, ok bool) {
	// #nosec G103 -- TIOCGWINSZ requires a pointer to a winsize struct.
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	return ws, errno == 0
}

// Size returns the columns and rows of the terminal attached to fd.
// ok is false when fd is not a terminal.
func Size(fd uintptr) (cols, rows int, ok bool) {
	ws, ok := getWinsize(fd)
	if !ok || ws.col == 0 {
		return 0, 0, false
	}
	return int(ws.col), int(ws.row), true
}

// PixelSize returns the width and height in pixels of the text area of the
// terminal attached to fd (ws_xpixel/ws_ypixel). Many terminals and most
// multiplexers leave these fields at 0; ok is false in that case and when fd
// is not a terminal.
func PixelSize(fd uintptr) (w, h int, ok bool) {
	ws, ok := getWinsize(fd)
	if !ok || ws.x == 0 || ws.y == 0 {
		return 0, 0, false
	}
	return int(ws.x), int(ws.y), true
}
