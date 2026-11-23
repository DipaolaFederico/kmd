package view

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// The file picker lists every Markdown document under a folder. The walk
// and the text search run in the background (see task) and are bounded:
// a folder link in an untrusted document can point at "/". (Variables so
// tests can lower them.)
var (
	maxPickDepth   = 20      // folder levels below the root
	maxPickFiles   = 10000   // documents listed
	maxPickEntries = 200_000 // directory entries read in total
	maxPickLinks   = 1000    // symlinked documents checked

	maxSearchFile  = 16 << 20  // bytes read from one document
	maxSearchTotal = 256 << 20 // bytes read by one search
	maxHits        = 1000      // matches listed
	maxFileHits    = 50        // matches listed per document
)

const maxHitText = 300 // bytes kept of a matching line

// skipDirs are never walked (besides hidden folders such as .git).
var skipDirs = map[string]bool{"node_modules": true, "vendor": true}

// doc is a document found by the walk.
type doc struct {
	path string // absolute path, to open it
	name string // path relative to the root, slash-separated and sanitized
	link bool   // a symlink (checked again before it is read)
}

// hit is a line of a document that matches a text search.
type hit struct {
	doc           doc
	line          int    // 1-based line in the source
	heading, text string // section it is in, and the line (sanitized)
}

// task is a background job (walk, search) whose results are applied on the
// viewer's loop: the goroutine sends closures to bg, and stops sending once
// the task is cancelled.
type task struct{ stop chan struct{} }

func newTask() *task { return &task{stop: make(chan struct{})} }

func (t *task) cancel() {
	if t != nil {
		select {
		case <-t.stop:
		default:
			close(t.stop)
		}
	}
}

func (t *task) stopped() bool {
	select {
	case <-t.stop:
		return true
	default:
		return false
	}
}

// send hands f to the loop; false when the task was cancelled meanwhile.
func (t *task) send(bg chan<- func(*model), f func(*model)) bool {
	select {
	case bg <- f:
		return true
	case <-t.stop:
		return false
	}
}

// walkTree lists the Markdown documents under root, breadth first (so a
// truncated list keeps the shallow documents), calling emit with batches
// of them; emit returns false to stop. It skips hidden folders and
// skipDirs, stays on the root's file system (no automounted or network
// shares), never enters a symlinked folder (no loops, no escaping the
// root), and lists a symlinked document only if it points directly at a
// regular Markdown file inside the root (see linkInside). The result
// explains any limit that was reached.
func walkTree(root string, stopped func() bool, emit func([]doc) bool) string {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return errText(err)
	}
	st, err := os.Stat(rootAbs)
	if err != nil {
		return errText(err)
	}
	dev, hasDev := device(st)
	type dir struct {
		path  string
		depth int
	}
	queue := []dir{{rootAbs, 0}}
	var batch []doc
	files, entries, links := 0, 0, 0
	var notes []string
	flush := func() bool {
		ok := len(batch) == 0 || emit(batch)
		batch = nil
		return ok
	}
	deep, full, mounts := false, false, false
walk:
	for first := true; len(queue) > 0; first = false {
		d := queue[0]
		queue = queue[1:]
		budget := maxPickEntries - entries
		if budget <= 0 {
			break
		}
		open := openDir
		if first {
			open = os.Open // the root itself may be a symlink
		}
		f, err := open(d.path)
		if err != nil {
			continue // unreadable, or swapped for a symlink: skip it
		}
		if fi, err := f.Stat(); err != nil || !fi.IsDir() {
			_ = f.Close()
			continue
		} else if d2, ok := device(fi); hasDev && ok && d2 != dev {
			_ = f.Close()
			mounts = true
			continue
		}
		ents, _ := f.ReadDir(budget)
		_ = f.Close()
		if entries += len(ents); entries >= maxPickEntries && !full {
			full = true
			notes = append(notes, "stopped after reading "+strconv.Itoa(maxPickEntries)+" entries")
		}
		sort.Slice(ents, func(i, j int) bool { return ents[i].Name() < ents[j].Name() })
		for _, e := range ents {
			if stopped() {
				return ""
			}
			n := e.Name()
			if strings.HasPrefix(n, ".") {
				continue
			}
			p := filepath.Join(d.path, n)
			t := e.Type()
			link := false
			switch {
			case t.IsDir():
				if skipDirs[n] {
					continue
				}
				if d.depth >= maxPickDepth {
					deep = true
					continue
				}
				queue = append(queue, dir{p, d.depth + 1})
				continue
			case !render.MarkdownExt[strings.ToLower(filepath.Ext(n))]:
				continue
			case t&os.ModeSymlink != 0:
				if links++; links > maxPickLinks || !linkInside(p, rootAbs) {
					continue
				}
				link = true
			case !t.IsRegular():
				continue
			}
			rel, err := filepath.Rel(rootAbs, p)
			if err != nil {
				continue
			}
			batch = append(batch, doc{path: p, name: render.Clean(filepath.ToSlash(rel)), link: link})
			if files++; files >= maxPickFiles {
				notes = append(notes, "only the first "+strconv.Itoa(maxPickFiles)+" documents")
				break walk
			}
		}
		if len(batch) >= 256 && !flush() {
			return ""
		}
	}
	if deep {
		notes = append(notes, "folders deeper than "+strconv.Itoa(maxPickDepth)+" levels skipped")
	}
	if links > maxPickLinks {
		notes = append(notes, "only the first "+strconv.Itoa(maxPickLinks)+" symlinked documents checked")
	}
	if mounts {
		notes = append(notes, "other file systems skipped")
	}
	if !flush() {
		return ""
	}
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
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
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

