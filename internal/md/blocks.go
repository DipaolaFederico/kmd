package md

import (
	"bytes"
	"strconv"
	"strings"
)

// maxNest bounds container nesting so adversarial input (thousands of
// nested quotes or lists) cannot make rendering quadratic.
const maxNest = 100

type ref struct{ dest, title string }

type blockParser struct {
	doc, tip, oldtip, lastMatched *Node

	line                                     string
	lineNumber                               int
	offset, column                           int
	nextNonspace, nextNonspaceColumn, indent int
	indented, blank, partiallyConsumedTab    bool
	allClosed                                bool
	consumed                                 bool // the line was used up by a block start

	refs      map[string]ref
	footnotes map[string]*Node
	opt       Options
	cells     int // table cells created so far (bounded by maxTableCells)
	refBudget int // bytes reference links may still copy from definitions
	blocks    int // block nodes created so far (bounded by MaxBlocks)
}

// MaxBlocks bounds the number of block nodes in a document. Real documents
// have a few thousand; a crafted one could have one per 3 bytes of input,
// which would cost gigabytes of tree. Parsing stops at the limit and a
// notice paragraph is appended.
const MaxBlocks = 250000

// Table limits keep a short document from expanding into millions of
// (padding) cells: at most maxTableCols columns and maxTableCells cells.
// A cell costs two nodes (the cell and its text, 640 bytes) and its
// layout: 2¹⁸ cells keep the tree of a table-only document near 170 MB.
const (
	maxTableCols  = 1000
	maxTableCells = 1 << 18
)

// Parse parses Markdown source into a document tree using opt extensions.
func Parse(src string, opt Options) *Node {
	src = strings.ToValidUTF8(src, "�")
	if strings.IndexByte(src, 0) >= 0 {
		src = strings.ReplaceAll(src, "\x00", "�")
	}
	p := &blockParser{opt: opt, refs: map[string]ref{}, footnotes: map[string]*Node{}, refBudget: refBudget(len(src))}
	p.doc = &Node{Type: Document, open: true, startLine: 1}
	p.tip, p.lastMatched = p.doc, p.doc
	if opt.FrontMatter {
		src = p.frontMatter(src)
	}
	for len(src) > 0 && p.blocks < MaxBlocks {
		i := strings.IndexAny(src, "\r\n")
		if i < 0 {
			p.incorporate(src)
			break
		}
		p.incorporate(src[:i])
		if src[i] == '\r' && i+1 < len(src) && src[i+1] == '\n' {
			i++
		}
		src = src[i+1:]
	}
	for p.tip != nil {
		p.finalize(p.tip, p.lineNumber)
	}
	if p.blocks >= MaxBlocks {
		p.doc.AppendChild(&Node{Type: Paragraph, content: []byte("[kmd: document truncated after " +
			strconv.Itoa(MaxBlocks) + " blocks]")})
	}
	p.processInlines()
	return p.doc
}

