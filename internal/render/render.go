package render

import (
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

// Options controls rendering.
type Options struct {
	Width      int    // total columns available, margins included
	Theme      *Theme // nil means Dark
	Color      bool   // emit SGR color/attribute sequences
	TrueColor  bool   // 24-bit colors instead of the 256 palette
	Hyperlinks bool   // emit OSC 8 hyperlinks

	Images    termimg.Protocol // how to draw images (None: alt text only)
	Cell      termimg.Cell     // terminal cell size in pixels (0 = unknown)
	ImageRows int              // maximum image height in rows (0 = 20)
	BaseDir   string           // directory relative image paths resolve against

	// AnyImagePath lets images be read from anywhere: absolute paths, file:
	// URLs and paths leaving BaseDir. Off, local images are confined to
	// BaseDir (none without it). Only for documents the user chose to open.
	AnyImagePath bool

	Math     bool // render LaTeX math as Unicode
	Diagrams bool // render mermaid / dot code blocks as diagrams

	// Navigate fills Result.Links and Result.Anchors (interactive viewer).
	Navigate bool

	// Budget, when set, is shared by several Render calls that make up
	// one document (the slides of a presentation); nil: a new one.
	Budget *Budget
}

// Budget holds the per-document limits: a short hostile document must not
// be able to trigger unbounded image decoding, diagram layout or math
// layout. Images are decoded once per budget. The zero value is ready.
type Budget struct {
	images    map[string]imageResult    // frames by source and size
	loaded    map[string]*termimg.Image // loaded images by source (nil: failed)
	imgBytes  int
	imgPixels int // source pixels rendered (decoded and scaled)
	specials  int
	mathCount int
}

func budget(o Options) *Budget {
	if o.Budget != nil {
		return o.Budget
	}
	return &Budget{}
}

// Result is the rendered document.
type Result struct {
	Text string
	// Graphics is true when terminal graphics sequences (kitty, iTerm2,
	// sixel) were emitted; such output must not be sent through a pager.
	Graphics bool
	// Links and Anchors locate links and top-level headings (by GitHub
	// slug) in Text; only filled with Options.Navigate.
	Links    []Link
	Anchors  map[string]int
	Headings []Heading   // top-level headings in order (Navigate)
	Code     []CodeBlock // code blocks in order (Navigate)
}

// Heading is a top-level heading of the rendered document.
type Heading struct {
	Level int
	Text  string // plain text, not yet cleaned for the terminal
	Line  int    // 0-based line of Result.Text
}

// maxHeadings bounds the outline kept for the viewer.
const maxHeadings = 10000

const margin = "  "

// minNestWidth is the narrowest content width containers indent to; deeper
// quotes and lists keep this width instead of collapsing to one column.
const minNestWidth = 20

type renderer struct {
	th       *Theme
	opt      Options
	base     Style // inherited inline style (e.g. inside quotes)
	depth    int   // list nesting
	graphics bool
	langs    map[string]*lang // lexers built for this document

	*Budget

	links  *nav   // link registry (Navigate)
	absDir string // absolute BaseDir, resolved once for href
}

const (
	maxLinks      = 100000   // links the viewer can select per document
	maxCodeBlocks = 10000    // code blocks the viewer can copy per document
	maxImages     = 50       // distinct images drawn per document
	maxImageBytes = 64 << 20 // total escape-sequence bytes for images
	// maxImagePixels bounds the source pixels rendered per document: one
	// image shown at many sizes (nested quotes, list depths) is scaled from
	// its full resolution each time. Five of the largest accepted images.
	maxImagePixels = 200_000_000
	maxSpecials    = 100   // diagrams and display formulas per document
	maxInlineMath  = 10000 // inline formulas rendered per document
)

// Render renders a parsed document to terminal text.
func Render(doc *md.Node, o Options) Result {
	if o.Theme == nil {
		o.Theme = &Dark
	}
	r := &renderer{th: o.Theme, opt: o, Budget: budget(o)}
	width := max(o.Width-2*len(margin), 10)
	p := &painter{color: o.Color, truecolor: o.TrueColor, links: o.Hyperlinks}
	var anchors *anchorSet
	if o.Navigate {
		r.links = &nav{row: 1} // Text starts with a blank line
		p.nav = r.links
		anchors = &anchorSet{lines: map[string]int{}, next: map[string]int{}}
	}
	var b strings.Builder
	b.WriteByte('\n')
	blank := func() {
		b.WriteByte('\n')
		if p.nav != nil {
			p.nav.row++
		}
	}
	paint := func(lines []rline) {
		for _, l := range lines {
			if len(l) == 0 {
				blank()
				continue
			}
			b.WriteString(margin)
			p.line(&b, l, len(margin))
		}
	}
	// Top-level blocks are painted as soon as they are laid out, so memory
	// holds the tree and the output, never every styled line at once.
	first := true
	var headings []Heading
	for c := doc.FirstChild; c != nil; c = c.Next {
		if c.Type == md.FootnoteDef {
			continue
		}
		lines := r.block(c, width)
		if len(lines) == 0 {
			continue
		}
		if !first {
			blank()
			if c.Type == md.Heading && c.Level <= 2 {
				blank()
			}
		}
		first = false
		if anchors != nil && c.Type == md.Heading {
			text := md.PlainText(c)
			anchors.add(text, r.links.row)
			if len(headings) < maxHeadings {
				headings = append(headings, Heading{Level: c.Level, Text: text, Line: r.links.row})
			}
		}
		paint(lines)
	}
	if defs := md.Footnotes(doc); len(defs) > 0 {
		paint([]rline{nil, {{text: strings.Repeat("─", min(width, 24)), st: Style{Fg: r.th.Border}}}})
		for _, d := range defs {
			mark := run{text: superscript(d.Index) + " ", st: Style{Fg: r.th.Link, Attr: Bold}}
			body := r.blocks(d, width-strWidth(mark.text), true)
			if len(body) == 0 {
				body = []rline{nil}
			}
			paint(prefixed(body, mark, run{text: strings.Repeat(" ", strWidth(mark.text))}))
		}
	}
	b.WriteByte('\n')
	res := Result{Text: b.String(), Graphics: r.graphics}
	if anchors != nil {
		res.Anchors, res.Headings = anchors.lines, headings
	}
	if r.links != nil {
		for _, c := range r.links.code {
			if c.Start >= 0 { // a block can be laid out and then dropped
				res.Code = append(res.Code, c)
			}
		}
		for _, l := range r.links.links {
			if len(l.Spans) > 0 { // a link can be laid out and then dropped
				res.Links = append(res.Links, l)
			}
		}
	}
	return res
}

// RenderString parses Markdown (with all GFM extensions) and renders it.
func RenderString(src string, o Options) string { return Render(md.Parse(src, md.GFM), o).Text }

func (r *renderer) blocks(parent *md.Node, width int, tight bool) []rline {
	var out []rline
	first := true
	for c := parent.FirstChild; c != nil; c = c.Next {
		if c.Type == md.FootnoteDef {
			continue
		}
		lines := r.block(c, width)
		if len(lines) == 0 {
			continue
		}
		if !first && !tight {
			out = append(out, nil)
			if c.Type == md.Heading && c.Level <= 2 {
				out = append(out, nil)
			}
		}
		first = false
		out = append(out, lines...)
	}
	return out
}

func (r *renderer) inline(n *md.Node, base Style) []run { return r.runs(n, base, nil) }

func (r *renderer) runs(parent *md.Node, base Style, out []run) []run {
	th := r.th
	for n := parent.FirstChild; n != nil; n = n.Next {
		switch n.Type {
		case md.Text:
			out = append(out, run{text: n.Literal, st: base})
		case md.SoftBreak:
			out = append(out, run{text: " ", st: base})
		case md.LineBreak:
			out = append(out, run{text: "\n", st: base})
		case md.Code:
			st := Style{Fg: th.Code, Bg: th.CodeBg, Attr: base.Attr &^ (Italic | Faint), URL: base.URL}
			if r.opt.Color {
				out = append(out, run{text: " " + n.Literal + " ", st: st})
			} else {
				out = append(out, run{text: "`" + n.Literal + "`", st: st})
			}
		case md.Emph:
			st := base
			st.Attr |= Italic
			out = r.runs(n, st, out)
		case md.Strong:
			st := base
			st.Attr |= Bold
			if st.Fg == 0 {
				st.Fg = th.Strong
			}
			out = r.runs(n, st, out)
		case md.Strike:
			st := base
			st.Attr |= Strike | Faint
			out = r.runs(n, st, out)
		case md.Link:
			st := base
			st.Fg, st.URL = th.Link, n.Dest
			href := n.Dest
			if r.opt.Hyperlinks || r.links != nil {
				var local bool
				href, local = r.href(n.Dest)
				switch {
				case local:
					st.URL = href
				case strings.HasPrefix(strings.ToLower(n.Dest), "file:") && !openable(n.Dest):
					// a clicked file:// link is opened by the desktop: never a
					// script or .desktop file
					st.URL = ""
				}
			}
			if r.links != nil {
				if n := len(r.links.links); n < maxLinks {
					r.links.links = append(r.links.links, Link{Href: href})
					st.Link = int32(n + 1)
				}
			}
			st.Attr |= Underline
			out = r.runs(n, st, out)
			if !r.opt.Hyperlinks && n.Dest != "" && n.Dest != md.PlainText(n) &&
				"mailto:"+md.PlainText(n) != n.Dest && "http://"+md.PlainText(n) != n.Dest && !strings.HasPrefix(n.Dest, "#") {
				out = append(out, run{text: " (" + n.Dest + ")", st: Style{Fg: th.Muted, Attr: Faint}})
			}
		case md.Image:
			out = r.imageRef(md.PlainText(n), n.Dest, base, out)
		case md.HTMLInline:
			out = r.htmlInline(n.Literal, base, out)
		case md.Math:
			out = r.mathInline(n.Literal, base, out)
		case md.FootnoteRef:
			out = append(out, run{text: superscript(n.Index), st: Style{Fg: th.Link, Attr: base.Attr | Bold}})
		}
	}
	return out
}

func (r *renderer) imageRef(alt, dest string, base Style, out []run) []run {
	if alt == "" {
		alt = "image"
	}
	st := Style{Fg: r.th.Image, Attr: base.Attr | Italic, URL: dest}
	out = append(out, run{text: "▣ " + alt, st: st})
	if !r.opt.Hyperlinks && dest != "" {
		out = append(out, run{text: " (" + dest + ")", st: Style{Fg: r.th.Muted, Attr: Faint}})
	}
	return out
}

func superscript(n int) string {
	sup := [...]string{"⁰", "¹", "²", "³", "⁴", "⁵", "⁶", "⁷", "⁸", "⁹"}
	var b strings.Builder
	for _, d := range strconv.Itoa(n) {
		b.WriteString(sup[d-'0'])
	}
	return b.String()
}

func (r *renderer) block(n *md.Node, width int) []rline {
	th := r.th
	switch n.Type {
	case md.Paragraph:
		if m := displayMath(n); m != nil && r.opt.Math {
			return r.mathDisplay(m.Literal, width)
		}
		if lines, ok := r.imageParagraph(n, width); ok {
			return lines
		}
		return wrap(r.inline(n, r.base), width)
	case md.Heading:
		return r.heading(n, width)
	case md.CodeBlock:
		lang := ""
		if f := strings.Fields(n.Info); len(f) > 0 {
			lang = strings.ToLower(f[0])
		}
		src := strings.TrimSuffix(n.Literal, "\n")
		if lines, ok := r.special(src, lang, width); ok {
			return lines
		}
		return r.codeBox(src, lang, lang, width)
	case md.MathBlock:
		if r.opt.Math {
			return r.mathDisplay(n.Literal, width)
		}
		return r.codeBox(n.Literal, "math", "math", width)
	case md.HTMLBlock:
		st := r.base
		st.Fg = th.Muted
		rs := r.htmlInline(n.Literal, st, nil)
		if strings.TrimSpace(runsText(rs)) == "" {
			return nil
		}
		return wrap(rs, width)
	case md.FrontMatter:
		return r.codeBox(n.Literal, "yaml", "metadata", width)
	case md.ThematicBreak:
		return []rline{{{text: strings.Repeat("─", width), st: Style{Fg: th.Border}}}}
	case md.BlockQuote:
		return r.quote(n, width)
	case md.List:
		return r.list(n, width)
	case md.Table:
		return r.table(n, width)
	}
	return nil
}

func runsText(rs []run) string {
	var b strings.Builder
	for _, r := range rs {
		b.WriteString(r.text)
	}
	return b.String()
}

// displayMath returns the math node of a paragraph that holds only $$…$$.
func displayMath(p *md.Node) *md.Node {
	var m *md.Node
	for c := p.FirstChild; c != nil; c = c.Next {
		switch {
		case c.Type == md.Math && c.Display && m == nil:
			m = c
		case c.Type == md.Text && strings.TrimSpace(c.Literal) == "", c.Type == md.SoftBreak:
		default:
			return nil
		}
	}
	return m
}

func prefixed(lines []rline, first, rest run) []rline {
	for i, l := range lines {
		p := rest
		if i == 0 {
			p = first
		}
		lines[i] = append(rline{p}, l...)
	}
	return lines
}

func (r *renderer) heading(n *md.Node, width int) []rline {
	th := r.th
	switch n.Level {
	case 1:
		st := Style{Fg: th.H1Fg, Bg: th.H1Bg, Attr: Bold}
		if !r.opt.Color {
			lines := wrap(upper(r.inline(n, st)), width)
			return append(lines, rline{{text: strings.Repeat("═", min(width, max(lineWidth(lines[0]), 1))), st: st}})
		}
		lines := wrap(r.inline(n, st), max(width-2, 1))
		for i, l := range lines {
			pad := run{text: " ", st: st}
			lines[i] = append(append(rline{pad}, l...), pad)
		}
		return lines
	case 2:
		st := Style{Fg: th.H2, Attr: Bold}
		lines := prefixed(wrap(r.inline(n, st), max(width-2, 1)), run{text: "▌ ", st: Style{Fg: th.H2}}, run{text: "  "})
		return append(lines, rline{{text: strings.Repeat("─", width), st: Style{Fg: th.Border}}})
	}
	mark, c := "◆ ", th.H3
	switch n.Level {
	case 4:
		mark, c = "◇ ", th.H4
	case 5, 6:
		mark, c = "· ", th.H5
	}
	st := Style{Fg: c, Attr: Bold}
	return prefixed(wrap(r.inline(n, st), max(width-2, 1)), run{text: mark, st: Style{Fg: c}}, run{text: "  "})
}

func upper(rs []run) []run {
	for i := range rs {
		rs[i].text = strings.ToUpper(rs[i].text)
	}
	return rs
}

// expandTabs replaces tabs with spaces up to the next multiple of 4 columns.
func expandTabs(l string) string {
	if strings.IndexByte(l, '\t') < 0 {
		return l
	}
	var b strings.Builder
	col := 0
	for _, c := range l {
		if c == '\n' {
			col = 0
		}
		if c == '\t' {
			n := 4 - col%4
			b.WriteString("    "[:n])
			col += n
			continue
		}
		b.WriteRune(c)
		col += runeWidth(c)
	}
	return b.String()
}

func (r *renderer) codeBox(text, lang, label string, width int) []rline {
	border := Style{Fg: r.th.Border}
	inner := max(width-4, 1)
	top := rline{{text: "╭─", st: border}}
	used := 2
	if label != "" {
		lbl := " " + label + " "
		if lw := strWidth(lbl); lw <= width-4 {
			top = append(top, run{text: lbl, st: Style{Fg: r.th.Muted, Attr: Italic}})
			used += lw
		}
	}
	top = append(top, run{text: strings.Repeat("─", max(width-used-1, 0)) + "╮", st: border})
	edge := border // top and bottom borders
	if r.links != nil {
		// they carry the block's id, so painting records its first and
		// last line (not the sides: lines are only measured up to their
		// last marked run)
		if n := len(r.links.code); n < maxCodeBlocks {
			r.links.code = append(r.links.code, CodeBlock{Lang: lang, Text: text, Start: -1, End: -1})
			edge.Link = -int32(n + 1)
			top[0].st = edge
		}
	}
	out := []rline{top}
	for _, l := range r.highlight(expandTabs(text), lang) {
		for _, seg := range cut(l, inner) {
			row := append(rline{{text: "│ ", st: border}}, seg...)
			row = append(row, run{text: strings.Repeat(" ", max(inner-lineWidth(seg), 0)+1)}, run{text: "│", st: border})
			out = append(out, row)
		}
	}
	// only the corner is marked: marked runs are measured when painted
	return append(out, rline{{text: "╰", st: edge}, {text: strings.Repeat("─", max(width-2, 0)) + "╯", st: border}})
}

var alertMeta = [...][3]string{
	{"NOTE", "ℹ", "Note"}, {"TIP", "✦", "Tip"}, {"IMPORTANT", "❖", "Important"},
	{"WARNING", "▲", "Warning"}, {"CAUTION", "✖", "Caution"},
}

func (r *renderer) alertColor(a string) Color {
	switch a {
	case "NOTE":
		return r.th.Note
	case "TIP":
		return r.th.Tip
	case "IMPORTANT":
		return r.th.Important
	case "WARNING":
		return r.th.Warning
	}
	return r.th.Caution
}

func (r *renderer) quote(n *md.Node, width int) []rline {
	saved := r.base
	defer func() { r.base = saved }()
	bar := Style{Fg: r.th.QuoteBar}
	var lines []rline
	if n.Alert != "" {
		c := r.alertColor(n.Alert)
		bar.Fg = c
		for _, m := range alertMeta {
			if m[0] == n.Alert {
				lines = append(lines, rline{{text: m[1] + " " + m[2], st: Style{Fg: c, Attr: Bold}}})
			}
		}
	} else {
		r.base.Fg = r.th.Quote
		r.base.Attr |= Italic
	}
	if width-2 < minNestWidth { // too deep: keep the width, drop the bar
		return append(lines, r.blocks(n, width, false)...)
	}
	lines = append(lines, r.blocks(n, width-2, false)...)
	if len(lines) == 0 {
		lines = []rline{nil}
	}
	return prefixed(lines, run{text: "┃ ", st: bar}, run{text: "┃ ", st: bar})
}

var bullets = [...]string{"•", "◦", "▪"}

func (r *renderer) list(n *md.Node, width int) []rline {
	th := r.th
	r.depth++
	defer func() { r.depth-- }()
	count := 0
	for it := n.FirstChild; it != nil; it = it.Next {
		count++
	}
	numW := 0
	if n.Ordered {
		numW = len(strconv.Itoa(n.Start+count-1)) + 1
	}
	var out []rline
	i := 0
	for it := n.FirstChild; it != nil; it = it.Next {
		var mark run
		switch {
		case it.Task == md.TaskDone:
			mark = run{text: "☑", st: Style{Fg: th.TaskDone, Attr: Bold}}
		case it.Task == md.TaskOpen:
			mark = run{text: "☐", st: Style{Fg: th.TaskOpen}}
		case n.Ordered:
			num := strconv.Itoa(n.Start+i) + string(n.Delim)
			mark = run{text: strings.Repeat(" ", max(numW-len(num), 0)) + num, st: Style{Fg: th.Number, Attr: Bold}}
		default:
			mark = run{text: bullets[(r.depth-1)%len(bullets)], st: Style{Fg: th.Bullet}}
		}
		mw := strWidth(mark.text) + 1
		saved := r.base
		if it.Task == md.TaskDone {
			r.base.Attr |= Faint
		}
		inner, indent := width-mw, mw
		if inner < minNestWidth { // too deep: stop indenting further
			inner, indent = width, 0
		}
		lines := r.blocks(it, max(inner, 1), n.Tight)
		r.base = saved
		if len(lines) == 0 {
			lines = []rline{nil}
		}
		if i > 0 && !n.Tight {
			out = append(out, nil)
		}
		mark.text += " "
		if indent == 0 {
			out = append(out, append(rline{mark}, lines[0]...))
			out = append(out, lines[1:]...)
		} else {
			out = append(out, prefixed(lines, mark, run{text: strings.Repeat(" ", indent)})...)
		}
		i++
	}
	return out
}

func (r *renderer) table(n *md.Node, width int) []rline {
	th := r.th
	ncol := len(n.Aligns)
	var cells [][][]run
	widths := make([]int, ncol)
	for row := n.FirstChild; row != nil; row = row.Next {
		base := r.base
		if row.Header {
			base = Style{Fg: th.TableHead, Attr: Bold}
		}
		rc := make([][]run, ncol)
		j := 0
		for c := row.FirstChild; c != nil && j < ncol; c = c.Next {
			rc[j] = r.inline(c, base)
			for _, l := range wrap(rc[j], 1<<20) {
				widths[j] = max(widths[j], lineWidth(l))
			}
			j++
		}
		cells = append(cells, rc)
	}
	avail := width - 3*ncol - 1
	for {
		sum, big := 0, 0
		for j, w := range widths {
			sum += w
			if w > widths[big] {
				big = j
			}
		}
		if sum <= avail || widths[big] <= 4 {
			break
		}
		widths[big]--
	}
	if sum := sumInts(widths); sum > avail {
		if avail < 4*ncol { // too many columns for a grid: one record per row
			return r.stackedTable(cells, width)
		}
		for sum > avail { // shrink the widest columns further, down to 4
			big := 0
			for j, w := range widths {
				if w > widths[big] {
					big = j
				}
			}
			widths[big]--
			sum--
		}
	}
	border := Style{Fg: th.Border}
	rule := func(l, m, rr string) rline {
		var sb strings.Builder
		sb.WriteString(l)
		for j, w := range widths {
			if j > 0 {
				sb.WriteString(m)
			}
			sb.WriteString(strings.Repeat("─", w+2))
		}
		sb.WriteString(rr)
		return rline{{text: sb.String(), st: border}}
	}
	out := []rline{rule("╭", "┬", "╮")}
	header := n.FirstChild != nil && n.FirstChild.Header
	for i, row := range cells {
		wrapped := make([][]rline, ncol)
		h := 1
		for j, rs := range row {
			wrapped[j] = wrap(rs, max(widths[j], 1))
			h = max(h, len(wrapped[j]))
		}
		for k := 0; k < h; k++ {
			line := rline{{text: "│", st: border}}
			for j := range row {
				var cl rline
				if k < len(wrapped[j]) {
					cl = wrapped[j][k]
				}
				pad := max(widths[j]-lineWidth(cl), 0)
				left := 0
				switch n.Aligns[j] {
				case md.AlignRight:
					left = pad
				case md.AlignCenter:
					left = pad / 2
				}
				line = append(line, run{text: strings.Repeat(" ", left+1)})
				line = append(line, cl...)
				line = append(line, run{text: strings.Repeat(" ", pad-left+1)}, run{text: "│", st: border})
			}
			out = append(out, line)
		}
		if i == 0 && header && len(cells) > 1 {
			out = append(out, rule("├", "┼", "┤"))
		}
	}
	return append(out, rule("╰", "┴", "╯"))
}

func sumInts(v []int) int {
	t := 0
	for _, x := range v {
		t += x
	}
	return t
}

// stackedTable renders a table too wide for a grid as "header: value"
// records, one block per body row.
func (r *renderer) stackedTable(cells [][][]run, width int) []rline {
	if len(cells) == 0 {
		return nil
	}
	head := cells[0]
	label := Style{Fg: r.th.TableHead, Attr: Bold}
	sep := rline{{text: strings.Repeat("┄", min(width, 24)), st: Style{Fg: r.th.Border}}}
	var out []rline
	body := cells[1:]
	if len(body) == 0 {
		body = [][][]run{make([][]run, len(head))}
	}
	for i, row := range body {
		if i > 0 {
			out = append(out, sep)
		}
		for j, cell := range row {
			name := strings.TrimSpace(runsText(head[j]))
			if name == "" {
				name = "#" + strconv.Itoa(j+1)
			}
			rs := append([]run{{text: name + ":", st: label}, {text: " "}}, cell...)
			out = append(out, wrap(rs, width)...)
		}
	}
	return out
}

// TOC renders the document outline (headings) as a numbered tree.
func TOC(doc *md.Node, o Options) string {
	if o.Theme == nil {
		o.Theme = &Dark
	}
	r := &renderer{th: o.Theme, opt: o, Budget: budget(o)}
	width := max(o.Width-2*len(margin), 10)
	var counters [7]int
	var stack []int // levels of the currently open headings
	var lines []rline
	colors := [...]Color{r.th.H1Bg, r.th.H2, r.th.H3, r.th.H4, r.th.H5, r.th.H5}
	for c := doc.FirstChild; c != nil; c = c.Next {
		if c.Type != md.Heading {
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1] >= c.Level {
			stack = stack[:len(stack)-1]
		}
		lvl := len(stack) // depth in the outline, independent of skipped levels
		stack = append(stack, c.Level)
		counters[lvl]++
		for k := lvl + 1; k < len(counters); k++ {
			counters[k] = 0
		}
		var num []string
		for k := 0; k <= lvl; k++ {
			num = append(num, strconv.Itoa(counters[k]))
		}
		label := strings.Join(num, ".") + "  "
		indent := strings.Repeat("  ", lvl)
		st := Style{Fg: colors[min(c.Level-1, 5)]}
		if lvl == 0 {
			st.Attr = Bold
		}
		body := wrap(r.inline(c, st), max(width-len(indent)-strWidth(label), 1))
		lines = append(lines, prefixed(body,
			run{text: indent + label, st: Style{Fg: r.th.Muted}},
			run{text: strings.Repeat(" ", len(indent)+strWidth(label))})...)
	}
	p := &painter{color: o.Color, truecolor: o.TrueColor, links: o.Hyperlinks}
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(margin)
		p.line(&b, l, len(margin))
	}
	return b.String()
}
