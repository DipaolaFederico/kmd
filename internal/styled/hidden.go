package styled

import (
	"strings"
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
	case r < 0x600: // Latin, Greek, Cyrillic…
		return r == 0xad
	case r < 0x2000: // Arabic, Syriac, Indic, Thai, Mongolian…
		return r <= 0x605 || r == 0x61c || r == 0x6dd || r == 0x70f ||
			r == 0x890 || r == 0x891 || r == 0x8e2 || r == 0x180e
	case r < 0x2070: // general punctuation
		return r == 0x200b || r == 0x200e || r == 0x200f ||
			r >= 0x2028 && r <= 0x202e || r >= 0x2060 && r != 0x2065
	case r < 0xfeff: // symbols, CJK, variation selectors…
		return false
	case r <= 0xffff:
		return r == 0xfeff || r >= 0xfff9 && r <= 0xfffb
	}
	return hiddenAstral(r)
}

// hiddenAstral is Hidden above the Basic Multilingual Plane: the format
// characters of Kaithi, Egyptian hieroglyphs, shorthand and musical
// notation, and the tag characters. The ranges of category Cf are written
// out because unicode.Is is several times slower on the scripts they live
// in; TestHiddenMatchesUnicode compares them with the unicode package of
// the Go release, so a new Unicode version cannot add one unnoticed.
func hiddenAstral(r rune) bool {
	switch {
	case r < 0x13430:
		return r == 0x110bd || r == 0x110cd
	case r < 0x1bca0:
		return r <= 0x1343f
	case r < 0x1d173:
		return r <= 0x1bca3
	case r < 0xe0001:
		return r <= 0x1d17a
	}
	return r == 0xe0001 || r >= 0xe0020 && r <= 0xe007f
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
