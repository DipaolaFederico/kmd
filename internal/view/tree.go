package view

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// pickLimits bound the file picker's walk, which lists every Markdown
// document under a folder in the background (see task): a folder link in
// an untrusted document can point at "/".
type pickLimits struct {
	depth   int // folder levels below the root
	files   int // documents listed
	entries int // directory entries read in total
	links   int // symlinked documents checked
	name    int // bytes of a listed path (filtering cost per keystroke)
}

// defaultPickLimits are the viewer's limits. Tests pass lower ones to walkTree
// rather than change these, which walks still running would read.
var defaultPickLimits = pickLimits{depth: 20, files: 10000, entries: 200_000, links: 1000, name: 512}

// skipDirs are never walked, like hidden folders and files (.git…).
var skipDirs = map[string]bool{"node_modules": true, "vendor": true}

// doc is a document found by the walk.
type doc struct {
	path  string // absolute path, to open it
	name  string // path relative to the root, slash-separated and sanitized
	lower string // name lowercased (ASCII), for the substring bonus
	link  bool   // a symlink (checked again before it is read)
}

// newDoc makes the doc of path, relative name rel.
func newDoc(path, rel string, link bool) doc {
	name := render.Clean(filepath.ToSlash(rel))
	return doc{path: path, name: name, lower: string(lowerASCII(nil, []byte(name))), link: link}
}

// walkTree lists the Markdown documents under root, breadth first (so a
// truncated list keeps the shallow documents), calling emit with batches
// of them; emit returns false to stop. It skips hidden files and folders
// and skipDirs, stays on the root's file system (no automounted or network
// shares), never enters a symlinked folder (no loops, no escaping the
// root), and lists a symlinked document only if it points directly at a
// regular Markdown file inside the root (see linkInside). The result
// explains any limit of lim that was reached.
func walkTree(root string, lim pickLimits, stopped func() bool, emit func([]doc) bool) string {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return errText(err)
	}
	st, err := os.Stat(rootAbs)
	if err != nil {
		return errText(err)
	}
	w := &walker{root: rootAbs, lim: lim, stopped: stopped}
	w.dev, w.hasDev = device(st)
	w.queue = []walkDir{{rootAbs, 0}}
	for first := true; len(w.queue) > 0 && !w.done(); first = false {
		d := w.queue[0]
		w.queue = w.queue[1:]
		open := openDir
		if first {
			open = os.Open // the root itself may be a symlink
		}
		if !w.visit(d, open) {
			return "" // stopped
		}
		if len(w.batch) >= 256 && !w.flush(emit) {
			return ""
		}
	}
	if !w.flush(emit) {
		return ""
	}
	return w.notes()
}

type walkDir struct {
	path  string
	depth int
}

// walker is the state of a walk: what is left to visit, what was found
// and which limits were reached.
type walker struct {
	root    string
	lim     pickLimits
	dev     uint64
	hasDev  bool
	stopped func() bool
	queue   []walkDir
	batch   []doc

	files, entries, links         int
	deep, mounts, long, manyLinks bool
}

// done reports whether a limit stops the walk.
func (w *walker) done() bool { return w.files >= w.lim.files || w.entries >= w.lim.entries }

func (w *walker) flush(emit func([]doc) bool) bool {
	ok := len(w.batch) == 0 || emit(w.batch)
	w.batch = nil
	return ok
}

// visit reads folder d, queuing its subfolders and collecting its
// documents; false when the walk was stopped.
func (w *walker) visit(d walkDir, open func(string) (*os.File, error)) bool {
	f, err := open(d.path)
	if err != nil {
		return true // unreadable, or swapped for a symlink: skip it
	}
	ents, err := w.read(f)
	_ = f.Close()
	if err != nil {
		return true
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
	for _, e := range ents {
		if w.stopped() {
			return false
		}
		if w.add(d, e); w.files >= w.lim.files {
			break
		}
	}
	return true
}

// read lists an open folder within the entry budget, if it is a folder
// on the root's file system.
func (w *walker) read(f *os.File) ([]os.DirEntry, error) {
	fi, err := f.Stat()
	if err != nil || !fi.IsDir() {
		return nil, os.ErrInvalid
	}
	if dev, ok := device(fi); w.hasDev && ok && dev != w.dev {
		w.mounts = true
		return nil, os.ErrInvalid
	}
	ents, _ := f.ReadDir(w.lim.entries - w.entries)
	w.entries += len(ents)
	return ents, nil
}

// add handles entry e of folder d.
func (w *walker) add(d walkDir, e os.DirEntry) {
	n := e.Name()
	if strings.HasPrefix(n, ".") {
		return
	}
	p := filepath.Join(d.path, n)
	t := e.Type()
	link := false
	switch {
	case t.IsDir():
		switch {
		case skipDirs[n]:
		case d.depth >= w.lim.depth:
			w.deep = true
		default:
			w.queue = append(w.queue, walkDir{p, d.depth + 1})
		}
		return
	case !render.MarkdownExt[strings.ToLower(filepath.Ext(n))]:
		return
	case t&os.ModeSymlink != 0:
		if w.links++; w.links > w.lim.links {
			w.manyLinks = true
			return
		}
		if !linkInside(p, w.root) {
			return
		}
		link = true
	case !t.IsRegular():
		return
	}
	rel, err := filepath.Rel(w.root, p)
	switch {
	case err != nil:
	case len(rel) > w.lim.name:
		w.long = true
	default:
		w.batch = append(w.batch, newDoc(p, rel, link))
		w.files++
	}
}

// notes explains the limits that were reached.
func (w *walker) notes() string {
	var notes []string
	note := func(cond bool, s string, n int) {
		if cond {
			notes = append(notes, strings.Replace(s, "N", strconv.Itoa(n), 1))
		}
	}
	note(w.files >= w.lim.files, "only the first N documents", w.lim.files)
	note(w.entries >= w.lim.entries, "stopped after reading N entries", w.lim.entries)
	note(w.deep, "folders deeper than N levels skipped", w.lim.depth)
	note(w.manyLinks, "only the first N symlinked documents checked", w.lim.links)
	note(w.mounts, "other file systems skipped", 0)
	note(w.long, "paths longer than N bytes skipped", w.lim.name)
	return strings.Join(notes, " · ")
}

// maxLinkTarget bounds the target of a symlinked document: checking it
// costs one lstat per path component.
const maxLinkTarget = 1024

// linkInside reports whether the symlink p (under root, reached without
// following symlinked folders) points directly at a regular file inside
// root with the same extension: one hop (no symlink chains), through real
// folders only. It resolves the target itself, with one lstat per
// component of a short target, rather than with filepath.EvalSymlinks,
// whose cost a hostile link chain can multiply (255 hops of 4 KiB paths).
func linkInside(p, root string) bool {
	target, err := os.Readlink(p)
	if err != nil || len(target) > maxLinkTarget {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(p), target)
	}
	target = filepath.Clean(target)
	if !strings.EqualFold(filepath.Ext(target), filepath.Ext(p)) {
		return false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	cur := root
	parts := strings.Split(rel, string(filepath.Separator))
	for i, c := range parts {
		cur = filepath.Join(cur, c)
		st, err := os.Lstat(cur)
		if err != nil {
			return false
		}
		if i == len(parts)-1 {
			return st.Mode().IsRegular()
		}
		if !st.IsDir() { // a symlinked folder on the way could lead anywhere
			return false
		}
	}
	return false
}
