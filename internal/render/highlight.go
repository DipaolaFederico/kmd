package render

import (
	"strings"
	"sync"
)

// Syntax highlighting is table driven: every language is a langDef in
// langs.go (plain data: word lists and lexical rules) read by one generic
// tokenizer. It is a lexer, not a parser: enough for pleasant colours
// without shipping grammars, and linear in the input — every step consumes
// at least one byte, nothing looks back more than one byte, and only
// multi-line comments and strings carry state (two integers) to the next
// line.

// tokKind is what a word or span is highlighted as.
type tokKind uint8

const (
	kNone tokKind = iota
	kKeyword
	kType
	kFunc
	kString
	kNumber
	kComment
	kVar
)

// lang is a langDef compiled for the tokenizer: the word lists become one
// map and the space-separated rules slices. Built once per language, on
// first use, so the tables cost nothing at start-up.
type lang struct {
	words      map[string]tokKind
	line       []string // line-comment prefixes
	lineStart  []string // line comments only as the first thing on a line
	open       string   // block comment delimiters
	close      string
	nested     bool
	quotes     string
	escapes    bool
	long       [][2]string // multi-line strings (raw: no escapes)
	identExtra string
	sigils     string
	prefix     string
	caps       bool
	ci         bool
	keyColon   bool
	lispCall   bool
}

func compile(d *langDef) *lang {
	lg := &lang{
		words:      map[string]tokKind{},
		line:       strings.Fields(d.line),
		lineStart:  strings.Fields(d.lineStart),
		nested:     d.nested,
		quotes:     d.quotes,
		escapes:    !d.noEscapes,
		identExtra: d.identExtra,
		sigils:     d.sigils,
		prefix:     d.prefix,
		caps:       d.caps,
		ci:         d.ci,
		keyColon:   d.keyColon,
		lispCall:   d.lispCall,
	}
	if d.keyColon {
		lg.identExtra += "-" // keys such as `key-name:`
	}
	if b := strings.Fields(d.block); len(b) == 2 {
		lg.open, lg.close = b[0], b[1]
	}
	for l := strings.Fields(d.long); len(l) >= 2; l = l[2:] {
		lg.long = append(lg.long, [2]string{l[0], l[1]})
	}
	// later lists win: a word listed as keyword and builtin is a keyword
	for _, set := range [...]struct {
		words string
		kind  tokKind
	}{{d.builtins, kFunc}, {d.types, kType}, {d.keywords, kKeyword}} {
		for _, w := range strings.Fields(set.words) {
			lg.words[lg.key(w)] = set.kind
		}
	}
	return lg
}

var (
	compiled     [len(langDefs) + 1]*lang // the last one: fallbackLang
	compiledOnce [len(langDefs) + 1]sync.Once
)

// langFor returns the lexer for a fenced-code info string (lowercased), or
// nil for plain text. Unknown languages get a generic C/shell-like lexer.
func langFor(name string) *lang {
	if name == "" {
		return nil
	}
	i := langIndex(name)
	compiledOnce[i].Do(func() {
		if i == len(langDefs) {
			compiled[i] = compile(&fallbackLang)
		} else {
			compiled[i] = compile(&langDefs[i])
		}
	})
	return compiled[i]
}

// langIndex finds name among the aliases, or returns len(langDefs).
func langIndex(name string) int {
	for i := range langDefs {
		if hasWord(langDefs[i].aliases, name) {
			return i
		}
	}
	return len(langDefs)
}

// hasWord reports whether w is one of the space-separated words of list,
// without allocating.
func hasWord(list, w string) bool {
	for list != "" {
		i := strings.IndexByte(list, ' ')
		if i < 0 {
			return list == w
		}
		if list[:i] == w {
			return true
		}
		list = list[i+1:]
	}
	return false
}

func isIdentStart(c byte) bool { return isAlphaByte(c) || c == '_' }
func isIdent(c byte) bool      { return isIdentStart(c) || (c >= '0' && c <= '9') }
func isAlphaByte(c byte) bool  { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }

func (lg *lang) isWordByte(c byte) bool {
	return isIdent(c) || lg.identExtra != "" && strings.IndexByte(lg.identExtra, c) >= 0
}

func (lg *lang) key(w string) string {
	if lg.ci {
		return strings.ToLower(w)
	}
	return w
}

// highlight tokenizes code into styled lines.
func (r *renderer) highlight(src, name string) []rline { return highlight(src, name, r.th) }