func peek(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

func (p *blockParser) findNextNonspace() {
	i, cols := p.offset, p.column
	var c byte
	for i < len(p.line) {
		c = p.line[i]
		if c == ' ' {
			i++
			cols++
		} else if c == '\t' {
			i++
			cols += 4 - cols%4
		} else {
			break
		}
	}
	p.blank = i >= len(p.line)
	p.nextNonspace, p.nextNonspaceColumn = i, cols
	p.indent = cols - p.column
	p.indented = p.indent >= 4
}

func (p *blockParser) advanceNextNonspace() {
	p.offset, p.column = p.nextNonspace, p.nextNonspaceColumn
	p.partiallyConsumedTab = false
}

func (p *blockParser) advanceOffset(count int, columns bool) {
	for count > 0 && p.offset < len(p.line) {
		if p.line[p.offset] == '\t' {
			toTab := 4 - p.column%4
			if columns {
				p.partiallyConsumedTab = toTab > count
				adv := min(toTab, count)
				p.column += adv
				if !p.partiallyConsumedTab {
					p.offset++
				}
				count -= adv
			} else {
				p.partiallyConsumedTab = false
				p.column += toTab
				p.offset++
				count--
			}
		} else {
			p.partiallyConsumedTab = false
			p.offset++
			p.column++
			count--
		}
	}
}

func (p *blockParser) addLine() {
	if p.tip.Type == Table {
		p.addTableRow(p.tip, p.line[p.offset:], false)
		return
	}
	if p.partiallyConsumedTab {
		p.offset++
		p.tip.content = append(p.tip.content, "    "[:4-p.column%4]...)
	}
	p.tip.content = append(p.tip.content, p.line[p.offset:]...)
	p.tip.content = append(p.tip.content, '\n')
}

func canContain(parent, child NodeType) bool {
	switch parent {
	case Document, BlockQuote, Item, FootnoteDef:
		return child != Item
	case List:
		return child == Item
	}
	return false
}

func acceptsLines(t NodeType) bool {
	return t == Paragraph || t == CodeBlock || t == HTMLBlock || t == MathBlock || t == Table
}

func depthOf(n *Node) int {
	d := 0
	for ; n != nil; n = n.Parent {
		d++
	}
	return d
}

func (p *blockParser) addChild(t NodeType) *Node {
	for !canContain(p.tip.Type, t) {
		p.finalize(p.tip, p.lineNumber-1)
	}
	p.blocks++
	n := &Node{Type: t, open: true, startLine: p.lineNumber}
	p.tip.AppendChild(n)
	p.tip = n
	return n
}

func (p *blockParser) closeUnmatched() {
	if !p.allClosed {
		for p.oldtip != p.lastMatched {
			parent := p.oldtip.Parent
			p.finalize(p.oldtip, p.lineNumber-1)
			p.oldtip = parent
		}
		p.allClosed = true
	}
}

// maybeSpecial reports whether a line could start a block other than a paragraph.
func (p *blockParser) maybeSpecial(c byte) bool {
	if strings.IndexByte("#`~*+_=<>-", c) >= 0 || isDigit(c) {
		return true
	}
	return (c == '[' && p.opt.Footnotes) || (c == '$' && p.opt.Math) || ((c == '|' || c == ':') && p.opt.Tables)
}

func (p *blockParser) incorporate(ln string) {
	allMatched := true
	container := p.doc
	p.oldtip = p.tip
	p.offset, p.column, p.blank, p.partiallyConsumedTab, p.consumed = 0, 0, false, false, false
	p.lineNumber++
	p.line = ln

	for {
		last := container.LastChild
		if last == nil || !last.open {
			break
		}
		container = last
		p.findNextNonspace()
		switch p.continueBlock(container) {
		case 1:
			allMatched = false
		case 2:
			return
		}
		if !allMatched {
			container = container.Parent
			break
		}
	}
	p.allClosed = container == p.oldtip
	p.lastMatched = container

	matchedLeaf := container.Type != Paragraph && container.Type != Table && acceptsLines(container.Type)
	for !matchedLeaf {
		p.findNextNonspace()
		if !p.indented && !p.maybeSpecial(peek(ln, p.nextNonspace)) {
			p.advanceNextNonspace()
			break
		}
		res := p.tryStarts(container)
		if res == 0 {
			p.advanceNextNonspace()
			break
		}
		container = p.tip
		matchedLeaf = res == 2
	}

	if !p.allClosed && !p.blank && p.tip.Type == Paragraph {
		p.addLine() // lazy paragraph continuation
		return
	}
	p.closeUnmatched()
	if p.consumed {
		return
	}
	if acceptsLines(container.Type) {
		p.addLine()
		if container.Type == HTMLBlock && container.htmlType >= 1 && container.htmlType <= 5 &&
			htmlBlockEnds(container.htmlType, ln[p.offset:]) {
			p.finalize(container, p.lineNumber)
		}
	} else if p.offset < len(ln) && !p.blank {
		p.addChild(Paragraph)
		p.advanceNextNonspace()
		p.addLine()
	}
}

// continueBlock returns 0 if the line continues c, 1 if not, 2 if the line
// was fully consumed (closing code fence or math delimiter).
func (p *blockParser) continueBlock(c *Node) int {
	switch c.Type {
	case BlockQuote:
		if !p.indented && peek(p.line, p.nextNonspace) == '>' {
			p.advanceNextNonspace()
			p.advanceOffset(1, false)
			if isSpaceOrTab(peek(p.line, p.offset)) {
				p.advanceOffset(1, true)
			}
			return 0
		}
		return 1
	case Item:
		if p.blank {
			if c.FirstChild == nil {
				return 1
			}
			p.advanceNextNonspace()
		} else if p.indent >= c.markerOffset+c.padding {
			p.advanceOffset(c.markerOffset+c.padding, true)
		} else {
			return 1
		}
		return 0
	case FootnoteDef:
		if p.blank {
			if c.FirstChild == nil {
				return 1
			}
			p.advanceNextNonspace()
			return 0
		}
		if p.indent >= 4 {
			p.advanceOffset(4, true)
			return 0
		}
		return 1
	case Heading, ThematicBreak:
		return 1
	case CodeBlock:
		if c.Fenced {
			if p.indent <= 3 && peek(p.line, p.nextNonspace) == c.fenceChar &&
				closingFence(p.line[p.nextNonspace:], c.fenceChar) >= c.fenceLen {
				p.finalize(c, p.lineNumber)
				return 2
			}
			for i := c.fenceOffset; i > 0 && isSpaceOrTab(peek(p.line, p.offset)); i-- {
				p.advanceOffset(1, true)
			}
			return 0
		}
		if p.indent >= 4 {
			p.advanceOffset(4, true)
			return 0
		}
		if p.blank {
			p.advanceNextNonspace()
			return 0
		}
		return 1
	case MathBlock:
		if t := trimLineSpace(p.line[p.offset:]); strings.HasSuffix(t, "$$") {
			c.content = append(c.content, t[:len(t)-2]...)
			p.finalize(c, p.lineNumber)
			return 2
		}
		return 0
	case HTMLBlock:
		if p.blank && (c.htmlType == 6 || c.htmlType == 7) {
			return 1
		}
		return 0
	case Paragraph:
		if p.blank {
			return 1
		}
		return 0
	case Table:
		if p.blank || p.cells+len(c.Aligns) > maxTableCells {
			return 1
		}
		return 0
	}
	return 0 // Document, List
}

func closingFence(s string, ch byte) int {
	n := 0
	for n < len(s) && s[n] == ch {
		n++
	}
	for i := n; i < len(s); i++ {
		if !isSpaceOrTab(s[i]) {
			return 0
		}
	}
	return n
}

// tryStarts tries every block start in precedence order: 0 = none,
// 1 = opened a container, 2 = opened a leaf that consumed the line.
func (p *blockParser) tryStarts(container *Node) int {
	ln := p.line
	rest := ln[p.nextNonspace:]
	c := peek(ln, p.nextNonspace)
	nestOK := depthOf(container) < maxNest

	// block quote
	if !p.indented && c == '>' && nestOK {
		p.advanceNextNonspace()
		p.advanceOffset(1, false)
		if isSpaceOrTab(peek(ln, p.offset)) {
			p.advanceOffset(1, true)
		}
		p.closeUnmatched()
		p.addChild(BlockQuote)
		return 1
	}
	// ATX heading
	if !p.indented && c == '#' {
		n := 0
		for n < len(rest) && rest[n] == '#' {
			n++
		}
		if n <= 6 && (n == len(rest) || isSpaceOrTab(rest[n])) {
			p.advanceNextNonspace()
			p.advanceOffset(n, false)
			p.closeUnmatched()
			h := p.addChild(Heading)
			h.Level = n
			h.content = []byte(stripClosingHashes(ln[p.offset:]))
			p.advanceOffset(len(ln)-p.offset, false)
			return 2
		}
	}
	// fenced code
	if !p.indented && (c == '`' || c == '~') {
		n := 0
		for n < len(rest) && rest[n] == c {
			n++
		}
		if n >= 3 && (c == '~' || strings.IndexByte(rest[n:], '`') < 0) {
			p.closeUnmatched()
			b := p.addChild(CodeBlock)
			b.Fenced, b.fenceLen, b.fenceChar, b.fenceOffset = true, n, c, p.indent
			p.advanceNextNonspace()
			p.advanceOffset(n, false)
			return 2
		}
	}
	// display math block
	if p.opt.Math && !p.indented && strings.HasPrefix(rest, "$$") {
		p.closeUnmatched()
		m := p.addChild(MathBlock)
		p.advanceNextNonspace()
		p.advanceOffset(2, false)
		if t := trimLineSpace(ln[p.offset:]); len(t) >= 2 && strings.HasSuffix(t, "$$") {
			m.content = []byte(t[:len(t)-2])
			p.finalize(m, p.lineNumber)
			p.advanceOffset(len(ln)-p.offset, false)
		}
		return 2
	}
	// HTML block
	if !p.indented && c == '<' {
		inPara := container.Type == Paragraph || (!p.allClosed && !p.blank && p.tip.Type == Paragraph)
		if t := htmlBlockStart(rest, inPara); t > 0 {
			p.closeUnmatched()
			b := p.addChild(HTMLBlock)
			b.htmlType = t
			return 2
		}
	}
	// setext heading
	if !p.indented && container.Type == Paragraph && (c == '=' || c == '-') && isSetextLine(rest) {
		p.closeUnmatched()
		content := p.stripRefs(string(container.content))
		if len(content) > 0 {
			h := &Node{Type: Heading, open: true, startLine: container.startLine, content: []byte(content)}
			h.Level = 2
			if c == '=' {
				h.Level = 1
			}
			container.insertAfter(h)
			container.unlink()
			p.tip = h
			p.advanceOffset(len(ln)-p.offset, false)
			return 2
		}
		container.content = []byte(content)
	}
	// thematic break
	if !p.indented && isThematicBreak(rest) {
		p.closeUnmatched()
		p.addChild(ThematicBreak)
		p.advanceOffset(len(ln)-p.offset, false)
		return 2
	}
	// GFM table (the paragraph's last line becomes the header row)
	if p.opt.Tables && !p.indented && container.Type == Paragraph {
		if p.tableStart(container, rest) {
			p.advanceOffset(len(ln)-p.offset, false)
			return 2
		}
	}
	// footnote definition
	if p.opt.Footnotes && !p.indented && nestOK && strings.HasPrefix(rest, "[^") {
		if e := strings.Index(rest, "]:"); e > 2 && !strings.ContainsAny(rest[2:e], " \t]") {
			p.closeUnmatched()
			f := p.addChild(FootnoteDef)
			f.Label = normalizeLabel(rest[2:e])
			if _, dup := p.footnotes[f.Label]; !dup {
				p.footnotes[f.Label] = f
			}
			p.advanceNextNonspace()
			p.advanceOffset(e+2, false)
			return 1
		}
	}
	// list item
	if (!p.indented || container.Type == List) && nestOK {
		if m, ok := p.parseListMarker(container); ok {
			p.closeUnmatched()
			if p.tip.Type != List || p.tip.Ordered != m.Ordered || p.tip.BulletChar != m.BulletChar || p.tip.Delim != m.Delim {
				l := p.addChild(List)
				l.Ordered, l.Start, l.BulletChar, l.Delim, l.Tight = m.Ordered, m.Start, m.BulletChar, m.Delim, true
			}
			it := p.addChild(Item)
			it.Ordered, it.BulletChar, it.Delim, it.Start = m.Ordered, m.BulletChar, m.Delim, m.Start
			it.markerOffset, it.padding = m.markerOffset, m.padding
			return 1
		}
	}
	// indented code
	if p.indented && p.tip.Type != Paragraph && p.tip.Type != Table && !p.blank {
		p.advanceOffset(4, true)
		p.closeUnmatched()
		p.addChild(CodeBlock)
		return 2
	}
	return 0
}

func stripClosingHashes(s string) string {
	t := strings.TrimRight(s, " \t")
	u := strings.TrimRight(t, "#")
	if u == t {
		return s
	}
	if strings.TrimLeft(u, " \t") == "" {
		return ""
	}
	if isSpaceOrTab(u[len(u)-1]) {
		return u
	}
	return s
}

func isSetextLine(s string) bool {
	c := s[0]
	i := 0
	for i < len(s) && s[i] == c {
		i++
	}
	for ; i < len(s); i++ {
		if !isSpaceOrTab(s[i]) {
			return false
		}
	}
	return true
}

func isThematicBreak(s string) bool {
	if s == "" || (s[0] != '*' && s[0] != '-' && s[0] != '_') {
		return false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == s[0]:
			n++
		case isSpaceOrTab(s[i]):
		default:
			return false
		}
	}
	return n >= 3
}

