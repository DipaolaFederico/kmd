package diagram

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// maxLabelLines bounds how many lines a wrapped label may take.
const maxLabelLines = 8

// cleanLabel normalizes a user supplied label: HTML line breaks become
// newlines, a few common entities are decoded, Markdown-string backticks are
// removed, tabs become spaces and zero-width or control runes are dropped so
// that every remaining rune has a terminal width of at least one column.
func cleanLabel(s string) string {
	if s == "" {
		return s
	}
	s = replaceBreaks(s)
	s = decodeEntities(s)
	if len(s) >= 2 && s[0] == '`' && s[len(s)-1] == '`' {
		s = strings.ReplaceAll(s[1:len(s)-1], "**", "")
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteByte('\n')
		case r == '\t':
			b.WriteByte(' ')
		case styled.RuneWidth(r) == 0:
		default:
			b.WriteRune(r)
		}
	}
	lines := strings.Split(b.String(), "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// replaceBreaks turns <br>, <br/> and <br /> (any case) into newlines.
func replaceBreaks(s string) string {
	if !strings.Contains(s, "<") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '<' && i+2 < len(s) && lower(s[i+1]) == 'b' && lower(s[i+2]) == 'r' {
			k := i + 3
			for k < len(s) && s[k] == ' ' {
				k++
			}
			if k < len(s) && s[k] == '/' {
				k++
			}
			if k < len(s) && s[k] == '>' {
				b.WriteByte('\n')
				i = k + 1
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// decodeEntities decodes the handful of entities Mermaid users commonly write.
func decodeEntities(s string) string {
	if !strings.ContainsAny(s, "&#") {
		return s
	}
	for _, p := range [...][2]string{
		{"#quot;", "\""}, {"&quot;", "\""}, {"#lt;", "<"}, {"&lt;", "<"},
		{"#gt;", ">"}, {"&gt;", ">"}, {"&nbsp;", " "}, {"#amp;", "&"}, {"&amp;", "&"},
	} {
		if strings.Contains(s, p[0]) {
			s = strings.ReplaceAll(s, p[0], p[1])
		}
	}
	return s
}

// oneLine joins a multi-line label into a single line.
func oneLine(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

// wrap word-wraps s to lines of at most max columns, honoring explicit
// newlines and hard-splitting words that are longer than max.
func wrap(s string, max int) []string {
	if max < 1 {
		max = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line, lw := "", 0
		for _, w := range strings.Fields(para) {
			ww := styled.Width(w)
			for ww > max {
				if line != "" {
					out = append(out, line)
					line, lw = "", 0
				}
				head, rest := splitWidth(w, max)
				out = append(out, head)
				w = rest
				ww = styled.Width(w)
			}
			switch {
			case w == "":
			case line == "":
				line, lw = w, ww
			case lw+1+ww <= max:
				line += " " + w
				lw += 1 + ww
			default:
				out = append(out, line)
				line, lw = w, ww
			}
		}
		out = append(out, line)
	}
	for len(out) > 1 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	if len(out) > maxLabelLines {
		out = out[:maxLabelLines]
		out[maxLabelLines-1] = truncate(out[maxLabelLines-1]+" …", max)
	}
	return out
}

// splitWidth splits s after at most max columns, always consuming at least
// one rune so callers make progress.
func splitWidth(s string, max int) (string, string) {
	w := 0
	for i, r := range s {
		rw := styled.RuneWidth(r)
		if w+rw > max && i > 0 {
			return s[:i], s[i:]
		}
		w += rw
	}
	return s, ""
}

// truncate shortens s to at most max columns, ending with an ellipsis when
// something was cut.
func truncate(s string, max int) string {
	if styled.Width(s) <= max {
		return s
	}
	if max <= 0 {
		return ""
	}
	w := 0
	for i, r := range s {
		rw := styled.RuneWidth(r)
		if w+rw > max-1 {
			return s[:i] + "…"
		}
		w += rw
	}
	return s
}

// maxWidth returns the widest line of lines.
func maxWidth(lines []string) int {
	m := 0
	for _, l := range lines {
		if w := styled.Width(l); w > m {
			m = w
		}
	}
	return m
}

// splitWord splits s at the first run of spaces.
func splitWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

// unquote strips one pair of surrounding double quotes.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
