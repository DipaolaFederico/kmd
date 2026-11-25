package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// sources reads the documents of a run. The documents named on the
// command line are the user's choice: their first read may block on a
// pipe (kmd <(command), a FIFO waiting for its writer), before the viewer
// takes the terminal; a pipe, like stdin, is read once and kept. Every
// other read — a reload, a document reached by a link or the file picker —
// must find a regular file or a folder: it is opened without blocking and
// checked on the descriptor, so a FIFO swapped in for the document cannot
// hang kmd with the terminal in raw mode.
type sources struct {
	named map[string]bool   // documents named on the command line
	fresh map[string]bool   // named documents not read yet
	once  map[string]string // stdin and pipes, which can only be read once
}

func newSources(names ...string) *sources {
	s := &sources{named: map[string]bool{}, fresh: map[string]bool{}, once: map[string]string{}}
	for _, n := range names {
		s.named[n], s.fresh[n] = true, true
	}
	return s
}

// read reads a document ("-" is stdin); a folder reads as its listing
// (dir true).
func (s *sources) read(name string, stdin io.Reader) (src string, dir bool, err error) {
	if src, ok := s.once[name]; ok {
		return src, false, nil
	}
	if name == "-" {
		src, err = readAll(stdin, name)
		s.once[name] = src
		return src, false, err
	}
	f, st, err := s.open(name)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = f.Close() }()
	switch {
	case st.IsDir():
		src, err := dirListing(name, f)
		return src, true, err
	case st.Mode().IsRegular() && st.Size() > maxInput:
		return "", false, tooLarge(name)
	}
	if src, err = readAll(f, name); err == nil && !st.Mode().IsRegular() {
		s.once[name] = src // a pipe named on the command line
	}
	return src, false, err
}

// open opens a document: the first read of a named one as asked, any
// other only if it is a regular file or a folder (openChecked).
func (s *sources) open(name string) (*os.File, fs.FileInfo, error) {
	if !s.fresh[name] {
		return openChecked(name, !s.named[name])
	}
	delete(s.fresh, name)
	// #nosec G304 -- reading the file the user asked for is the point of the tool.
	f, err := os.Open(filepath.Clean(name))
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, st, nil
}

// openChecked opens a document without blocking and checks, on the open
// descriptor, that it is a regular file or a folder. A linked document (one
// the viewer reached through a link or the picker, not named by the user)
// must also be what its name says: a symlink to a file of another kind is
// refused, as the viewer's followFile does when the link is followed;
// checking again here, at the open, covers a symlink swapped in since and
// every reload.
func openChecked(name string, linked bool) (*os.File, fs.FileInfo, error) {
	path, flags := filepath.Clean(name), os.O_RDONLY|syscall.O_NONBLOCK
	if linked {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, nil, err
		}
		// the resolved path must not have become a symlink meanwhile
		path, flags = real, flags|noFollow
	}
	// #nosec G304 -- a document the user opened, or linked from one; checked below.
	f, err := os.OpenFile(path, flags, 0)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	switch {
	case err != nil:
	case !st.IsDir() && !st.Mode().IsRegular():
		err = fmt.Errorf("%s: not a regular file", name)
	case linked && !st.IsDir() && !sameKind(name, path):
		err = fmt.Errorf("%s: link points to another kind of file", name)
	}
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, st, nil
}

// sameKind reports whether the files named a and b are of the same kind:
// both Markdown, or with the same extension.
func sameKind(a, b string) bool {
	ea, eb := strings.ToLower(filepath.Ext(a)), strings.ToLower(filepath.Ext(b))
	return ea == eb || render.MarkdownExt[ea] && render.MarkdownExt[eb]
}

// readAll reads a document of at most maxInput bytes.
func readAll(r io.Reader, name string) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxInput+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxInput {
		return "", tooLarge(name)
	}
	return string(data), nil
}

func tooLarge(name string) error {
	return fmt.Errorf("%s: larger than %d MiB", name, maxInput>>20)
}