type listMarker struct {
	Ordered               bool
	Start                 int
	BulletChar, Delim     byte
	markerOffset, padding int
}

func (p *blockParser) parseListMarker(container *Node) (listMarker, bool) {
	ln := p.line
	rest := ln[p.nextNonspace:]
	m := listMarker{markerOffset: p.indent}
	if p.indent >= 4 || rest == "" {
		return m, false
	}
	n := 0
	switch c := rest[0]; {
	case c == '*' || c == '+' || c == '-':
		m.BulletChar = c
		n = 1
	case isDigit(c):
		for n < len(rest) && n < 10 && isDigit(rest[n]) {
			m.Start = m.Start*10 + int(rest[n]-'0')
			n++
		}
		if n > 9 || n >= len(rest) || (rest[n] != '.' && rest[n] != ')') {
			return m, false
		}
		if container.Type == Paragraph && m.Start != 1 {
			return m, false
		}
		m.Ordered, m.Delim = true, rest[n]
		n++
	default:
		return m, false
	}
	if next := peek(rest, n); n < len(rest) && next != ' ' && next != '\t' {
		return m, false
	}
	if container.Type == Paragraph && strings.TrimLeft(rest[n:], " \t") == "" {
		return m, false
	}
	p.advanceNextNonspace()
	p.advanceOffset(n, true)
	spacesStartCol, spacesStartOffset := p.column, p.offset
	for {
		p.advanceOffset(1, true)
		if !(p.column-spacesStartCol < 5 && isSpaceOrTab(peek(ln, p.offset))) {
			break
		}
	}
	blankItem := p.offset >= len(ln)
	spacesAfter := p.column - spacesStartCol
	if spacesAfter >= 5 || spacesAfter < 1 || blankItem {
		m.padding = n + 1
		p.column, p.offset = spacesStartCol, spacesStartOffset
		if isSpaceOrTab(peek(ln, p.offset)) {
			p.advanceOffset(1, true)
		}
	} else {
		m.padding = n + spacesAfter
	}
	return m, true
}

