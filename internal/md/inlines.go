package md

import (
	"strconv"
	"strings"
)

type delimiter struct {
	cc                byte
	num, orig         int
	pos               int // offset in the subject: increases up the stack
	node              *Node
	prev, next        *delimiter
	canOpen, canClose bool
}

type bracket struct {
	node         *Node
	prev         *bracket
	prevDelim    *delimiter
	index        int
	image        bool
	bracketAfter bool
}

type inlineParser struct {
	subj      string
	pos       int
	block     *Node
	delims    *delimiter
	brackets  *bracket
	refs      map[string]ref
	footnotes map[string]*Node
	opt       *Options
	tickRuns  map[int][]int   // backtick run length -> start offsets (built lazily)
	htmlEnds  htmlEnds        // absent HTML terminators (see scanHTMLTag)
	refBudget *int            // see refBudget
	nodes     *nodeBudget     // see MaxInlines
	run       *Node           // the text node addText may extend
	runText   strings.Builder // its text once extended (see addText)
	// linkEnd is where the last link of the block ends. A link cannot
	// contain another, so every non-image opener before it is inactive.
	// One position instead of a flag per opener: deactivating them one by
	// one walked past every unclosed "![" at each link (quadratic).
	linkEnd int
}

// refBudget is how many bytes of destinations and titles the reference links
// of a document of n bytes may copy from their definitions. Without it,
// "[x]: <64 KiB URL>" followed by thousands of "[x]" makes every renderer
// write gigabytes: each use repeats the URL. Once it is spent, further
// reference links keep their text but lose their destination and title.
func refBudget(n int) int { return 1<<20 + n }

// MaxInlines bounds the inline nodes (text, emphasis, links, code spans,
// line breaks…) of a document, as MaxBlocks bounds its blocks. A node costs
// hundreds of bytes and hostile text makes one every two or three bytes
// ("*a", "a\n", "[a](b)"): gigabytes for a 16 MiB document. Past the limit
// the rest of each block is plain text and a notice paragraph is appended.
const MaxInlines = 500000

// nodeBudget counts the inline nodes a document may still create.
type nodeBudget struct {
	left int
	cut  bool // some markup was left as text because the budget ran out
}

// spent reports whether the budget is used up, noting that the caller
// leaves markup as text because of it.
func (b *nodeBudget) spent() bool {
	if b.left > 0 {
		return false
	}
	b.cut = true
	return true
}

func text(s string) *Node { return &Node{Type: Text, Literal: s} }

func (p *blockParser) processInlines() {
	nodes := &nodeBudget{left: MaxInlines}
	ip := &inlineParser{refs: p.refs, footnotes: p.footnotes, opt: &p.opt, refBudget: &p.refBudget, nodes: nodes}
	Walk(p.doc, func(n *Node, entering bool) bool {
		if entering && (n.Type == Paragraph || n.Type == Heading || n.Type == TableCell) {
			ip.parse(n, string(n.content))
			n.content = nil
			flattenDeep(n, maxInlineDepth)
			mergeText(n)
			if p.opt.Emoji || p.opt.WikiLinks {
				shortcuts(n, &p.opt, false, nodes)
			}
			if p.opt.Autolinks {
				emailAutolinks(n, nodes)
			}
			return false
		}
		return true
	})
	if p.opt.Footnotes && len(p.footnotes) > 0 {
		numberFootnotes(p.doc, p.footnotes)
	}
	if nodes.cut {
		notice := &Node{Type: Paragraph}
		notice.AppendChild(text("[kmd: inline markup ignored after " + strconv.Itoa(MaxInlines) + " elements]"))
		p.doc.AppendChild(notice)
	}
}

func (p *inlineParser) parse(block *Node, content string) {
	p.subj, p.pos, p.block, p.run = trimLineSpace(content), 0, block, nil
	p.delims, p.brackets, p.tickRuns, p.htmlEnds, p.linkEnd = nil, nil, nil, htmlEnds{}, 0
	for p.pos < len(p.subj) {
		if p.nodes.spent() {
			p.addText(p.subj[p.pos:], true)
			break
		}
		p.parseInline()
	}
	p.processEmphasis(nil)
}

