//go:build !(linux || darwin || freebsd || netbsd || openbsd)

package view

import "os"

// The viewer does not run on these platforms (see run_other.go); the walk
// only needs to compile.

func openDir(path string) (*os.File, error) { return os.Open(path) }

func openDoc(path string, _ bool) (*os.File, error) { return os.Open(path) }

func device(os.FileInfo) (uint64, bool) { return 0, false }
