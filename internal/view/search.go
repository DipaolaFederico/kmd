package view

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/DipaolaFederico/kmd/internal/render"
)

// searchLimits bound a text search across the picker's documents, which
// reads files only when the user asks for it.
type searchLimits struct {
	file     int // bytes read from one document
	total    int // bytes read by one search
	hits     int // matches listed
	fileHits int // matches listed per document
}

// searchLimit are the viewer's limits. Tests pass lower ones to searchDocs
// rather than change these, which searches still running would read.
var searchLimit = searchLimits{file: 16 << 20, total: 256 << 20, hits: 1000, fileHits: 50}

const maxHitText = 300 // bytes kept of a matching line

// hit is a line of a document that matches a text search.
type hit struct {
	doc           doc
	line          int    // 1-based line in the source
	heading, text string // section it is in, and the line (sanitized)
	// section is the index of that heading among the document's headings
	// (-1: before the first), nth the number of earlier hits in it: the
	// viewer opens the hit by position, since a heading text can repeat
	// ("### Fixed" in a changelog)
	section, nth int
}

// searchDocs finds the lines of docs (found under root) containing q,
// ignoring case unless q has an uppercase letter (like / in the viewer).
// It calls emit with each document's hits and progress (documents
// searched); emit returns false to stop. Reads are bounded per document and
// in total; the result explains any limit of lim that was reached.
func searchDocs(docs []doc, root, q string, lim searchLimits, stopped func() bool, emit func(hits []hit, done int) bool) string {
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
		if total >= lim.total {
			return "stopped after reading " + strconv.Itoa(lim.total>>20) + " MiB"
		}
		data := readDoc(d, root, min(lim.file, lim.total-total))
		total += len(data)
		hay := data
		if fold && ascii {
			low = lowerASCII(low[:0], data)
			hay = low
		}
		hits := scanDoc(d, data, hay, needle, fold && !ascii, min(lim.fileHits, lim.hits-nhits))
		nhits += len(hits)
		last := i == len(docs)-1 || nhits >= lim.hits
		if (len(hits) > 0 || i%64 == 63 || last) && !emit(hits, i+1) {
			return ""
		}
		if nhits >= lim.hits {
			return "only the first " + strconv.Itoa(lim.hits) + " matches"
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
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() {
		return nil
	}
	var buf bytes.Buffer
	buf.Grow(int(min(int64(n), st.Size()+bytes.MinRead))) // one allocation if it did not grow
	_, _ = buf.ReadFrom(io.LimitReader(f, int64(n)))
	data := buf.Bytes()
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

// scanDoc returns up to limit lines of data that contain needle. hay is
// data itself or its ASCII-lowered copy (same offsets); foldLines lowers
// each line instead, for a non-ASCII query. Each hit carries its section:
// ATX and setext headings outside fenced code, as the renderer sees them.
func scanDoc(d doc, data, hay, needle []byte, foldLines bool, limit int) []hit {
	var hits []hit
	var low []byte // the line lowered, for a non-ASCII query
	heading, prev := "", []byte(nil)
	section, nth := -1, 0
	fence := byte(0)
	for ln, start := 1, 0; start < len(data) && len(hits) < limit; ln++ {
		end := bytes.IndexByte(data[start:], '\n')
		if end < 0 {
			end = len(data)
		} else {
			end += start
		}
		line := data[start:end]
		t := bytes.TrimLeft(line, " ")
		isHeading := false
		switch {
		case len(line)-len(t) > 3: // indented code: nothing to track
		case fence != 0:
			if bytes.HasPrefix(t, []byte{fence, fence, fence}) {
				fence = 0
			}
		case bytes.HasPrefix(t, []byte("```")) || bytes.HasPrefix(t, []byte("~~~")):
			fence = t[0]
		case len(t) > 0 && t[0] == '#':
			var h string
			if h, isHeading = atxHeading(t); isHeading {
				heading = h
			}
		case len(prev) > 0 && setextUnderline(t):
			heading, isHeading = clipBytes(prev), true
		}
		if isHeading {
			section, nth = section+1, 0
		}
		h := hay[start:end]
		if foldLines {
			low = lowerLine(low[:0], h)
			h = low
		}
		if bytes.Contains(h, needle) {
			hits = append(hits, hit{doc: d, line: ln, heading: heading, text: clipBytes(bytes.TrimSpace(line)),
				section: section, nth: nth})
			nth++
		}
		prev = bytes.TrimSpace(line)
		if fence != 0 {
			prev = nil
		}
		start = end + 1
	}
	return hits
}

// setextUnderline reports whether t (indented at most 3 spaces) underlines
// the previous line as a heading: "===" (level 1) or "---" (level 2).
func setextUnderline(t []byte) bool {
	t = bytes.TrimSpace(t)
	return allSame(t, '=', 1) || allSame(t, '-', 2)
}

// allSame reports whether b is at least n bytes, all c (a setext underline).
func allSame(b []byte, c byte, n int) bool {
	if len(b) < n {
		return false
	}
	for _, x := range b {
		if x != c {
			return false
		}
	}
	return true
}

// lowerLine appends line lowered rune by rune (like strings.ToLower, which
// the query was lowered with) to dst.
func lowerLine(dst, line []byte) []byte {
	for len(line) > 0 {
		r, n := utf8.DecodeRune(line)
		dst = utf8.AppendRune(dst, unicode.ToLower(r))
		line = line[n:]
	}
	return dst
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
