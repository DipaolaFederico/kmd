package md

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// HTMLOptions controls HTML output.
type HTMLOptions struct {
	// TagFilter escapes GFM-disallowed raw HTML tags (<script>, <iframe>, …).
	TagFilter bool
	// Unsafe passes raw HTML through and keeps every URL. By default raw
	// HTML is omitted and javascript:, vbscript:, file: and non-image data:
	// URLs are dropped, as in cmark's safe mode: documents are untrusted.
	Unsafe bool
	// BaseURL, when set, is the absolute URL relative link destinations
	// resolve against (see ResolveLink). Images are left as written: a
	// browser would download them.
	BaseURL *url.URL
}

// MaxURL is the longest link target kmd resolves or emits as a hyperlink:
// a hostile document may use one destination thousands of times.
const MaxURL = 2048

// ResolveLink resolves a relative link destination against base, an
// absolute URL, as a browser would. In-document anchors ("#x"),
// destinations with a scheme and ones that do not parse as a URL are
// returned unchanged, so the result never has a scheme other than base's
// unless dest already had it; so are ones longer than MaxURL. Nothing is
// fetched.
func ResolveLink(base *url.URL, dest string) string {
	if base == nil || dest == "" || dest[0] == '#' || len(dest) > MaxURL {
		return dest
	}
	u, err := url.Parse(dest)
	if err != nil || u.Scheme != "" {
		return dest
	}
	return base.ResolveReference(u).String()
}

// unsafeURL reports URLs that can run code or read local files in a browser.
func unsafeURL(u string) bool {
	l := strings.ToLower(strings.TrimLeft(u, " \t\n\r\f\v"))
	for _, p := range [...]string{"data:image/png", "data:image/gif", "data:image/jpeg", "data:image/webp"} {
		if strings.HasPrefix(l, p) {
			return false
		}
	}
	return strings.HasPrefix(l, "javascript:") || strings.HasPrefix(l, "vbscript:") ||
		strings.HasPrefix(l, "file:") || strings.HasPrefix(l, "data:")
}

func (w *htmlWriter) url(u string) string {
	if !w.opt.Unsafe && unsafeURL(u) {
		return ""
	}
	return esc(u)
}

type htmlWriter struct {
	b       strings.Builder
	opt     HTMLOptions
	disable int // >0 while rendering image alt text
	doc     *Node
}

// HTML renders a document as HTML in the reference implementation's format.
func HTML(doc *Node, opt HTMLOptions) string {
	w := &htmlWriter{opt: opt, doc: doc}
	w.render(doc)
	w.footnoteSection()
	return w.b.String()
}

var xmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

// esc escapes text and attribute values. It also drops C0 controls other
// than tab and newlines, DEL and C1 controls: they are invalid in HTML and,
// should the page reach a terminal (kmd --html x.md), could drive it.
func esc(s string) string {
	if hasControl(s) {
		s = strings.Map(func(r rune) rune {
			if isControl(r) {
				return -1
			}
			return r
		}, s)
	}
	if !strings.ContainsAny(s, `&<>"`) {
		return s
	}
	return xmlEscaper.Replace(s)
}

func isControl(r rune) bool {
	return r < 0x20 && r != '\t' && r != '\n' && r != '\r' || r >= 0x7f && r <= 0x9f
}

// hasControl reports whether s holds a character isControl drops; C1
// controls are encoded in UTF-8 as 0xC2 0x80–0x9F.
func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		switch b := s[i]; {
		case b < 0x20 && b != '\t' && b != '\n' && b != '\r', b == 0x7f:
			return true
		case b == 0xc2 && i+1 < len(s) && s[i+1] >= 0x80 && s[i+1] <= 0x9f:
			return true
		}
	}
	return false
}

func (w *htmlWriter) lit(s string) { w.b.WriteString(s) }

func (w *htmlWriter) cr() {
	if n := w.b.Len(); n > 0 && w.b.String()[n-1] != '\n' {
		w.b.WriteByte('\n')
	}
}

func (w *htmlWriter) tag(s string) {
	if w.disable == 0 {
		w.b.WriteString(s)
	}
}

