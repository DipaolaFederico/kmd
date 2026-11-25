//go:build kmdai && unix

package ai

import (
	"path/filepath"
	"syscall"
	"testing"
)

// A FIFO is refused before it is opened: opening it would wait for a writer.
func TestReadKeyFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no FIFOs here:", err)
	}
	if key, err := readKey(fifo); err == nil {
		t.Errorf("readKey(FIFO) = %q, want an error", key)
	}
}