func highlight(src, name string, th *Theme) []rline {
	return highlightWith(src, name, langFor(name), th)
}

func (th *Theme) tokStyle(k tokKind) Style {
	switch k {
	case kKeyword:
		return Style{Fg: th.Keyword}
	case kType, kVar:
		return Style{Fg: th.Type}
	case kFunc:
		return Style{Fg: th.Func}
	case kString:
		return Style{Fg: th.String}
	case kNumber:
		return Style{Fg: th.Num}
	case kComment:
		return Style{Fg: th.Comment, Attr: Italic}
	}
	return Style{}
}

func highlightWith(src, name string, lg *lang, th *Theme) []rline {
	lines := strings.Split(src, "\n")
	out := make([]rline, len(lines))
	if name == "diff" || name == "patch" {
		for i, l := range lines {
			st := Style{}
			switch {
			case strings.HasPrefix(l, "+++") || strings.HasPrefix(l, "---"):
				st = Style{Fg: th.Muted, Attr: Bold}
			case strings.HasPrefix(l, "+"):
				st.Fg = th.Added
			case strings.HasPrefix(l, "-"):
				st.Fg = th.Removed
			case strings.HasPrefix(l, "@@"):
				st.Fg = th.Type
			}
			out[i] = rline{{text: l, st: st}}
		}
		return out
	}
	if lg == nil {
		for i, l := range lines {
			out[i] = rline{{text: l}}
		}
		return out
	}
	var st lexState
	for i, l := range lines {
		h := lineLexer{l: l, th: th, mark: -1}
		lg.lexLine(&h, &st)
		h.token(len(l), Style{})
		out[i] = h.out
	}
	return out
}

// lexState is what carries over from one line to the next.
type lexState struct {
	depth int // block comment nesting; 0: not in a comment
	long  int // in the multi-line string long[long-1]; 0: not in one
}

func (lg *lang) lexLine(h *lineLexer, st *lexState) {
	l := h.l
	indent := len(l) - len(strings.TrimLeft(l, " \t")) // once: rescanning it per byte is quadratic
	for h.j < len(l) {
		if st.depth > 0 {
			lg.blockComment(h, st)
			continue
		}
		if st.long > 0 {
			lg.longString(h, st)
			continue
		}
		rest := l[h.j:]
		// the longer of a line and a block comment opener wins: Lua's
		// "--[[" opens a block, its "--" a line comment
		lineLen := lg.commentPrefix(rest, lg.line)
		if lineLen == 0 && len(lg.lineStart) > 0 && h.j <= indent {
			lineLen = lg.commentPrefix(rest, lg.lineStart)
		}
		if lg.open != "" && len(lg.open) > lineLen && strings.HasPrefix(rest, lg.open) {
			h.mark = h.j
			h.j += len(lg.open)
			st.depth = 1
			lg.blockComment(h, st)
			continue
		}
		if lineLen > 0 {
			h.token(len(l), h.th.tokStyle(kComment))
			return
		}
		if k := lg.longOpen(rest); k > 0 {
			h.mark = h.j
			h.j += len(lg.long[k-1][0])
			st.long = k
			lg.longString(h, st)
			continue
		}
		c := l[h.j]
		switch {
		case strings.IndexByte(lg.quotes, c) >= 0:
			k := h.j + 1
			for k < len(l) && l[k] != c {
				if l[k] == '\\' && lg.escapes {
					k++
				}
				k++
			}
			if k < len(l) {
				k++
			}
			h.token(min(k, len(l)), h.th.tokStyle(kString))
		case c >= '0' && c <= '9' && (h.j == 0 || !isIdent(l[h.j-1])):
			k := h.j
			for k < len(l) && (isIdent(l[k]) || l[k] == '.') {
				k++
			}
			h.token(k, h.th.tokStyle(kNumber))
		case lg.sigils != "" && strings.IndexByte(lg.sigils, c) >= 0 && h.j+1 < len(l) && (isIdentStart(l[h.j+1]) || l[h.j+1] == '{'):
			k := h.j + 1
			for k < len(l) && (isIdent(l[k]) || l[k] == '{' || l[k] == '}') {
				k++
			}
			h.token(k, h.th.tokStyle(kVar))
		case lg.prefix != "" && strings.IndexByte(lg.prefix, c) >= 0 && h.j+1 < len(l):
			k := h.j + 1
			if !isIdentStart(l[k]) {
				h.j = k
				if c == '\\' {
					h.j++ // an escaped character, such as \% in LaTeX
				}
				break
			}
			for k < len(l) && lg.isWordByte(l[k]) {
				k++
			}
			kind, ok := lg.words[lg.key(l[h.j:k])]
			if !ok {
				kind = kKeyword
			}
			h.token(k, h.th.tokStyle(kind))
		case isIdentStart(c):
			k := h.j
			for k < len(l) && lg.isWordByte(l[k]) {
				k++
			}
			if kind := lg.wordKind(l, h.j, k); kind != kNone {
				h.token(k, h.th.tokStyle(kind))
			} else {
				h.j = k // stays in the pending plain text
			}
		default:
			h.j++
		}
	}
}