// stripRefs removes leading link reference definitions from paragraph text.
func (p *blockParser) stripRefs(content string) string {
	for len(content) > 0 && content[0] == '[' {
		n := parseReference(content, p.refs)
		if n == 0 {
			break
		}
		content = content[n:]
	}
	return content
}

func (p *blockParser) finalize(b *Node, line int) {
	above := b.Parent
	b.open = false
	b.endLine = line
	switch b.Type {
	case Paragraph:
		content := string(b.content)
		stripped := p.stripRefs(content)
		b.content = []byte(stripped)
		if len(stripped) != len(content) && isBlankString(b.content) {
			b.unlink()
		}
	case CodeBlock:
		s := string(b.content)
		if b.Fenced {
			first, rest := s, ""
			if nl := strings.IndexByte(s, '\n'); nl >= 0 {
				first, rest = s[:nl], s[nl+1:]
			}
			b.Info = unescape(trimLineSpace(first))
			b.Literal = rest
		} else {
			k := len(s)
			for {
				m := k
				for m > 0 && s[m-1] == ' ' {
					m--
				}
				if m > 0 && s[m-1] == '\n' {
					k = m - 1
					continue
				}
				break
			}
			b.Literal = s[:k] + "\n"
		}
		b.content = nil
	case HTMLBlock:
		b.Literal = strings.TrimSuffix(string(b.content), "\n")
		b.content = nil
	case MathBlock:
		b.Literal = trimLineSpace(string(b.content))
		b.content = nil
	case Item, FootnoteDef:
		if b.LastChild != nil {
			b.endLine = b.LastChild.endLine
		} else {
			b.endLine = b.startLine
		}
		if b.Type == Item && p.opt.TaskLists {
			taskItem(b)
		}
	case List:
		b.Tight = true
	items:
		for it := b.FirstChild; it != nil; it = it.Next {
			if endsWithBlankLine(it) {
				b.Tight = false
				break
			}
			for sub := it.FirstChild; sub != nil; sub = sub.Next {
				if endsWithBlankLine(sub) {
					b.Tight = false
					break items
				}
			}
		}
		if b.LastChild != nil {
			b.endLine = b.LastChild.endLine
		}
	case BlockQuote:
		if p.opt.Alerts {
			alertQuote(b)
		}
	case Table:
		b.content = nil
	}
	p.tip = above
}

