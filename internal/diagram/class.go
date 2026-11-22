package diagram

import "strings"

// classParser holds the state of a Mermaid class diagram parse.
type classParser struct {
	g      *graph
	block  int          // node whose `{ ... }` member block is open, or -1
	depth  int          // open namespace blocks
	stereo map[int]bool // classes that already have a «stereotype»
}

// parseClass parses a Mermaid class diagram into a graph of compartment
// boxes (name, attributes, methods). Namespaces are flattened; notes,
// styling and interaction are ignored.
func parseClass(body []string) (*graph, bool) {
	p := &classParser{g: newGraph(), block: -1, stereo: map[int]bool{}}
	for _, line := range body {
		if !p.line(strings.TrimSpace(line)) {
			return nil, false
		}
	}
	if p.g.tooBig || len(p.g.nodes) == 0 {
		return nil, false
	}
	return p.g, true
}

func (p *classParser) line(line string) bool {
	if p.block >= 0 {
		if line == "}" {
			p.block = -1
			return true
		}
		return p.member(p.block, line)
	}
	kw, rest := splitWord(line)
	switch {
	case line == "}":
		if p.depth == 0 {
			return false
		}
		p.depth--
		return true
	case kw == "direction":
		return setDirection(p.g, rest)
	case kw == "namespace":
		if !strings.HasSuffix(rest, "{") || p.depth >= maxDepth {
			return false
		}
		p.depth++
		return true
	case kw == "note", kw == "click", kw == "link", kw == "callback", kw == "style",
		kw == "classDef", kw == "cssClass", strings.HasPrefix(kw, "accTitle"), strings.HasPrefix(kw, "accDescr"):
		return true
	case kw == "class":
		return p.classDecl(rest)
	case strings.HasPrefix(line, "<<"):
		end := strings.Index(line, ">>")
		if end < 0 {
			return false
		}
		n, ok := p.ref(line[end+2:])
		if !ok {
			return false
		}
		p.annotate(n, line[2:end])
		return true
	}
	if found, ok := p.relation(line); found {
		return ok
	}
	if name, member, ok := strings.Cut(line, ":"); ok {
		n, ok := p.ref(name)
		return ok && p.member(n, member)
	}
	_, ok := p.ref(line)
	return ok
}

// classDecl handles `class Name[~T~][["Label"]][:::css] [{]`.
func (p *classParser) classDecl(rest string) bool {
	open := strings.HasSuffix(rest, "{")
	rest = strings.TrimSpace(strings.TrimSuffix(rest, "{"))
	if i := strings.Index(rest, ":::"); i >= 0 {
		rest = strings.TrimSpace(rest[:i])
	}
	var label string
	if i := strings.IndexByte(rest, '['); i > 0 && strings.HasSuffix(rest, "]") {
		label = cleanLabel(unquote(strings.TrimSpace(rest[i+1 : len(rest)-1])))
		rest = strings.TrimSpace(rest[:i])
	}
	n, ok := p.ref(rest)
	if !ok {
		return false
	}
	if label != "" {
		p.g.nodes[n].label = label
	}
	if open {
		p.block = n
	}
	return true
}

// ref resolves a class name (optionally in backticks, with ~generic~
// parameters) to its node, creating it on first use.
func (p *classParser) ref(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, ":::"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		s = s[1 : len(s)-1]
	} else if s == "" || strings.ContainsAny(s, " \t\"{}()[]<>:;|*") {
		return 0, false
	}
	id, generic, _ := strings.Cut(s, "~")
	if id == "" {
		return 0, false
	}
	n, ok := p.g.ensure(id, shapeClass)
	if !ok {
		return 0, false
	}
	if generic != "" && p.g.nodes[n].label == cleanLabel(id) {
		p.g.nodes[n].label = cleanLabel(id + generics("~"+generic))
	}
	return n, true
}

// generics turns Mermaid's `~T~` generic notation into `<T>`: a tilde
// followed by a name opens a parameter list, any other tilde closes the
// innermost open one (`Map~K, List~V~~` is `Map<K, List<V>>`).
func generics(s string) string {
	if !strings.Contains(s, "~") {
		return s
	}
	b := []byte(s)
	depth := 0
	for i, c := range b {
		if c != '~' {
			continue
		}
		switch {
		case i+1 < len(b) && b[i+1] < 0x80 && isWordByte(b[i+1]):
			b[i] = '<'
			depth++
		case depth > 0:
			b[i] = '>'
			depth--
		}
	}
	return string(b)
}

// annotate adds a «stereotype» line above the class name.
func (p *classParser) annotate(n int, a string) {
	// Mermaid shows one stereotype per class; later ones are ignored so a
	// hostile list of annotations cannot rebuild the label over and over.
	if p.stereo[n] {
		return
	}
	a = oneLine(cleanLabel(a))
	if a == "" {
		return
	}
	p.stereo[n] = true
	nd := &p.g.nodes[n]
	nd.label = "«" + a + "»\n" + nd.label
}