var filteredTags = [...]string{"title", "textarea", "style", "xmp", "iframe", "noembed", "noframes", "script", "plaintext"}

// tagFilter escapes the '<' of disallowed tags (GFM "tagfilter" extension).
func tagFilter(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '<' {
			j := i + 1
			if j < len(s) && s[j] == '/' {
				j++
			}
			for _, t := range filteredTags {
				if hasPrefixFold(s[j:], t) {
					k := j + len(t)
					if k == len(s) || isLineSpace(s[k]) || s[k] == '>' || strings.HasPrefix(s[k:], "/>") {
						b.WriteString("&lt;")
						goto next
					}
				}
			}
		}
		b.WriteByte(s[i])
	next:
	}
	return b.String()
}

func (w *htmlWriter) raw(s string) {
	switch {
	case w.disable > 0: // inside image alt text: never emit markup
		w.lit(esc(s))
		return
	case !w.opt.Unsafe:
		w.lit("<!-- raw HTML omitted -->")
		return
	}
	if w.opt.TagFilter {
		s = tagFilter(s)
	}
	w.lit(s)
}

func inTightList(n *Node) bool {
	gp := n.Parent
	if gp != nil {
		gp = gp.Parent
	}
	return gp != nil && gp.Type == List && gp.Tight
}

func (w *htmlWriter) children(n *Node) {
	for c := n.FirstChild; c != nil; c = c.Next {
		w.render(c)
	}
}

func taskBox(t Task) string {
	if t == TaskDone {
		return `<input checked="" disabled="" type="checkbox"> `
	}
	return `<input disabled="" type="checkbox"> `
}

func (w *htmlWriter) render(n *Node) {
	switch n.Type {
	case Document:
		w.children(n)
	case Paragraph:
		task := ""
		if it := n.Parent; it != nil && it.Type == Item && it.Task != TaskNone && it.FirstChild == n {
			task = taskBox(it.Task)
		}
		if inTightList(n) {
			w.lit(task)
			w.children(n)
			return
		}
		w.cr()
		w.lit("<p>" + task)
		w.children(n)
		w.lit("</p>")
		w.cr()
	case Heading:
		h := strconv.Itoa(n.Level)
		w.cr()
		w.lit("<h" + h + ">")
		w.children(n)
		w.lit("</h" + h + ">")
		w.cr()
	case BlockQuote:
		w.cr()
		if n.Alert != "" {
			kind := strings.ToLower(n.Alert)
			w.lit(`<div class="markdown-alert markdown-alert-` + kind + `">`)
			w.cr()
			w.lit(`<p class="markdown-alert-title">` + n.Alert[:1] + kind[1:] + "</p>")
			w.cr()
			w.children(n)
			w.cr()
			w.lit("</div>")
			w.cr()
			return
		}
		w.lit("<blockquote>")
		w.cr()
		w.children(n)
		w.cr()
		w.lit("</blockquote>")
		w.cr()
	case List:
		tag := "ul"
		attrs := ""
		if n.Ordered {
			tag = "ol"
			if n.Start != 1 {
				attrs = ` start="` + strconv.Itoa(n.Start) + `"`
			}
		}
		w.cr()
		w.lit("<" + tag + attrs + ">")
		w.cr()
		w.children(n)
		w.cr()
		w.lit("</" + tag + ">")
		w.cr()
	case Item:
		w.lit("<li>")
		if n.Task != TaskNone && (n.FirstChild == nil || n.FirstChild.Type != Paragraph) {
			w.lit(taskBox(n.Task))
		}
		w.children(n)
		w.lit("</li>")
		w.cr()
	case CodeBlock:
		w.cr()
		w.lit("<pre><code")
		if f := strings.Fields(n.Info); len(f) > 0 {
			w.lit(` class="language-` + esc(f[0]) + `"`)
		}
		w.lit(">" + esc(n.Literal) + "</code></pre>")
		w.cr()
	case MathBlock:
		w.cr()
		w.lit(`<div class="math math-display">` + esc(n.Literal) + "</div>")
		w.cr()
	case HTMLBlock:
		w.cr()
		w.raw(n.Literal)
		w.cr()
	case ThematicBreak:
		w.cr()
		w.lit("<hr />")
		w.cr()
	case Table:
		w.table(n)
	case FootnoteDef, FrontMatter:
		// footnotes go to the footnote section; front matter is metadata
	case Text:
		w.lit(esc(n.Literal))
	case SoftBreak:
		w.lit("\n")
	case LineBreak:
		w.tag("<br />")
		w.cr()
	case Code:
		w.tag("<code>")
		w.lit(esc(n.Literal))
		w.tag("</code>")
	case Math:
		cls := "math math-inline"
		if n.Display {
			cls = "math math-display"
		}
		w.tag(`<span class="` + cls + `">`)
		w.lit(esc(n.Literal))
		w.tag("</span>")
	case Emph, Strong, Strike:
		t := map[NodeType]string{Emph: "em", Strong: "strong", Strike: "del"}[n.Type]
		w.tag("<" + t + ">")
		w.children(n)
		w.tag("</" + t + ">")
	case Link:
		a := `<a href="` + w.url(ResolveLink(w.opt.BaseURL, n.Dest)) + `"`
		if n.Title != "" {
			a += ` title="` + esc(n.Title) + `"`
		}
		w.tag(a + ">")
		w.children(n)
		w.tag("</a>")
	case Image:
		if w.disable == 0 {
			w.lit(`<img src="` + w.url(n.Dest) + `" alt="`)
		}
		w.disable++
		w.children(n)
		w.disable--
		if w.disable == 0 {
			if n.Title != "" {
				w.lit(`" title="` + esc(n.Title))
			}
			w.lit(`" />`)
		}
	case HTMLInline:
		w.raw(n.Literal)
	case FootnoteRef:
		id := strconv.Itoa(n.Index)
		w.tag(`<sup class="footnote-ref"><a href="#fn-` + esc(n.Label) + `" id="fnref-` + esc(n.Label) + `" data-footnote-ref>` + id + "</a></sup>")
	}
}