// add appends n to the block, charging it to the document's budget.
func (p *inlineParser) add(n *Node) {
	p.nodes.left--
	p.block.AppendChild(n)
}

// addText appends text, extending the previous node when addText made it
// too: one node per "]", "!", "<", "&" or escaped character would cost
// hundreds of bytes per byte of input. noExt marks text the emoji and wiki
// link extensions must not touch (an escaped or entity-encoded character);
// only text with the same mark is joined.
func (p *inlineParser) addText(s string, noExt bool) {
	if r := p.run; r != nil && r == p.block.LastChild && r.noExt == noExt {
		if p.runText.Len() == 0 {
			p.runText.WriteString(r.Literal)
		}
		p.runText.WriteString(s)
		r.Literal = p.runText.String() // no copy: a Builder never rewrites its bytes
		return
	}
	p.run = &Node{Type: Text, Literal: s, noExt: noExt}
	p.runText.Reset()
	p.add(p.run)
}

// autolink appends a link showing label, which no extension rewrites.
func (p *inlineParser) autolink(dest, label string) {
	l := &Node{Type: Link, Dest: dest, noExt: true}
	l.AppendChild(text(label))
	p.nodes.left--
	p.add(l)
}

// mergeText joins adjacent text nodes below n that have the same noExt
// mark: delimiters that matched nothing are left as nodes of their own.
// Its recursion depth is bounded by flattenDeep.
func mergeText(n *Node) {
	for c := n.FirstChild; c != nil; c = c.Next {
		if c.Type != Text {
			if c.FirstChild != nil {
				mergeText(c)
			}
			continue
		}
		next := c.Next
		if next == nil || next.Type != Text || next.noExt != c.noExt {
			continue
		}
		var b strings.Builder
		b.WriteString(c.Literal)
		for ; next != nil && next.Type == Text && next.noExt == c.noExt; next = c.Next {
			b.WriteString(next.Literal)
			next.unlink()
		}
		c.Literal = b.String()
	}
}

func (p *inlineParser) parseInline() {
	c := p.subj[p.pos]
	switch c {
	case '\n':
		p.newline()
	case '\\':
		p.backslash()
	case '`':
		p.backticks()
	case '*', '_':
		p.handleDelim(c)
	case '~':
		if p.opt.Strikethrough {
			p.handleDelim(c)
		} else {
			p.str()
		}
	case '[':
		p.pos++
		node := text("[")
		p.add(node)
		p.addBracket(node, p.pos-1, false)
	case '!':
		if p.pos+1 < len(p.subj) && p.subj[p.pos+1] == '[' {
			p.pos += 2
			node := text("![")
			p.add(node)
			p.addBracket(node, p.pos-1, true)
		} else {
			p.pos++
			p.addText("!", false)
		}
	case ']':
		p.closeBracket()
	case '<':
		p.angle()
	case '&':
		if dec, n, ok := entityAt(p.subj, p.pos); ok {
			p.pos += n
			p.addText(dec, true)
		} else {
			p.pos++
			p.addText("&", false)
		}
	case '$':
		if p.opt.Math && p.math() {
			return
		}
		p.str()
	default:
		p.str()
	}
}

func (p *inlineParser) isSpecial(c byte) bool {
	switch c {
	case '\n', '\\', '`', '*', '_', '[', ']', '!', '<', '&':
		return true
	case '~':
		return p.opt.Strikethrough
	case '$':
		return p.opt.Math
	}
	return false
}

func autolinkBoundary(c byte) bool {
	return isLineSpace(c) || c == '*' || c == '_' || c == '~' || c == '('
}

// str consumes plain text up to the next character that may start markup.
func (p *inlineParser) str() {
	st := p.pos
	if p.opt.Autolinks && p.brackets == nil && (st == 0 || autolinkBoundary(p.subj[st-1])) {
		if n, dest := extAutolink(p.subj[st:]); n > 0 {
			p.autolink(dest, p.subj[st:st+n])
			p.pos += n
			return
		}
	}
	j := st + 1
	for j < len(p.subj) {
		c := p.subj[j]
		if p.isSpecial(c) {
			break
		}
		if p.opt.Autolinks && (c == 'w' || c == 'h' || c == 'f') && autolinkBoundary(p.subj[j-1]) {
			break
		}
		j++
	}
	p.pos = j
	p.addText(p.subj[st:j], false)
}

