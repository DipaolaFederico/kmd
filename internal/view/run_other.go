//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package view

import "errors"

// Supported reports whether the viewer can run on this platform.
const Supported = false

// Run is not available on this platform: kmd falls back to the pager.
func Run(opt Options) error { return errors.ErrUnsupported }
