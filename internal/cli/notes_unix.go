//go:build unix

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// privateDir creates dir for the user alone, or checks that an existing
// one is theirs and closed to everyone else: in a shared temporary
// directory, another user could create it first to feed or read the state.
func privateDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !st.IsDir() || !ok || int(sys.Uid) != os.Getuid() || st.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s: not a private directory (owned by you, mode 0700); set XDG_RUNTIME_DIR to one", dir)
	}
	return nil
}