func (p *inlineParser) newline() {
	p.pos++
	if last := p.block.LastChild; last != nil && last.Type == Text && strings.HasSuffix(last.Literal, " ") {
		trimmed := strings.TrimRight(last.Literal, " ")
		hard := len(last.Literal)-len(trimmed) >= 2
		last.Literal = trimmed
		if hard {
			p.add(&Node{Type: LineBreak})
		} else {
			p.add(&Node{Type: SoftBreak})
		}
	} else {
		p.add(&Node{Type: SoftBreak})
	}
	for p.pos < len(p.subj) && p.subj[p.pos] == ' ' {
		p.pos++
	}
}

func (p *inlineParser) backslash() {
	p.pos++
	switch {
	case p.pos < len(p.subj) && p.subj[p.pos] == '\n':
		p.pos++
		p.add(&Node{Type: LineBreak})
	case p.pos < len(p.subj) && isEscapable(p.subj[p.pos]):
		p.addText(p.subj[p.pos:p.pos+1], true)
		p.pos++
	default:
		p.addText("\\", false)
	}
}

func runOf(s string, i int, c byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	return n
}

func (p *inlineParser) backticks() {
	st := p.pos
	n := runOf(p.subj, st, '`')
	after := st + n
	if p.tickRuns == nil {
		// index every maximal backtick run once, so finding a closer is a
		// binary search instead of a rescan (linear overall, like cmark)
		p.tickRuns = map[int][]int{}
		for i := 0; i < len(p.subj); {
			k := strings.IndexByte(p.subj[i:], '`')
			if k < 0 {
				break
			}
			i += k
			m := runOf(p.subj, i, '`')
			p.tickRuns[m] = append(p.tickRuns[m], i)
			i += m
		}
	}
	runs := p.tickRuns[n]
	lo, hi := 0, len(runs)
	for lo < hi {
		mid := (lo + hi) / 2
		if runs[mid] < after {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(runs) {
		k := runs[lo]
		content := strings.ReplaceAll(p.subj[after:k], "\n", " ")
		if len(content) > 0 && strings.Trim(content, " ") != "" && content[0] == ' ' && content[len(content)-1] == ' ' {
			content = content[1 : len(content)-1]
		}
		p.add(&Node{Type: Code, Literal: content})
		p.pos = k + n
		return
	}
	p.pos = after
	p.addText(p.subj[st:after], false)
}

func (p *inlineParser) handleDelim(c byte) {
	st := p.pos
	n := runOf(p.subj, st, c)
	before, after := runeBefore(p.subj, st), runeAt(p.subj, st+n)
	afterSpace, afterPunct := isUnicodeSpace(after), isUnicodePunct(after)
	beforeSpace, beforePunct := isUnicodeSpace(before), isUnicodePunct(before)
	left := !afterSpace && (!afterPunct || beforeSpace || beforePunct)
	right := !beforeSpace && (!beforePunct || afterSpace || afterPunct)
	canOpen, canClose := left, right
	if c == '_' {
		canOpen = left && (!right || beforePunct)
		canClose = right && (!left || afterPunct)
	}
	p.pos += n
	node := text(p.subj[st:p.pos])
	p.add(node)
	if c == '~' && n > 2 {
		return
	}
	if canOpen || canClose {
		d := &delimiter{cc: c, num: n, orig: n, pos: st, node: node, prev: p.delims, canOpen: canOpen, canClose: canClose}
		if d.prev != nil {
			d.prev.next = d
		}
		p.delims = d
	}
}

func (p *inlineParser) removeDelim(d *delimiter) {
	if d.prev != nil {
		d.prev.next = d.next
	}
	if d.next != nil {
		d.next.prev = d.prev
	} else {
		p.delims = d.prev
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// delimPos is the position of d, -1 for the bottom of the stack (nil).
func delimPos(d *delimiter) int {
	if d == nil {
		return -1
	}
	return d.pos
}

// bottomSlot is closer's entry in openersBottom: closers that can match
// the same openers share one.
func bottomSlot(closer *delimiter) int {
	switch closer.cc {
	case '_':
		return 2 + b2i(closer.canOpen)*3 + closer.orig%3
	case '*':
		return 8 + b2i(closer.canOpen)*3 + closer.orig%3
	}
	return 13 + closer.orig // '~'
}

// findOpener returns the nearest delimiter after position stop that closer
// can close, or nil.
func findOpener(closer *delimiter, stop int) *delimiter {
	for opener := closer.prev; opener != nil && opener.pos > stop; opener = opener.prev {
		if closer.cc == '~' {
			if opener.cc == '~' && opener.canOpen && opener.num == closer.num {
				return opener
			}
			continue
		}
		odd := (closer.canOpen || opener.canClose) && closer.orig%3 != 0 && (opener.orig+closer.orig)%3 == 0
		if opener.cc == closer.cc && opener.canOpen && !odd {
			return opener
		}
	}
	return nil
}

func (p *inlineParser) processEmphasis(bottom *delimiter) {
	// openersBottom holds positions, not delimiters: a match removes the
	// delimiters between its opener and closer, and a stored pointer to one
	// of them was never met again, so every later closer of that kind
	// walked to the bottom of the stack (quadratic).
	var openersBottom [16]int
	for i := range openersBottom {
		openersBottom[i] = delimPos(bottom)
	}
	if p.delims == bottom {
		return // nothing above the bottom: avoid walking the whole stack
	}
	closer := p.delims
	for closer != nil && closer.prev != bottom {
		closer = closer.prev
	}
	for closer != nil && !p.nodes.spent() {
		if !closer.canClose {
			closer = closer.next
			continue
		}
		idx := bottomSlot(closer)
		opener := findOpener(closer, openersBottom[idx])
		old := closer
		if opener == nil {
			closer = closer.next
			openersBottom[idx] = delimPos(old.prev)
			if !old.canOpen {
				p.removeDelim(old)
			}
			continue
		}
		use := 1
		switch {
		case closer.cc == '~':
			use = closer.num
		case closer.num >= 2 && opener.num >= 2:
			use = 2
		}
		oi, ci := opener.node, closer.node
		opener.num -= use
		closer.num -= use
		oi.Literal = oi.Literal[:len(oi.Literal)-use]
		ci.Literal = ci.Literal[:len(ci.Literal)-use]
		typ := Emph
		if closer.cc == '~' {
			typ = Strike
		} else if use == 2 {
			typ = Strong
		}
		emph := &Node{Type: typ}
		p.nodes.left--
		for tmp := oi.Next; tmp != nil && tmp != ci; {
			next := tmp.Next
			emph.AppendChild(tmp)
			tmp = next
		}
		oi.insertAfter(emph)
		if opener.next != closer {
			opener.next = closer
			closer.prev = opener
		}
		if opener.num == 0 {
			oi.unlink()
			p.removeDelim(opener)
		}
		if closer.num == 0 {
			ci.unlink()
			next := closer.next
			p.removeDelim(closer)
			closer = next
		}
	}
	for p.delims != nil && p.delims != bottom {
		p.removeDelim(p.delims)
	}
}

func (p *inlineParser) addBracket(node *Node, index int, image bool) {
	if p.brackets != nil {
		p.brackets.bracketAfter = true
	}
	p.brackets = &bracket{node: node, prev: p.brackets, prevDelim: p.delims, index: index, image: image}
}

// active reports whether b can still open a link or image.
func (p *inlineParser) active(b *bracket) bool { return b.image || b.index >= p.linkEnd }

func (p *inlineParser) spnl() {
	for p.pos < len(p.subj) && isSpaceOrTab(p.subj[p.pos]) {
		p.pos++
	}
	if p.pos < len(p.subj) && p.subj[p.pos] == '\n' {
		p.pos++
		for p.pos < len(p.subj) && isSpaceOrTab(p.subj[p.pos]) {
			p.pos++
		}
	}
}

// maxLabel is the longest link label CommonMark allows, in bytes between
// the brackets.
const maxLabel = 999

// linkLabel matches [label] at pos and returns its length (0 if none).
func (p *inlineParser) linkLabel() int {
	s := p.subj
	if p.pos >= len(s) || s[p.pos] != '[' {
		return 0
	}
	for j := p.pos + 1; j < len(s) && j-p.pos <= maxLabel+1; j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			return 0
		case ']':
			n := j + 1 - p.pos
			if n > maxLabel+2 {
				return 0
			}
			p.pos = j + 1
			return n
		}
	}
	return 0
}

func (p *inlineParser) linkDestination() (string, bool) {
	s := p.subj
	if p.pos < len(s) && s[p.pos] == '<' {
		for j := p.pos + 1; j < len(s); j++ {
			switch s[j] {
			case '\\':
				if j+1 < len(s) && s[j+1] != '\n' {
					j++
				}
			case '>':
				d := s[p.pos+1 : j]
				p.pos = j + 1
				return normalizeURI(unescape(d)), true
			case '<', '\n':
				return "", false
			}
		}
		return "", false
	}
	st, open := p.pos, 0
	j := st
loop:
	for j < len(s) {
		c := s[j]
		switch {
		case c == '\\' && j+1 < len(s) && isEscapable(s[j+1]):
			j += 2
		case c == '(':
			open++
			if open > 32 { // cmark's limit: keeps unclosed "[a](" runs linear
				return "", false
			}
			j++
		case c == ')':
			if open < 1 {
				break loop
			}
			open--
			j++
		case c <= ' ' || c == 0x7f:
			break loop
		default:
			j++
		}
	}
	if (j == st && (j >= len(s) || s[j] != ')')) || open != 0 {
		return "", false
	}
	p.pos = j
	return normalizeURI(unescape(s[st:j])), true
}

func (p *inlineParser) linkTitle() (string, bool) {
	s := p.subj
	if p.pos >= len(s) {
		return "", false
	}
	open := s[p.pos]
	closer := open
	switch open {
	case '"', '\'':
	case '(':
		closer = ')'
	default:
		return "", false
	}
	for j := p.pos + 1; j < len(s); j++ {
		c := s[j]
		switch {
		case c == '\\' && j+1 < len(s):
			j++
		case c == closer:
			t := unescape(s[p.pos+1 : j])
			p.pos = j + 1
			return t, true
		case open == '(' && c == '(':
			return "", false
		}
	}
	return "", false
}

func (p *inlineParser) closeBracket() {
	p.pos++
	startpos := p.pos
	opener := p.brackets
	if opener == nil {
		p.addText("]", false)
		return
	}
	if !p.active(opener) {
		p.addText("]", false)
		p.brackets = opener.prev
		return
	}
	isImage := opener.image
	s := p.subj

	// footnote reference [^label]. A label holds no unescaped bracket and
	// at most maxLabel bytes; checking that first keeps nested "[^[^[^…]]]"
	// from normalizing ever longer non-labels at each "]" (quadratic).
	if p.opt.Footnotes && !isImage && !opener.bracketAfter && opener.index+1 < startpos-1 &&
		startpos-1-(opener.index+1) <= maxLabel && s[opener.index+1] == '^' {
		label := normalizeLabel(s[opener.index+2 : startpos-1])
		if _, ok := p.footnotes[label]; ok && label != "" {
			for tmp := opener.node.Next; tmp != nil; {
				next := tmp.Next
				tmp.unlink()
				tmp = next
			}
			for p.delims != nil && p.delims != opener.prevDelim {
				p.removeDelim(p.delims)
			}
			p.nodes.left--
			opener.node.insertAfter(&Node{Type: FootnoteRef, Label: label})
			opener.node.unlink()
			p.brackets = opener.prev
			return
		}
	}

	savepos := p.pos
	matched := false
	var dest, title string
	if p.pos < len(s) && s[p.pos] == '(' {
		p.pos++
		p.spnl()
		if d, ok := p.linkDestination(); ok {
			dest = d
			beforeTitle := p.pos
			p.spnl()
			if p.pos != beforeTitle {
				if t, ok := p.linkTitle(); ok {
					title = t
				}
			}
			p.spnl()
			if p.pos < len(s) && s[p.pos] == ')' {
				p.pos++
				matched = true
			}
		}
		if !matched {
			p.pos = savepos
		}
	}
	if !matched {
		beforeLabel := p.pos
		n := p.linkLabel()
		label, has := "", false
		if n > 2 {
			label, has = s[beforeLabel:beforeLabel+n], true
		} else if !opener.bracketAfter && startpos-opener.index-2 <= maxLabel {
			label, has = s[opener.index:startpos], true
		}
		if n == 0 {
			p.pos = savepos
		}
		if has {
			if r, ok := p.refs[normalizeLabel(label[1:len(label)-1])]; ok {
				matched = true
				if cost := len(r.dest) + len(r.title); *p.refBudget >= cost {
					*p.refBudget -= cost
					dest, title = r.dest, r.title
				}
			}
		}
	}
	if !matched {
		p.brackets = opener.prev
		p.pos = startpos
		p.addText("]", false)
		return
	}
	typ := Link
	if isImage {
		typ = Image
	}
	node := &Node{Type: typ, Dest: dest, Title: title}
	for tmp := opener.node.Next; tmp != nil; {
		next := tmp.Next
		node.AppendChild(tmp)
		tmp = next
	}
	p.add(node)
	p.processEmphasis(opener.prevDelim)
	p.brackets = opener.prev
	opener.node.unlink()
	if !isImage {
		p.linkEnd = p.pos
	}
}

func (p *inlineParser) angle() {
	s, i := p.subj, p.pos
	if e := scanURIAutolink(s, i); e > 0 {
		uri := s[i+1 : e-1]
		p.autolink(normalizeURI(uri), uri)
		p.pos = e
		return
	}
	if e := scanEmailAutolink(s, i); e > 0 {
		addr := s[i+1 : e-1]
		p.autolink(normalizeURI("mailto:"+addr), addr)
		p.pos = e
		return
	}
	if e := scanHTMLTag(s, i, p.htmlEnds); e > 0 {
		p.add(&Node{Type: HTMLInline, Literal: s[i:e]})
		p.pos = e
		return
	}
	p.pos++
	p.addText("<", false)
}

// math parses $…$ (inline) or $$…$$ (display) at pos.
func (p *inlineParser) math() bool {
	s, st := p.subj, p.pos
	if strings.HasPrefix(s[st:], "$$") {
		if e := strings.Index(s[st+2:], "$$"); e > 0 {
			p.add(&Node{Type: Math, Display: true, Literal: trimLineSpace(s[st+2 : st+2+e])})
			p.pos = st + 2 + e + 2
			return true
		}
		return false
	}
	if st+1 >= len(s) || isLineSpace(s[st+1]) {
		return false
	}
	for j := st + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '$':
			// the first unescaped $ decides: it must close cleanly or this is not math
			if isLineSpace(s[j-1]) || (j+1 < len(s) && isDigit(s[j+1])) {
				return false
			}
			p.add(&Node{Type: Math, Literal: s[st+1 : j]})
			p.pos = j + 1
			return true
		}
	}
	return false
}