func endsWithBlankLine(n *Node) bool { return n.Next != nil && n.endLine != n.Next.startLine-1 }

// taskItem turns "[ ] text" / "[x] text" at the start of an item into a task.
func taskItem(it *Node) {
	para := it.FirstChild
	if para == nil || para.Type != Paragraph {
		return
	}
	c := para.content
	if len(c) < 4 || c[0] != '[' || c[2] != ']' || !isLineSpace(c[3]) {
		return
	}
	switch c[1] {
	case ' ':
		it.Task = TaskOpen
	case 'x', 'X':
		it.Task = TaskDone
	default:
		return
	}
	para.content = c[3:]
}

// alertQuote recognizes GitHub alerts: a quote whose first line is [!TYPE].
func alertQuote(q *Node) {
	para := q.FirstChild
	if para == nil || para.Type != Paragraph {
		return
	}
	c := string(para.content)
	first, rest := c, ""
	if nl := strings.IndexByte(c, '\n'); nl >= 0 {
		first, rest = c[:nl], c[nl+1:]
	}
	first = strings.TrimRight(first, " \t")
	if len(first) < 5 || !strings.HasPrefix(first, "[!") || first[len(first)-1] != ']' {
		return
	}
	kind := strings.ToUpper(first[2 : len(first)-1])
	switch kind {
	case "NOTE", "TIP", "IMPORTANT", "WARNING", "CAUTION":
	default:
		return
	}
	q.Alert = kind
	para.content = []byte(rest)
	if isBlankString(para.content) {
		para.unlink()
	}
}

