//go:build linux || darwin || freebsd || netbsd || openbsd

package view

import (
	"os"
	"syscall"
)

// openDir opens a folder found by the walk without following a symlink
// swapped in since it was listed, and without blocking on a FIFO.
func openDir(path string) (*os.File, error) {
	// #nosec G304 -- a folder under the root the user opened, listed by the walk.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}

// openDoc opens a listed document without blocking on a FIFO swapped in
// since; follow is false for a document that was not a symlink.
func openDoc(path string, follow bool) (*os.File, error) {
	flags := os.O_RDONLY | syscall.O_NONBLOCK
	if !follow {
		flags |= syscall.O_NOFOLLOW
	}
	// #nosec G304 -- a document listed by the walk; links are checked again by the caller.
	return os.OpenFile(path, flags, 0)
}

// device identifies the file system of fi (the walk stays on the root's).
func device(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true // #nosec G115 -- an identifier, only compared.
}
