//go:build unix

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

// readWithin reads name from s, failing the test if the read blocks.
func readWithin(t *testing.T, s *sources, name string) (string, error) {
	t.Helper()
	type result struct {
		src string
		err error
	}
	done := make(chan result, 1)
	go func() {
		src, _, err := s.read(name, strings.NewReader(""))
		done <- result{src, err}
	}()
	select {
	case r := <-done:
		return r.src, r.err
	case <-time.After(testlimit.Scale() * 5 * time.Second):
		t.Fatalf("reading %s blocked", name)
		return "", nil
	}
}

// A document replaced by a FIFO (kmd -f doc.md; rm doc.md; mkfifo doc.md)
// used to block the reload in open(2), with the viewer's terminal raw.
func TestReloadRefusesFIFO(t *testing.T) {
	dir := t.TempDir()
	doc, linked := filepath.Join(dir, "doc.md"), filepath.Join(dir, "linked.md")
	for _, p := range []string{doc, linked} {
		if err := os.WriteFile(p, []byte("# v1"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s := newSources(doc)
	if src, err := readWithin(t, s, doc); err != nil || src != "# v1" {
		t.Fatalf("first read: %q %v", src, err)
	}
	for _, p := range []string{doc, linked} {
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(p, 0o600); err != nil {
			t.Skip("no FIFOs:", err)
		}
		if _, err := readWithin(t, s, p); err == nil || !strings.Contains(err.Error(), "not a regular file") {
			t.Errorf("%s: FIFO read again: %v", filepath.Base(p), err)
		}
	}
}

// A FIFO or a pipe named on the command line (kmd <(command)) is read the
// first time, waiting for its writer, and kept: it cannot be read twice.
func TestNamedFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "doc.md")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no FIFOs:", err)
	}
	go func() {
		if f, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			_, _ = f.WriteString("# piped")
			_ = f.Close()
		}
	}()
	s := newSources(fifo)
	for range 2 {
		if src, err := readWithin(t, s, fifo); err != nil || src != "# piped" {
			t.Errorf("named FIFO: %q %v", src, err)
		}
	}
}

// A document reached by a link must stay what its name says on every
// read: a symlink swapped in to a file of another kind is refused.
func TestLinkedSymlink(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"secret.txt": "secret", "other.markdown": "# other"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	doc := filepath.Join(dir, "doc.md")
	if err := os.Symlink("other.markdown", doc); err != nil {
		t.Fatal(err)
	}
	s := newSources()
	if src, err := readWithin(t, s, doc); err != nil || src != "# other" {
		t.Errorf("Markdown to Markdown: %q %v", src, err)
	}
	if err := os.Remove(doc); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("secret.txt", doc); err != nil {
		t.Fatal(err)
	}
	if src, err := readWithin(t, s, doc); err == nil {
		t.Errorf("symlink to a .txt read: %q", src)
	}
	// named by the user, it is read: the user chose it
	if src, err := readWithin(t, newSources(doc), doc); err != nil || src != "secret" {
		t.Errorf("named symlink: %q %v", src, err)
	}
	// a linked folder still lists
	if src, err := readWithin(t, s, dir); err != nil || !strings.Contains(src, "other") {
		t.Errorf("linked folder: %q %v", src, err)
	}
}
