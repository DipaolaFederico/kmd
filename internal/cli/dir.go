package cli

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// maxDirEntries bounds the entries read from a directory for its listing.
const maxDirEntries = 2000

// dirIndex returns the README or index document of directory dir. Like
// links in the viewer, a symlink must lead to a regular Markdown file (a
// cloned repository's README.md → ~/.ssh/id_rsa is not shown).
func dirIndex(dir string) (string, bool) {
	for _, n := range render.IndexNames {
		p := filepath.Join(dir, n)
		real, err := filepath.EvalSymlinks(p)
		if err != nil {
			continue
		}
		st, err := os.Stat(real)
		if err == nil && st.Mode().IsRegular() && render.MarkdownExt[strings.ToLower(filepath.Ext(real))] {
			return p, true
		}
	}
	return "", false
}

// resolveDirs replaces each directory argument that has a README or index
// document with that document, so it renders with links resolved against
// the directory. Directories without one are listed (see dirListing).
func resolveDirs(files []string) {
	for i, f := range files {
		if f == "-" {
			continue
		}
		if st, err := os.Stat(f); err == nil && st.IsDir() {
			if p, ok := dirIndex(f); ok {
				files[i] = p
			}
		}
	}
}

// dirListing builds a Markdown document listing the subdirectories and
// Markdown files of dir (hidden entries excluded), as links the viewer can
// follow. At most maxDirEntries entries are read.
func dirListing(name string, f *os.File) (string, error) {
	entries, err := f.ReadDir(maxDirEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	more := len(entries) > maxDirEntries
	if more {
		entries = entries[:maxDirEntries]
	}
	var dirs, docs []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			st, err := os.Stat(filepath.Join(name, n))
			if err != nil {
				continue
			}
			isDir = st.IsDir()
		}
		switch {
		case isDir:
			dirs = append(dirs, n)
		case e.Type().IsRegular() || e.Type()&os.ModeSymlink != 0:
			if render.MarkdownExt[strings.ToLower(filepath.Ext(n))] {
				docs = append(docs, n)
			}
		}
	}
	sort.Strings(dirs)
	sort.Strings(docs)
	var b strings.Builder
	title := filepath.Base(filepath.Clean(name))
	if abs, err := filepath.Abs(name); err == nil {
		title = filepath.Base(abs)
	}
	b.WriteString("# " + mdEscape(title) + "/\n\n")
	if len(dirs)+len(docs) == 0 {
		b.WriteString("No Markdown documents or folders here.\n")
	}
	if len(dirs) > 0 {
		b.WriteString("## Folders\n\n")
		for _, d := range dirs {
			fmt.Fprintf(&b, "- [%s/](%s)\n", mdEscape(d), linkDest(d))
		}
		b.WriteString("\n")
	}
	if len(docs) > 0 {
		b.WriteString("## Documents\n\n")
		for _, d := range docs {
			fmt.Fprintf(&b, "- [%s](%s)\n", mdEscape(d), linkDest(d))
		}
		b.WriteString("\n")
	}
	if more {
		b.WriteString("Only the first " + strconv.Itoa(maxDirEntries) + " entries of this folder were read.\n")
	}
	return b.String(), nil
}

func isDir(name string) bool {
	st, err := os.Stat(name)
	return err == nil && st.IsDir()
}

// mdEscape makes a file name literal Markdown text.
func mdEscape(s string) string {
	var b strings.Builder
	for _, r := range render.Clean(s) {
		if strings.ContainsRune("\\`*_{}[]<>()#+-.!|~&$:", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// linkDest is a relative link destination for a file name: percent-encoded
// and prefixed with "./" so a name like "http:x.md" is not read as a URL.
func linkDest(name string) string {
	return "./" + (&url.URL{Path: name}).EscapedPath()
}
