package styled

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Hidden reports whether r must not reach a terminal, a clipboard or an
// exported page from document text:
//
//   - C0 and C1 controls and DEL, which drive terminals (callers keep the
//     tab and newline they allow, see Strip);
//   - Unicode format characters (category Cf): bidi embeddings, overrides,
//     isolates and marks, which reorder text ("Trojan Source"), and
//     zero-width spaces, word joiners, the BOM and tag characters, which
//     make two different strings look the same (install<U+200B>.sh);
//   - the line and paragraph separators, which some terminals and editors
//     treat as line breaks.
//
// The zero-width joiner and non-joiner stay, as do variation selectors
// (category Mn): emoji sequences and scripts such as Persian and the
// Indic ones need them to be spelled correctly.
func Hidden(r rune) bool {
	switch {
	case r < 0x20 || r >= 0x7f && r < 0xa0:
		return true
	case r < 0x600: // Latin, Greek, Cyrillic…: one format character
		return r == 0xad
	case r == 0x200c || r == 0x200d:
		return false
	case r == 0x2028 || r == 0x2029:
		return true
	case r >= 0x2070 && r < 0xfeff: // symbols, CJK…: no format character
		return false
	}
	return unicode.Is(unicode.Cf, r)
}

// Strip returns s without the characters Hidden reports, except the ASCII
// controls listed in keep (such as "\t\n"), and with invalid UTF-8 replaced
// by U+FFFD (a lone 0x9b byte is an 8-bit CSI to terminals that honour C1
// controls). It returns s itself, without allocating, when there is
// nothing to change: document text is cleaned on the rendering hot path.
func Strip(s, keep string) string {
	i := firstHidden(s, keep)
	if i < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	b.WriteString(s[:i])
	for _, r := range s[i:] { // range turns invalid UTF-8 into U+FFFD
		if !Hidden(r) || strings.ContainsRune(keep, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// firstHidden returns the index of the first byte of s that Strip changes,
// or -1.
func firstHidden(s, keep string) int {
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if (c < 0x20 || c == 0x7f) && strings.IndexByte(keep, c) < 0 {
				return i
			}
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 || Hidden(r) {
			return i
		}
		i += n
	}
	return -1
}
