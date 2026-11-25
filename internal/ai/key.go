//go:build kmdai

package ai

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"runtime"
)

// maxKeyFile bounds the key file: a key is a single short line.
const maxKeyFile = 4 << 10

// readKey returns the API key in path, or "" when path is empty or the file
// does not exist (the server needs no key). The key goes into an HTTP
// header and is a secret, so the file must be a small regular file that
// only its owner can read, holding one line of printable ASCII.
func readKey(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	// checked before opening too: opening a FIFO would wait for a writer
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("ai-key-file: %w", err)
	} else if !st.Mode().IsRegular() {
		return "", fmt.Errorf("ai-key-file %s: not a regular file", path)
	}
	f, err := os.Open(path) // #nosec G304 -- the user's configured key file, never a document's
	if err != nil {
		return "", fmt.Errorf("ai-key-file: %w", err)
	}
	defer f.Close()
	// and checked again on what was opened, in case the file was replaced
	if err := checkKeyFile(f); err != nil {
		return "", fmt.Errorf("ai-key-file %s: %w", path, err)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxKeyFile))
	if err != nil {
		return "", fmt.Errorf("ai-key-file: %w", err)
	}
	key := bytes.TrimSuffix(bytes.TrimSuffix(b, []byte("\n")), []byte("\r"))
	if len(key) == 0 || bytes.ContainsFunc(key, func(r rune) bool { return r <= ' ' || r > '~' }) {
		return "", fmt.Errorf("ai-key-file %s: want one line of printable ASCII without spaces", path)
	}
	return string(key), nil
}

func checkKeyFile(f *os.File) error {
	st, err := f.Stat()
	switch {
	case err != nil:
		return err
	case !st.Mode().IsRegular():
		return errors.New("not a regular file")
	case st.Size() > maxKeyFile:
		return fmt.Errorf("larger than %d bytes", maxKeyFile)
	case runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0:
		// Windows has no such permission bits; its ACLs are not checked
		return fmt.Errorf("readable by others (mode %v): chmod 600 it", st.Mode().Perm())
	}
	return nil
}
