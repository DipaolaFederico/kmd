package cli

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// sources reads the documents of a run. The documents named on the
// command line are the user's choice: their first read may block on a
// pipe (kmd <(command), a FIFO waiting for its writer), before the viewer
// takes the terminal; a pipe, like stdin, is read once and its result
// kept. Every other read — a reload, a document reached by a link or the
// file picker — must find a regular file or a folder: it is opened without
// blocking and checked on the descriptor, so a FIFO swapped in for the
// document cannot hang kmd with the terminal in raw mode.
type sources struct {
	origin map[string]origin   // how each document was reached (absent: linked)
	once   map[string]onceRead // stdin and pipes, which can only be read once
}

// origin says how a document was reached, which decides how it is opened.
type origin uint8

const (
	linked     origin = iota // through a link or the picker
	namedFresh               // named on the command line, not read yet
	named                    // named on the command line, read before
)

// onceRead is a read of stdin or a pipe, kept whether or not it failed: they
// cannot be read again, and a second read would return what the first one
// left (the rest of a document that was too large).
type onceRead struct {
	src string
	err error
}

func newSources(names ...string) *sources {
	s := &sources{origin: map[string]origin{}, once: map[string]onceRead{}}
	for _, n := range names {
		s.origin[n] = namedFresh
	}
	return s
}

// read reads a document ("-" is stdin); a folder reads as its listing
// (dir true).
func (s *sources) read(name string, stdin io.Reader) (src string, dir bool, err error) {
	if r, ok := s.once[name]; ok {
		return r.src, false, r.err
	}
	if name == "-" {
		src, err = readAll(stdin, name)
		s.once[name] = onceRead{src, err}
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
	src, err = readAll(f, name)
	if !st.Mode().IsRegular() {
		s.once[name] = onceRead{src, err} // a pipe named on the command line
	}
	return src, false, err
}

// open opens a document: the first read of a named one as asked, any
// other only if it is a regular file or a folder (openChecked).
func (s *sources) open(name string) (*os.File, fs.FileInfo, error) {
	o := s.origin[name]
	if o != namedFresh {
		return openChecked(name, o == linked)
	}
	s.origin[name] = named
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
// descriptor, that it is a regular file or a folder. A document named on
// the command line may be a symlink to any file, on every reload too: the
// user chose it. A linked document (one the viewer reached through a link
// or the picker) must also be what its name says: a symlink to a file of
// another kind is refused (render.SameKind, as the viewer's followFile
// does when the link is followed); checking again here, at the open,
// covers a symlink swapped in since and every reload.
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
	case linked && !st.IsDir() && !render.SameKind(name, path):
		err = fmt.Errorf("%s: link points to another kind of file", name)
	}
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, st, nil
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