// --- GFM tables -------------------------------------------------------------

// splitCells splits a table row on unescaped pipes; "\|" becomes "|".
func splitCells(s string) []string {
	s = strings.TrimPrefix(trimLineSpace(s), "|")
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, "\\|") {
		s = s[:len(s)-1]
	}
	var cells []string
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' && i+1 < len(s):
			if s[i+1] != '|' {
				b.WriteByte(c)
			}
			b.WriteByte(s[i+1])
			i++
		case c == '|':
			cells = append(cells, trimLineSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(c)
		}
	}
	return append(cells, trimLineSpace(b.String()))
}

func parseDelimRow(s string) ([]Align, bool) {
	if strings.IndexByte(s, '|') < 0 || strings.IndexByte(s, '-') < 0 {
		return nil, false
	}
	cells := splitCells(s)
	aligns := make([]Align, len(cells))
	for i, c := range cells {
		if c == "" {
			return nil, false
		}
		l, r := c[0] == ':', c[len(c)-1] == ':'
		core := strings.Trim(c, ":")
		if core == "" || strings.Trim(core, "-") != "" || strings.Count(c, ":") > 2 {
			return nil, false
		}
		switch {
		case l && r:
			aligns[i] = AlignCenter
		case l:
			aligns[i] = AlignLeft
		case r:
			aligns[i] = AlignRight
		}
	}
	return aligns, true
}

