package md

import (
	"html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func isSpaceOrTab(c byte) bool { return c == ' ' || c == '\t' }
func isASCIIAlpha(c byte) bool { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }
func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isASCIIAlnum(c byte) bool { return isASCIIAlpha(c) || isDigit(c) }
func isHexDigit(c byte) bool   { return isDigit(c) || ((c|0x20) >= 'a' && (c|0x20) <= 'f') }

// isEscapable reports ASCII punctuation, which may be backslash-escaped.
func isEscapable(c byte) bool {
	return (c >= '!' && c <= '/') || (c >= ':' && c <= '@') || (c >= '[' && c <= '`') || (c >= '{' && c <= '~')
}

// isWhitespaceChar is CommonMark's "whitespace character" for line content.
func isLineSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

// isUnicodeSpace is CommonMark's Unicode whitespace: Zs, tab, LF, FF, CR.
func isUnicodeSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\f' || r == '\r' || (r > 0x7f && unicode.Is(unicode.Zs, r))
}

// isUnicodePunct is CommonMark 0.31 punctuation: Unicode P or S categories.
func isUnicodePunct(r rune) bool {
	if r < 0x80 {
		return (r >= '!' && r <= '/') || (r >= ':' && r <= '@') || (r >= '[' && r <= '`') || (r >= '{' && r <= '~')
	}
	return unicode.IsPunct(r) || unicode.IsSymbol(r)
}

func isBlankString(s []byte) bool {
	for _, c := range s {
		if !isLineSpace(c) {
			return false
		}
	}
	return true
}

func trimLineSpace(s string) string {
	i, j := 0, len(s)
	for i < j && isLineSpace(s[i]) {
		i++
	}
	for j > i && isLineSpace(s[j-1]) {
		j--
	}
	return s[i:j]
}

// entityAt matches &name; &#123; or &#x1F; at s[i:], returning the decoded
// text and the length consumed.
func entityAt(s string, i int) (string, int, bool) {
	if i+2 >= len(s) || s[i] != '&' {
		return "", 0, false
	}
	j := i + 1
	if s[j] == '#' {
		j++
		hex := j < len(s) && (s[j] == 'x' || s[j] == 'X')
		if hex {
			j++
		}
		st := j
		maxLen := 7
		if hex {
			maxLen = 6
		}
		for j < len(s) && j-st < maxLen+1 && ((hex && isHexDigit(s[j])) || (!hex && isDigit(s[j]))) {
			j++
		}
		n := j - st
		if n == 0 || n > maxLen || j >= len(s) || s[j] != ';' {
			return "", 0, false
		}
		base := 10
		if hex {
			base = 16
		}
		v, err := strconv.ParseInt(s[st:j], base, 32)
		r := unicode.ReplacementChar
		if err == nil && v > 0 && v <= unicode.MaxRune && (v < 0xd800 || v > 0xdfff) {
			r = rune(v)
		}
		return string(r), j + 1 - i, true
	}
	st := j
	for j < len(s) && j-st <= 32 && isASCIIAlnum(s[j]) {
		j++
	}
	if j == st || !isASCIIAlpha(s[st]) || j-st > 32 || j >= len(s) || s[j] != ';' {
		return "", 0, false
	}
	cand := s[i : j+1]
	dec := html.UnescapeString(cand)
	// html also decodes legacy names without ';' ("&copycat;" → "©cat;"):
	// only a complete entity name is valid in CommonMark.
	if dec == cand || (len(dec) > 1 && strings.HasSuffix(dec, ";")) {
		return "", 0, false
	}
	return dec, j + 1 - i, true
}

// unescape resolves backslash escapes and entities (link destinations,
// titles, info strings).
func unescape(s string) string {
	if strings.IndexByte(s, '\\') < 0 && strings.IndexByte(s, '&') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) && isEscapable(s[i+1]) {
			b.WriteByte(s[i+1])
			i++
			continue
		}
		if c == '&' {
			if dec, n, ok := entityAt(s, i); ok {
				b.WriteString(dec)
				i += n - 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

const hexDigits = "0123456789ABCDEF"

// normalizeURI percent-encodes characters not allowed in URLs while keeping
// existing %XX escapes (the mdurl/encodeURI behaviour of the reference).
func normalizeURI(s string) string {
	clean := true
	for i := 0; i < len(s); i++ {
		if c := s[i]; c == '%' || !(isASCIIAlnum(c) || strings.IndexByte(";/?:@&=+$,-_.!~*'()#", c) >= 0) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '%' && i+2 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2]):
			b.WriteString(s[i : i+3])
			i += 2
		case isASCIIAlnum(c) || strings.IndexByte(";/?:@&=+$,-_.!~*'()#", c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&15])
		}
	}
	return b.String()
}

