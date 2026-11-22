package render

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// Link is a hyperlink of the rendered document, located on screen so an
// interactive viewer can select and follow it.
type Link struct {
	// Href is the destination with relative paths resolved against
	// Options.BaseDir ("file:///abs/path#frag"); "#frag" and URLs with a
	// scheme are kept as written.
	Href  string
	Spans []Span // one per rendered line the link text occupies
}

// Span is the part of a link drawn on one line of Result.Text.
type Span struct {
	Line  int    // 0-based line of Result.Text
	Col   int    // 0-based display column
	Width int    // display columns
	Text  string // plain text of the span
}

// CodeBlock is a code block of the rendered document, so the viewer can
// copy its source.
type CodeBlock struct {
	Lang       string
	Text       string // source, as written in the document
	Start, End int    // first and last line of its box in Result.Text (-1: not drawn)
}

// nav collects link positions while lines are painted.
type nav struct {
	row   int
	links []Link
	code  []CodeBlock
}

// span records that columns [col, col+w) of the current row belong to link
// id, or, for a negative id, that the row is part of code block -id.
func (n *nav) span(id int32, col, w int, text string) {
	if id < 0 {
		c := &n.code[-id-1]
		if c.Start < 0 {
			c.Start = n.row
		}
		c.End = n.row
		return
	}
	l := &n.links[id-1]
	if k := len(l.Spans) - 1; k >= 0 && l.Spans[k].Line == n.row && l.Spans[k].Col+l.Spans[k].Width == col {
		l.Spans[k].Width += w
		l.Spans[k].Text += text
		return
	}
	l.Spans = append(l.Spans, Span{Line: n.row, Col: col, Width: w, Text: text})
}

// hasScheme reports whether s starts with a URL scheme ("https:", "mailto:").
// One-letter schemes are treated as Windows drive letters, not schemes.
func hasScheme(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		case c == ':':
			return i > 1
		default:
			return false
		}
	}
	return false
}

// href resolves a link destination for hyperlinks and navigation: a relative
// path becomes an absolute file:// URL (terminals cannot resolve relative
// targets, they do not know which directory the document came from).
//
// local reports whether the result is a resolved file:// URL of a Markdown
// or document file (see openable).
func (r *renderer) href(dest string) (h string, local bool) {
	// longer targets are never emitted (safeURL) and would be re-processed
	// for every use of a reference link: a hostile document could make that
	// quadratic
	if len(dest) > maxURL || dest == "" || dest[0] == '#' || hasScheme(dest) || strings.HasPrefix(dest, "//") {
		return dest, false
	}
	p, frag, _ := strings.Cut(dest, "#")
	if q := strings.IndexByte(p, '?'); q >= 0 {
		p = p[:q]
	}
	if strings.IndexByte(dest, '%') >= 0 {
		if u, err := url.PathUnescape(p); err == nil {
			p = u
		}
		if f, err := url.PathUnescape(frag); err == nil {
			frag = f
		}
	}
	if !filepath.IsAbs(p) {
		if r.absDir == "" {
			dir := r.opt.BaseDir
			if dir == "" {
				dir = "."
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return dest, false
			}
			r.absDir = abs
		}
		p = filepath.Join(r.absDir, p)
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p), Fragment: frag}
	ext := strings.ToLower(filepath.Ext(p))
	return u.String(), MarkdownExt[ext] || DocumentExt[ext]
}

// IndexNames are the documents that stand for a folder, in order.
var IndexNames = []string{"README.md", "readme.md", "Readme.md", "README.markdown", "index.md"}

// MarkdownExt are the extensions of documents the viewer opens itself.
var MarkdownExt = map[string]bool{".md": true, ".markdown": true, ".mdown": true, ".mkd": true, ".mkdn": true, ".mdwn": true}

// DocumentExt are local files that may be handed to the desktop (images,
// PDFs, text and data): never programs, scripts or .desktop launchers.
var DocumentExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true,
	".pdf": true, ".txt": true, ".csv": true, ".json": true, ".yaml": true, ".yml": true, ".toml": true,
	".log": true, ".mp4": true, ".webm": true, ".mp3": true, ".ogg": true,
	// not .html or .svg: a browser would run the document author's scripts
}

// openable reports whether a resolved file:// href names a Markdown or
// document file. Only those become absolute OSC 8 targets: a terminal opens
// a clicked file:// link with the desktop, which would run a script or a
// .desktop file (relative targets are inert, terminals cannot resolve them).
func openable(href string) bool {
	u, err := url.Parse(href)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	return MarkdownExt[ext] || DocumentExt[ext]
}

// SafeURL reports whether a link target is free of control and invisible
// characters and of reasonable length (the rules for OSC 8 targets).
func SafeURL(u string) bool { return u != "" && safeURL(u) == u }

// Slug returns the GitHub-style anchor of a heading text: lowercase, with
// punctuation removed and spaces turned into hyphens ("Mapa de la etapa" →
// "mapa-de-la-etapa").
func Slug(text string) string {
	var b strings.Builder
	for _, c := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case c == ' ':
			b.WriteByte('-')
		case c == '-' || c == '_' || unicode.IsLetter(c) || unicode.IsNumber(c) || unicode.Is(unicode.Mn, c):
			b.WriteRune(c)
		}
	}
	return b.String()
}

// anchorSet maps heading slugs to lines, suffixing -1, -2… for repeated
// headings like GitHub does. next remembers the next suffix to try per slug,
// so a changelog repeating "### Fixed" hundreds of times stays linear.
type anchorSet struct {
	lines map[string]int
	next  map[string]int
}

func (a *anchorSet) add(text string, line int) {
	s := Slug(text)
	key := s
	for i := a.next[s]; ; i++ {
		if i > 0 {
			key = s + "-" + strconv.Itoa(i)
		}
		if _, dup := a.lines[key]; !dup {
			a.next[s] = i + 1
			break
		}
	}
	a.lines[key] = line
}

// ViewerStyles returns the SGR sequences the interactive viewer paints the
// selected link, search matches and its status bar with.
func ViewerStyles(th *Theme, color, truecolor bool) (sel, match, bar string) {
	if !color {
		return "\x1b[0;7m", "\x1b[0;1;4m", "\x1b[0;7m"
	}
	if th == nil {
		th = &Dark
	}
	p := &painter{color: true, truecolor: truecolor}
	seq := func(st Style) string {
		var b strings.Builder
		p.sgr(&b, st)
		return b.String()
	}
	return seq(Style{Fg: th.H1Fg, Bg: th.Link, Attr: Bold}),
		seq(Style{Fg: th.H1Fg, Bg: th.H5}),
		seq(Style{Fg: th.Muted, Bg: th.CodeBg})
}