func (p *blockParser) tableStart(para *Node, delim string) bool {
	aligns, ok := parseDelimRow(delim)
	if !ok || len(aligns) > maxTableCols || p.cells+2*len(aligns) > maxTableCells {
		return false
	}
	// Only the paragraph's last line is copied before the column counts
	// match: a long paragraph alternating text and delimiter-like lines
	// never becomes a table, and copying all of it at each such line was
	// quadratic.
	content := bytes.TrimSuffix(para.content, []byte("\n"))
	nl := bytes.LastIndexByte(content, '\n')
	head := string(content[nl+1:])
	if len(splitCells(head)) != len(aligns) || (strings.IndexByte(head, '|') < 0 && len(aligns) > 1) {
		return false
	}
	before := string(content[:nl+1])
	p.closeUnmatched()
	parent := para.Parent
	t := &Node{Type: Table, open: true, startLine: p.lineNumber - 1, Aligns: aligns}
	if rest := p.stripRefs(before); isBlankString([]byte(rest)) {
		para.insertAfter(t)
		para.unlink()
	} else {
		para.content = []byte(rest)
		p.finalize(para, p.lineNumber-2)
		parent.AppendChild(t)
	}
	p.tip = t
	p.consumed = true
	p.addTableRow(t, head, true)
	return true
}

// addTableRow adds a row with the cells line has, at most one per column:
// a shorter row is padded with empty cells when it is rendered, not here,
// where a ragged hostile table would cost a node per padding cell. The
// padding still counts towards maxTableCells.
func (p *blockParser) addTableRow(t *Node, line string, header bool) {
	p.cells += len(t.Aligns)
	row := &Node{Type: TableRow, Header: header}
	cells := splitCells(line)
	for i, c := range cells[:min(len(cells), len(t.Aligns))] {
		row.AppendChild(&Node{Type: TableCell, Align: t.Aligns[i], content: []byte(c)})
	}
	t.AppendChild(row)
}

// frontMatter extracts a leading YAML block delimited by "---" and "---" or
// "..." lines, returning the remaining source.
func (p *blockParser) frontMatter(src string) string {
	if !strings.HasPrefix(src, "---\n") && !strings.HasPrefix(src, "---\r\n") {
		return src
	}
	rest := src[strings.IndexByte(src, '\n')+1:]
	lines := 1
	for off := 0; off < len(rest); {
		nl := strings.IndexByte(rest[off:], '\n')
		end := len(rest)
		if nl >= 0 {
			end = off + nl
		}
		line := strings.TrimRight(rest[off:end], " \t\r")
		lines++
		if line == "---" || line == "..." {
			body := rest[:off]
			if !strings.Contains(body, ":") {
				return src
			}
			p.doc.AppendChild(&Node{Type: FrontMatter, Literal: strings.TrimRight(body, "\r\n"), startLine: 1, endLine: lines})
			p.lineNumber = lines
			if nl < 0 {
				return ""
			}
			return rest[end+1:]
		}
		if nl < 0 {
			break
		}
		off = end + 1
	}
	return src
}