// --- GFM extended autolinks ------------------------------------------------

func isHostChar(c byte) bool { return isASCIIAlnum(c) || c >= 0x80 }

// maxDomain is the longest host name DNS allows. A longer run of host
// characters is not a domain: without the bound, "www.a_www.a_…" scanned the
// rest of the line at every "www." (each candidate failing on the "_" of
// its last segments), which was quadratic.
const maxDomain = 253

// checkDomain returns the length of a valid domain prefix of s, or 0.
func checkDomain(s string, allowShort bool) int {
	if s == "" || !isHostChar(s[0]) {
		return 0
	}
	np, us1, us2 := 0, 0, 0
	i := 1
	for end := min(len(s), maxDomain+1); i < end; i++ {
		c := s[i]
		switch {
		case c == '_':
			us2++
		case c == '.':
			if i+1 >= len(s) || !isHostChar(s[i+1]) {
				goto done
			}
			us1, us2 = us2, 0
			np++
		case c == '-' || isHostChar(c):
		default:
			goto done
		}
	}
done:
	if i > maxDomain || us1 > 0 || us2 > 0 {
		return 0
	}
	if !allowShort && np == 0 {
		return 0
	}
	return i
}

// autolinkDelim trims trailing punctuation from an autolink candidate.
func autolinkDelim(s string, end int) int {
	if i := strings.IndexByte(s[:end], '<'); i >= 0 {
		end = i
	}
	// counted once, not at each trailing ')': "http://a.b/)))…" was quadratic
	open, closed := -1, 0
	for end > 0 {
		c := s[end-1]
		switch {
		case strings.IndexByte("?!.,:*_~'\"", c) >= 0:
			end--
		case c == ';':
			k := end - 2
			for k > 0 && isASCIIAlnum(s[k]) {
				k--
			}
			if k < end-2 && s[k] == '&' {
				end = k
			} else {
				end--
			}
		case c == ')':
			if open < 0 {
				open, closed = strings.Count(s[:end], "("), strings.Count(s[:end], ")")
			}
			if closed <= open {
				return end
			}
			closed--
			end--
		default:
			return end
		}
	}
	return end
}