// wordKind classifies the identifier l[j:k].
func (lg *lang) wordKind(l string, j, k int) tokKind {
	w := l[j:k]
	if lg.keyColon && k < len(l) && (l[k] == ':' || strings.HasPrefix(strings.TrimLeft(l[k:], " "), "=")) {
		return kFunc
	}
	if kind, ok := lg.words[lg.key(w)]; ok {
		return kind
	}
	switch {
	case k < len(l) && l[k] == '(':
		return kFunc
	case lg.lispCall && j > 0 && l[j-1] == '(':
		return kFunc
	case lg.caps && w[0] >= 'A' && w[0] <= 'Z':
		return kType
	}
	return kNone
}

// blockComment continues a block comment at h.j (its opener, if on this
// line, starts at h.mark), tracking nesting when the language allows it.
func (lg *lang) blockComment(h *lineLexer, st *lexState) {
	l := h.l
	if h.mark < 0 {
		h.mark = h.j
	}
	if !lg.nested {
		if e := strings.Index(l[h.j:], lg.close); e >= 0 {
			h.j += e + len(lg.close)
			st.depth = 0
		} else {
			h.j = len(l)
		}
	}
	for lg.nested && st.depth > 0 && h.j < len(l) {
		switch rest := l[h.j:]; {
		case strings.HasPrefix(rest, lg.close):
			h.j += len(lg.close)
			st.depth--
		case strings.HasPrefix(rest, lg.open):
			h.j += len(lg.open)
			st.depth++
		default:
			h.j++
		}
	}
	h.tokenFrom(h.mark, h.j, h.th.tokStyle(kComment))
}

// longString continues a multi-line string at h.j (its opener, if on this
// line, starts at h.mark).
func (lg *lang) longString(h *lineLexer, st *lexState) {
	if h.mark < 0 {
		h.mark = h.j
	}
	end := lg.long[st.long-1][1]
	if e := strings.Index(h.l[h.j:], end); e >= 0 {
		h.j += e + len(end)
		st.long = 0
	} else {
		h.j = len(h.l)
	}
	h.tokenFrom(h.mark, h.j, h.th.tokStyle(kString))
}

// commentPrefix returns the length of the comment prefix s starts with, or
// 0. A prefix ending in a letter (REM in Batch) must end the word; the
// match ignores case in case-insensitive languages.
func (lg *lang) commentPrefix(s string, prefixes []string) int {
	for _, p := range prefixes {
		if len(s) < len(p) {
			continue
		}
		if s[:len(p)] != p && !(lg.ci && strings.EqualFold(s[:len(p)], p)) {
			continue
		}
		if isAlphaByte(p[len(p)-1]) && len(s) > len(p) && isIdent(s[len(p)]) {
			continue
		}
		return len(p)
	}
	return 0
}

// longOpen returns 1 + the index of the multi-line string opening s, or 0.
func (lg *lang) longOpen(s string) int {
	for i, d := range lg.long {
		if strings.HasPrefix(s, d[0]) {
			return i + 1
		}
	}
	return 0
}

// lineLexer accumulates styled tokens; text between tokens stays unstyled.
type lineLexer struct {
	l     string
	th    *Theme
	j     int // scan position
	plain int // start of pending unstyled text
	mark  int // start of a comment or string being scanned; -1: none
	out   rline
}

// token emits pending plain text, then l[j:end] with style st.
func (h *lineLexer) token(end int, st Style) { h.tokenFrom(h.j, end, st) }

// tokenFrom emits pending plain text up to start, then l[start:end] with
// style st, and continues at end.
func (h *lineLexer) tokenFrom(start, end int, st Style) {
	if start > h.plain {
		h.out = append(h.out, run{text: h.l[h.plain:start]})
	}
	if end > start {
		h.out = append(h.out, run{text: h.l[start:end], st: st})
	}
	h.j, h.plain, h.mark = end, end, -1
}