// query is a lowercased filter, as runes (fuzzy matching) and as a string
// (substring bonus).
type query struct {
	r []rune
	s string
}

func newQuery(s string) query {
	s = strings.ToLower(s)
	return query{[]rune(s), s}
}

// fuzzy scores name against the query q: every rune of q must
// appear in name in order, ignoring case. Matches at the start of a path
// segment or word, consecutive matches and matches in the file name score
// higher. The byte offsets of the matches are appended to pos.
func fuzzy(name string, fq query, pos []int) (int, []int, bool) {
	q := fq.r
	if len(q) == 0 {
		return 0, pos, true
	}
	base := strings.LastIndexByte(name, '/') + 1
	score, qi, prevMatch := 0, 0, -2
	prev := '/'
	for i, r := range name {
		lr := r
		switch {
		case r >= 'A' && r <= 'Z':
			lr = r + 'a' - 'A'
		case r >= utf8.RuneSelf:
			lr = unicode.ToLower(r)
		}
		if lr == q[qi] {
			score++
			if prev == '/' || prev == '-' || prev == '_' || prev == '.' || prev == ' ' {
				score += 8
			}
			if prevMatch == i-utf8.RuneLen(prev) {
				score += 4
			}
			if i >= base {
				score += 2
			}
			prevMatch = i
			pos = append(pos, i)
			if qi++; qi == len(q) {
				return score + substringBonus(name, base, fq.s), pos, true
			}
		}
		prev = r
	}
	return 0, pos, false
}

// substringBonus favours names that contain the query as typed, above all
// in the file name and at its start ("guide" → guide.md first).
func substringBonus(name string, base int, q string) int {
	i := indexFold(name, q)
	switch {
	case i < 0:
		return 0
	case i == base:
		return 40
	case i > base:
		return 30
	}
	return 20
}

// indexFold is strings.Index ignoring ASCII case, for a lowercase needle
// (non-ASCII runes must match exactly: this is only a ranking bonus).
func indexFold(s, needle string) int {
	n := len(needle)
	for i := 0; i+n <= len(s); i++ {
		j := 0
		for j < n {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != needle[j] {
				break
			}
			j++
		}
		if j == n {
			return i
		}
	}
	return -1
}

// filterDocs returns the indexes of the documents matching q, best first;
// with an empty query, all of them in order.
func filterDocs(docs []doc, q query, out []int) []int {
	out = out[:0]
	if len(q.r) == 0 {
		for i := range docs {
			out = append(out, i)
		}
		return out
	}
	type scored struct{ i, s int }
	var ms []scored
	var pos []int
	for i, d := range docs {
		var s int
		var ok bool
		if s, pos, ok = fuzzy(d.name, q, pos[:0]); ok {
			ms = append(ms, scored{i, s})
		}
	}
	sort.Slice(ms, func(a, b int) bool {
		x, y := ms[a], ms[b]
		if x.s != y.s {
			return x.s > y.s
		}
		if lx, ly := len(docs[x.i].name), len(docs[y.i].name); lx != ly {
			return lx < ly
		}
		return x.i < y.i
	})
	for _, m := range ms {
		out = append(out, m.i)
	}
	return out
}