// extAutolink matches www. / http:// / https:// autolinks at the start of s.
func extAutolink(s string) (int, string) {
	var n int
	switch {
	case strings.HasPrefix(s, "www."):
		n = checkDomain(s, false)
	case strings.HasPrefix(s, "http://"), strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "ftp://"):
		k := strings.Index(s, "://") + 3
		d := checkDomain(s[k:], true)
		if d == 0 {
			return 0, ""
		}
		n = k + d
	default:
		return 0, ""
	}
	if n == 0 {
		return 0, ""
	}
	for n < len(s) && !isLineSpace(s[n]) && s[n] != '<' {
		n++
	}
	n = autolinkDelim(s, n)
	if n == 0 {
		return 0, ""
	}
	if strings.HasPrefix(s, "www.") {
		return n, "http://" + s[:n]
	}
	return n, s[:n]
}

func isEmailChar(c byte) bool { return isASCIIAlnum(c) || c == '.' || c == '+' || c == '-' || c == '_' }

// emailAutolinks links bare e-mail addresses in the text of block.
func emailAutolinks(block *Node, nodes *nodeBudget) {
	for n := block.FirstChild; n != nil; n = n.Next {
		switch n.Type {
		case Link, Image:
			continue
		case Text:
			if n.Next != nil && n.Next.Type == Text { // merge adjacent text
				var b strings.Builder
				b.WriteString(n.Literal)
				for n.Next != nil && n.Next.Type == Text {
					b.WriteString(n.Next.Literal)
					n.Next.unlink()
				}
				n.Literal = b.String()
			}
			if strings.IndexByte(n.Literal, '@') >= 0 {
				n = linkEmails(n, nodes)
			}
		default:
			if n.FirstChild != nil {
				emailAutolinks(n, nodes)
			}
		}
	}
}

