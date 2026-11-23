//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package view

import "errors"

// syscallMkfifo reports that this platform has no FIFOs; tests that need
// one skip.
func syscallMkfifo(string) error { return errors.ErrUnsupported }