// searchDocs finds the lines of docs (found under root) containing q (ignoring case unless q
// has an uppercase letter, like / in the viewer), calling emit with each
// document's hits and progress (documents searched); emit returns false to
// stop. Reads are bounded per document and in total; the result explains
// any limit that was reached.
func searchDocs(docs []doc, root, q string, stopped func() bool, emit func(hits []hit, done int) bool) string {
	fold := strings.ToLower(q) == q
	ascii := true
	for i := 0; i < len(q); i++ {
		ascii = ascii && q[i] < utf8.RuneSelf
	}
	needle := []byte(q)
	var low []byte
	total, nhits := 0, 0
	for i, d := range docs {
		if stopped() {
			return ""
		}
		if total >= maxSearchTotal {
			return "stopped after reading " + strconv.Itoa(maxSearchTotal>>20) + " MiB"
		}
		data := readDoc(d, root, min(maxSearchFile, maxSearchTotal-total))
		total += len(data)
		hay := data
		if fold && ascii {
			low = lowerASCII(low[:0], data)
			hay = low
		}
		hits := scanDoc(d, data, hay, needle, fold && !ascii, min(maxFileHits, maxHits-nhits))
		nhits += len(hits)
		last := i == len(docs)-1 || nhits >= maxHits
		if (len(hits) > 0 || i%64 == 63 || last) && !emit(hits, i+1) {
			return ""
		}
		if nhits >= maxHits {
			return "only the first " + strconv.Itoa(maxHits) + " matches"
		}
	}
	return ""
}

// readDoc reads at most n bytes of a document found by the walk. It may
// have been replaced since: a symlink is checked again, anything else is
// opened without following one, and the open file must be regular (opened
// non-blocking, so a FIFO swapped in cannot hang the search).
func readDoc(d doc, root string, n int) []byte {
	if d.link && !linkInside(d.path, root) {
		return nil
	}
	f, err := openDoc(d.path, d.link)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return nil
	}
	data, _ := io.ReadAll(io.LimitReader(f, int64(n)))
	return data
}

// lowerASCII appends src with ASCII letters lowered: the same length as
// src, so offsets in it are offsets in src.
func lowerASCII(dst, src []byte) []byte {
	for _, c := range src {
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst = append(dst, c)
	}
	return dst
}

// scanDoc returns up to max lines of data containing needle in hay (data
// itself, or its ASCII-lowered copy); foldLines lowers each line instead
// (non-ASCII query). Each hit carries the heading of its section: ATX and
// setext headings outside fenced code.
func scanDoc(d doc, data, hay, needle []byte, foldLines bool, max int) []hit {
	var hits []hit
	heading, prev := "", []byte(nil)
	fence := byte(0)
	for ln, start := 1, 0; start < len(data) && len(hits) < max; ln++ {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			end = len(data)
		} else {
			end += start
		}
		line := data[start:end]
		t := bytes.TrimLeft(line, " ")
		switch {
		case len(line)-len(t) > 3:
		case fence != 0:
			if bytes.HasPrefix(t, []byte{fence, fence, fence}) {
				fence = 0
			}
		case bytes.HasPrefix(t, []byte("```")) || bytes.HasPrefix(t, []byte("~~~")):
			fence = t[0]
		case len(t) > 0 && t[0] == '#':
			if h, ok := atxHeading(t); ok {
				heading = h
			}
		case len(prev) > 0 && len(bytes.TrimSpace(t)) > 0 && strings.Trim(string(bytes.TrimSpace(t)), "=") == "",
			len(prev) > 0 && len(bytes.TrimSpace(t)) > 1 && strings.Trim(string(bytes.TrimSpace(t)), "-") == "":
			heading = clipBytes(bytes.TrimSpace(prev))
		}
		h := hay[start:end]
		found := false
		if foldLines {
			found = strings.Contains(strings.ToLower(string(h)), string(needle))
		} else {
			found = bytes.Contains(h, needle)
		}
		if found {
			hits = append(hits, hit{doc: d, line: ln, heading: heading, text: clipBytes(bytes.TrimSpace(line))})
		}
		prev = bytes.TrimSpace(line)
		if fence != 0 {
			prev = nil
		}
		start = end + 1
	}
	return hits
}

// atxHeading returns the text of an ATX heading line ("## Title ##").
func atxHeading(t []byte) (string, bool) {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n > 6 || n < len(t) && t[n] != ' ' && t[n] != '\t' {
		return "", false
	}
	s := strings.TrimSpace(string(t[n:]))
	if c := strings.TrimRight(s, "#"); c == "" || strings.HasSuffix(c, " ") {
		s = strings.TrimSpace(c)
	}
	return clipBytes([]byte(s)), true
}

// clipBytes sanitizes a line of a document, keeping at most maxHitText
// bytes (on a rune boundary).
func clipBytes(b []byte) string {
	if len(b) > maxHitText {
		n := maxHitText
		for n > 0 && !utf8.RuneStart(b[n]) {
			n--
		}
		b = b[:n]
	}
	return render.Clean(string(b))
}
