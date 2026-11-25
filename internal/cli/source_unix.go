//go:build unix

package cli

import "syscall"

// noFollow makes an open fail on a symbolic link.
const noFollow = syscall.O_NOFOLLOW