func (w *htmlWriter) table(t *Node) {
	w.cr()
	w.lit("<table>\n<thead>\n")
	body := false
	for row := t.FirstChild; row != nil; row = row.Next {
		if !row.Header && !body {
			w.lit("<tbody>\n")
			body = true
		}
		w.lit("<tr>\n")
		cell := "td"
		if row.Header {
			cell = "th"
		}
		for c := row.FirstChild; c != nil; c = c.Next {
			align := ""
			switch c.Align {
			case AlignLeft:
				align = ` align="left"`
			case AlignCenter:
				align = ` align="center"`
			case AlignRight:
				align = ` align="right"`
			}
			w.lit("<" + cell + align + ">")
			w.children(c)
			w.lit("</" + cell + ">\n")
		}
		w.lit("</tr>\n")
		if row.Header {
			w.lit("</thead>\n")
		}
	}
	if body {
		w.lit("</tbody>\n")
	}
	w.lit("</table>\n")
}

// Footnotes returns the referenced footnote definitions in number order.
func Footnotes(doc *Node) []*Node {
	var defs []*Node
	Walk(doc, func(n *Node, entering bool) bool {
		if entering && n.Type == FootnoteDef && n.Index > 0 {
			defs = append(defs, n)
		}
		return n.Type == Document || !n.Type.IsBlock() || entering
	})
	slices.SortFunc(defs, func(a, b *Node) int { return a.Index - b.Index })
	return defs
}

func (w *htmlWriter) footnoteSection() {
	defs := Footnotes(w.doc)
	if len(defs) == 0 {
		return
	}
	w.cr()
	w.lit("<section class=\"footnotes\" data-footnotes>\n<ol>\n")
	for _, d := range defs {
		w.lit(`<li id="fn-` + esc(d.Label) + "\">\n")
		w.children(d)
		w.cr()
		w.lit(`<a href="#fnref-` + esc(d.Label) + `" class="footnote-backref" data-footnote-backref>↩</a>` + "\n</li>\n")
	}
	w.lit("</ol>\n</section>\n")
}
