//go:build !(linux || darwin || freebsd || netbsd || openbsd)

// Package term queries terminal properties using only the standard library.
package term

import (
	"errors"
	"time"
)

// Size reports no terminal on unsupported platforms.
func Size(fd uintptr) (cols, rows int, ok bool) { return 0, 0, false }

// PixelSize reports an unknown pixel size on unsupported platforms.
func PixelSize(fd uintptr) (w, h int, ok bool) { return 0, 0, false }

// MakeRaw is not supported on this platform.
func MakeRaw(fd uintptr) (restore func() error, err error) {
	return nil, errors.ErrUnsupported
}

// LightBackground cannot ask the terminal on unsupported platforms.
func LightBackground(path string, timeout time.Duration) (light, ok bool) { return false, false }