// normalizeLabel case-folds and collapses whitespace in a link label.
func normalizeLabel(s string) string {
	s = strings.Join(strings.FieldsFunc(s, func(r rune) bool { return isUnicodeSpace(r) }), " ")
	s = strings.ToLower(s)
	if strings.ContainsRune(s, 'ß') {
		s = strings.ReplaceAll(s, "ß", "ss")
	}
	return strings.ToUpper(s)
}

// runeBefore returns the rune ending at s[:i] ('\n' at the start).
func runeBefore(s string, i int) rune {
	if i <= 0 {
		return '\n'
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return r
}

// runeAt returns the rune starting at s[i] ('\n' past the end).
func runeAt(s string, i int) rune {
	if i >= len(s) {
		return '\n'
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return r
}

// --- raw HTML (spec section 6.6) ------------------------------------------

func skipHTMLSpace(s string, i int) int {
	for i < len(s) && isLineSpace(s[i]) {
		i++
	}
	return i
}

func scanTagName(s string, i int) int {
	if i >= len(s) || !isASCIIAlpha(s[i]) {
		return -1
	}
	i++
	for i < len(s) && (isASCIIAlnum(s[i]) || s[i] == '-') {
		i++
	}
	return i
}

// scanAttribute scans whitespace + attribute at s[i:], returning the end or -1.
func scanAttribute(s string, i int) int {
	j := skipHTMLSpace(s, i)
	if j == i || j >= len(s) {
		return -1
	}
	c := s[j]
	if !isASCIIAlpha(c) && c != '_' && c != ':' {
		return -1
	}
	j++
	for j < len(s) && (isASCIIAlnum(s[j]) || strings.IndexByte("_.:-", s[j]) >= 0) {
		j++
	}
	// optional value specification
	k := skipHTMLSpace(s, j)
	if k >= len(s) || s[k] != '=' {
		return j
	}
	k = skipHTMLSpace(s, k+1)
	if k >= len(s) {
		return -1
	}
	switch q := s[k]; q {
	case '"', '\'':
		e := strings.IndexByte(s[k+1:], q)
		if e < 0 {
			return -1
		}
		return k + e + 2
	default:
		st := k
		for k < len(s) && !isLineSpace(s[k]) && strings.IndexByte("\"'=<>`", s[k]) < 0 {
			k++
		}
		if k == st {
			return -1
		}
		return k
	}
}

// scanOpenTag matches an open tag at s[i:] ('<' included); returns end or -1.
func scanOpenTag(s string, i int) int {
	j := scanTagName(s, i+1)
	if j < 0 {
		return -1
	}
	for {
		k := scanAttribute(s, j)
		if k < 0 {
			break
		}
		j = k
	}
	j = skipHTMLSpace(s, j)
	if j < len(s) && s[j] == '/' {
		j++
	}
	if j < len(s) && s[j] == '>' {
		return j + 1
	}
	return -1
}

func scanCloseTag(s string, i int) int {
	if i+1 >= len(s) || s[i+1] != '/' {
		return -1
	}
	j := scanTagName(s, i+2)
	if j < 0 {
		return -1
	}
	j = skipHTMLSpace(s, j)
	if j < len(s) && s[j] == '>' {
		return j + 1
	}
	return -1
}

// htmlEnds remembers, per subject, offsets after which a terminator
// ("-->", "]]>", "?>", ">") does not occur, so unterminated constructs
// repeated many times stay linear.
type htmlEnds map[string]int

// find returns the index of term in s at or after from, or -1.
func (h htmlEnds) find(s string, from int, term string) int {
	if none, ok := h[term]; ok && from >= none {
		return -1
	}
	e := strings.Index(s[from:], term)
	if e < 0 {
		if none, ok := h[term]; !ok || from < none {
			h[term] = from
		}
		return -1
	}
	return from + e
}

// scanHTMLTag matches any inline raw HTML at s[i:] and returns its end or -1.
func scanHTMLTag(s string, i int, ends htmlEnds) int {
	if i+1 >= len(s) || s[i] != '<' {
		return -1
	}
	rest := s[i:]
	closing := func(from int, term string) int {
		if e := ends.find(s, from, term); e >= 0 {
			return e + len(term)
		}
		return -1
	}
	switch {
	case strings.HasPrefix(rest, "<!-->"):
		return i + 5
	case strings.HasPrefix(rest, "<!--->"):
		return i + 6
	case strings.HasPrefix(rest, "<!--"):
		return closing(i+4, "-->")
	case strings.HasPrefix(rest, "<![CDATA["):
		return closing(i+9, "]]>")
	case strings.HasPrefix(rest, "<?"):
		return closing(i+2, "?>")
	case strings.HasPrefix(rest, "<!") && len(rest) > 2 && isASCIIAlpha(rest[2]):
		return closing(i+2, ">")
	case rest[1] == '/':
		return scanCloseTag(s, i)
	}
	return scanOpenTag(s, i)
}

// --- autolinks (spec 6.5) --------------------------------------------------

func scanURIAutolink(s string, i int) int {
	j := i + 1
	st := j
	for j < len(s) && (isASCIIAlnum(s[j]) || s[j] == '+' || s[j] == '.' || s[j] == '-') {
		j++
	}
	if n := j - st; n < 2 || n > 32 || !isASCIIAlpha(s[st]) || j >= len(s) || s[j] != ':' {
		return -1
	}
	for j++; j < len(s); j++ {
		c := s[j]
		if c == '>' {
			return j + 1
		}
		if c <= ' ' || c == '<' {
			return -1
		}
	}
	return -1
}

func isEmailLocal(c byte) bool {
	return isASCIIAlnum(c) || strings.IndexByte(".!#$%&'*+/=?^_`{|}~-", c) >= 0
}

func scanEmailAutolink(s string, i int) int {
	j := i + 1
	st := j
	for j < len(s) && isEmailLocal(s[j]) {
		j++
	}
	if j == st || j >= len(s) || s[j] != '@' {
		return -1
	}
	j++
	for {
		lst := j
		for j < len(s) && j-lst < 63 && (isASCIIAlnum(s[j]) || s[j] == '-') {
			j++
		}
		if j == lst || s[lst] == '-' || s[j-1] == '-' {
			return -1
		}
		if j < len(s) && s[j] == '.' {
			j++
			continue
		}
		break
	}
	if j < len(s) && s[j] == '>' {
		return j + 1
	}
	return -1
}

// --- HTML block starts (spec 4.6) ------------------------------------------

const blockTags = " address article aside base basefont blockquote body caption center col colgroup dd details dialog dir div dl dt fieldset figcaption figure footer form frame frameset h1 h2 h3 h4 h5 h6 head header hr html iframe legend li link main menu menuitem nav noframes ol optgroup option p param search section summary table tbody td tfoot th thead title tr track ul "

func hasPrefixFold(s, p string) bool { return len(s) >= len(p) && strings.EqualFold(s[:len(p)], p) }

// htmlBlockStart returns the HTML block type (1–7) starting at s, or 0.
func htmlBlockStart(s string, inParagraph bool) int {
	if len(s) < 2 || s[0] != '<' {
		return 0
	}
	for _, t := range [...]string{"<script", "<pre", "<textarea", "<style"} {
		if hasPrefixFold(s, t) {
			if len(s) == len(t) || s[len(t)] == '>' || isLineSpace(s[len(t)]) {
				return 1
			}
		}
	}
	switch {
	case strings.HasPrefix(s, "<!--"):
		return 2
	case strings.HasPrefix(s, "<?"):
		return 3
	case len(s) > 2 && s[1] == '!' && isASCIIAlpha(s[2]):
		return 4
	case strings.HasPrefix(s, "<![CDATA["):
		return 5
	}
	j := 1
	if s[1] == '/' {
		j = 2
	}
	k := j
	for k < len(s) && isASCIIAlnum(s[k]) {
		k++
	}
	if k > j {
		name := strings.ToLower(s[j:k])
		if strings.Contains(blockTags, " "+name+" ") &&
			(k == len(s) || isLineSpace(s[k]) || s[k] == '>' || strings.HasPrefix(s[k:], "/>")) {
			return 6
		}
	}
	if inParagraph {
		return 0
	}
	end := scanOpenTag(s, 0)
	if s[1] == '/' {
		end = scanCloseTag(s, 0)
	} else if end > 0 {
		name := strings.ToLower(s[1:scanTagName(s, 1)])
		if name == "script" || name == "style" || name == "pre" || name == "textarea" {
			return 0
		}
	}
	if end > 0 && strings.TrimRight(s[end:], " \t\n\r\f\v") == "" {
		return 7
	}
	return 0
}

func htmlBlockEnds(t int, line string) bool {
	switch t {
	case 1:
		l := strings.ToLower(line)
		return strings.Contains(l, "</script>") || strings.Contains(l, "</pre>") ||
			strings.Contains(l, "</textarea>") || strings.Contains(l, "</style>")
	case 2:
		return strings.Contains(line, "-->")
	case 3:
		return strings.Contains(line, "?>")
	case 4:
		return strings.Contains(line, ">")
	case 5:
		return strings.Contains(line, "]]>")
	}
	return false
}