// member adds an attribute (no parentheses) or a method to class n.
func (p *classParser) member(n int, m string) bool {
	m = strings.TrimSpace(m)
	if m == "" {
		return true
	}
	if strings.HasPrefix(m, "<<") && strings.HasSuffix(m, ">>") {
		p.annotate(n, m[2:len(m)-2])
		return true
	}
	m = oneLine(cleanLabel(generics(m)))
	nd := &p.g.nodes[n]
	if nd.sections == nil {
		nd.sections = make([][]string, 2)
	}
	sec := 0
	if strings.Contains(m, "(") {
		sec = 1
	}
	nd.sections[sec] = append(nd.sections[sec], m)
	return true
}

// classEnd maps a relation end token to its decoration.
func classEnd(tok string) endKind {
	switch tok {
	case "<|", "|>":
		return endTriangle
	case "*":
		return endDiamond
	case "o":
		return endODiamond
	case "<", ">":
		return endArrow
	case "()":
		return endCircle
	}
	return endNone
}

// relation parses `A ["m"] <op> ["n"] B [: label]`. found is false when the
// line holds no relation operator, so the caller can try other forms; ok is
// false when it does but is malformed.
func (p *classParser) relation(line string) (found, ok bool) {
	lhs, label := line, ""
	if i := indexOutsideQuotes(line, ':'); i >= 0 {
		lhs, label = line[:i], line[i+1:]
	}
	k := opIndex(lhs)
	if k < 0 {
		return false, false
	}
	left, right := lhs[:k], lhs[k+2:]
	st := styleSolid
	if lhs[k] == '.' {
		st = styleDotted
	}
	var lt, rt string
	for _, t := range [...]string{"<|", "()", "*", "<", "o"} {
		if strings.HasSuffix(left, t) && (t != "o" || len(left) == 1 || left[len(left)-2] == ' ' || left[len(left)-2] == '"') {
			lt, left = t, left[:len(left)-len(t)]
			break
		}
	}
	for _, t := range [...]string{"|>", "()", "*", ">", "o"} {
		if strings.HasPrefix(right, t) && (t != "o" || len(right) == 1 || right[1] == ' ' || right[1] == '"') {
			rt, right = t, right[len(t):]
			break
		}
	}
	a, aCard, ok1 := splitCard(left, false)
	b, bCard, ok2 := splitCard(right, true)
	if !ok1 || !ok2 {
		return true, false
	}
	from, ok1 := p.ref(a)
	to, ok2 := p.ref(b)
	if !ok1 || !ok2 {
		return true, false
	}
	e := edge{from: from, to: to, style: st, tail: classEnd(lt), head: classEnd(rt),
		fromCard: aCard, toCard: bCard, label: cleanLabel(unquote(strings.TrimSpace(label)))}
	// Put the parent of an inheritance or the whole of a composition above.
	if parent := func(k endKind) bool { return k == endTriangle || k == endDiamond || k == endODiamond }; parent(e.head) && !parent(e.tail) {
		e.from, e.to, e.head, e.tail, e.fromCard, e.toCard = e.to, e.from, e.tail, e.head, e.toCard, e.fromCard
	}
	return true, p.g.addEdge(e)
}

// opIndex finds a `--` or `..` relation line outside quotes.
func opIndex(s string) int {
	quote := false
	for i := 0; i+1 < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			quote = !quote
		case !quote && (c == '-' || c == '.') && s[i+1] == c:
			return i
		}
	}
	return -1
}

// indexOutsideQuotes returns the first index of c outside double quotes.
func indexOutsideQuotes(s string, c byte) int {
	quote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			quote = !quote
		case c:
			if !quote {
				return i
			}
		}
	}
	return -1
}

// splitCard separates a class name from a quoted multiplicity next to the
// relation operator (`A "1"` on the left, `"*" B` on the right).
func splitCard(s string, cardFirst bool) (name, card string, ok bool) {
	s = strings.TrimSpace(s)
	if cardFirst && strings.HasPrefix(s, "\"") {
		end := strings.IndexByte(s[1:], '"')
		if end < 0 {
			return "", "", false
		}
		return strings.TrimSpace(s[end+2:]), oneLine(cleanLabel(s[1 : 1+end])), true
	}
	if !cardFirst && strings.HasSuffix(s, "\"") && len(s) >= 2 {
		start := strings.LastIndexByte(s[:len(s)-1], '"')
		if start < 0 {
			return "", "", false
		}
		return strings.TrimSpace(s[:start]), oneLine(cleanLabel(s[start+1 : len(s)-1])), true
	}
	return s, "", true
}
