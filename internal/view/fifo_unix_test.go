//go:build linux || darwin || freebsd || netbsd || openbsd

package view

import "syscall"

func syscallMkfifo(path string) error { return syscall.Mkfifo(path, 0o600) }
