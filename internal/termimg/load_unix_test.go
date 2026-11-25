//go:build linux || darwin || freebsd || netbsd || openbsd

package termimg

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLoadFIFO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "fifo.png")
	if err := syscall.Mkfifo(p, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := new(Loader).Load(p, "")
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, ErrNotRegular) {
			t.Fatalf("got %v; want ErrNotRegular", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Load blocked on a FIFO")
	}
}
