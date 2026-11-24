//go:build !unix

package cli

import "errors"

// privateDir is not implemented where the viewer does not run.
func privateDir(string) error { return errors.ErrUnsupported }