// linkEmails splits text node t around e-mail addresses; returns the last node.
func linkEmails(t *Node, nodes *nodeBudget) *Node {
	s := t.Literal
	for at := strings.IndexByte(s, '@'); at >= 0; {
		st := at
		for st > 0 && isEmailChar(s[st-1]) {
			st--
		}
		end, dots := at+1, 0
		for end < len(s) {
			c := s[end]
			if isASCIIAlnum(c) || c == '-' || c == '_' {
				end++
			} else if c == '.' && end+1 < len(s) && isASCIIAlnum(s[end+1]) {
				dots++
				end++
			} else {
				break
			}
		}
		if st < at && dots > 0 && end > at+1 && isASCIIAlpha(s[end-1]) {
			if nodes.spent() {
				break
			}
			nodes.left -= 3 // the link, its text and the text after it
			end = at + autolinkDelim(s[at:], end-at)
			addr := s[st:end]
			l := &Node{Type: Link, Dest: "mailto:" + addr}
			l.AppendChild(text(addr))
			t.Literal = s[:st]
			t.insertAfter(l)
			rest := text(s[end:])
			l.insertAfter(rest)
			if t.Literal == "" {
				t.unlink()
			}
			t, s = rest, s[end:]
			at = strings.IndexByte(s, '@')
			continue
		}
		next := strings.IndexByte(s[at+1:], '@')
		if next < 0 {
			break
		}
		at += next + 1
	}
	if t.Literal == "" && t.Parent != nil {
		prev := t.Prev
		t.unlink()
		if prev != nil {
			return prev
		}
	}
	return t
}

// numberFootnotes assigns numbers to footnotes in order of first reference.
func numberFootnotes(doc *Node, defs map[string]*Node) {
	next := 1
	Walk(doc, func(n *Node, entering bool) bool {
		if entering && n.Type == FootnoteRef {
			d := defs[n.Label]
			if d.Index == 0 {
				d.Index = next
				next++
			}
			d.refCount++
			n.Index = d.Index
		}
		return true
	})
}

// parseReference parses a link reference definition at the start of s,
// stores it in refs (first definition wins) and returns the bytes consumed.
func parseReference(s string, refs map[string]ref) int {
	p := &inlineParser{subj: s}
	n := p.linkLabel()
	if n == 0 {
		return 0
	}
	raw := s[:n]
	if p.pos >= len(s) || s[p.pos] != ':' {
		return 0
	}
	p.pos++
	p.spnl()
	dest, ok := p.linkDestination()
	if !ok {
		return 0
	}
	beforeTitle := p.pos
	p.spnl()
	title := ""
	hasTitle := false
	if p.pos != beforeTitle {
		title, hasTitle = p.linkTitle()
	}
	if !hasTitle {
		title = ""
		p.pos = beforeTitle
	}
	atLineEnd := p.spaceToLineEnd()
	if !atLineEnd {
		if !hasTitle {
			return 0
		}
		// the title is not followed by a line end: drop it and retry
		title = ""
		p.pos = beforeTitle
		if !p.spaceToLineEnd() {
			return 0
		}
	}
	label := normalizeLabel(raw[1 : len(raw)-1])
	if label == "" {
		return 0
	}
	if _, dup := refs[label]; !dup {
		refs[label] = ref{dest: dest, title: title}
	}
	return p.pos
}

// spaceToLineEnd consumes spaces/tabs and a newline (or end of input).
func (p *inlineParser) spaceToLineEnd() bool {
	j := p.pos
	for j < len(p.subj) && isSpaceOrTab(p.subj[j]) {
		j++
	}
	if j == len(p.subj) {
		p.pos = j
		return true
	}
	if p.subj[j] == '\n' {
		p.pos = j + 1
		return true
	}
	return false
}

// maxInlineDepth bounds nesting of inline containers (emphasis, links,
// images). Deeper containers are unwrapped so that every recursive consumer
// of the tree (renderers, walkers) stays far from stack limits.
const maxInlineDepth = 100

// flattenDeep unwraps inline containers nested deeper than limit below
// block, iteratively (the tree may be arbitrarily deep at this point).
func flattenDeep(block *Node, limit int) {
	type item struct {
		n     *Node
		depth int
	}
	stack := []item{}
	for c := block.FirstChild; c != nil; c = c.Next {
		stack = append(stack, item{c, 1})
	}
	for len(stack) > 0 {
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := it.n
		if n.FirstChild == nil {
			continue
		}
		if it.depth >= limit {
			// splice the children in place of n, keeping the same depth
			for c := n.FirstChild; c != nil; {
				next := c.Next
				n.insertBefore(c)
				stack = append(stack, item{c, it.depth})
				c = next
			}
			n.unlink()
			continue
		}
		for c := n.FirstChild; c != nil; c = c.Next {
			stack = append(stack, item{c, it.depth + 1})
		}
	}
}
