package view

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// query is a lowercased filter, as runes (fuzzy matching) and as a string
// (substring bonus).
type query struct {
	runes []rune
	lower string
}

func newQuery(s string) query {
	s = strings.ToLower(s)
	return query{runes: []rune(s), lower: s}
}

// fuzzy scores the name of d against q: every rune of q must appear in
// the name in order, ignoring case. Matches at the start of a path
// segment or word, consecutive matches and matches in the file name score
// higher. The byte offsets of the matches are appended to pos.
func fuzzy(d *doc, q query, pos []int) (int, []int, bool) {
	name, qr := d.name, q.runes
	if len(qr) == 0 {
		return 0, pos, true
	}
	base := strings.LastIndexByte(name, '/') + 1
	score, qi, prevMatch := 0, 0, -2
	prev := '/'
	for i, r := range name {
		lr := r
		switch {
		case r >= 'A' && r <= 'Z':
			lr = r + 'a' - 'A'
		case r >= utf8.RuneSelf:
			lr = unicode.ToLower(r)
		}
		if lr == qr[qi] {
			score++
			if prev == '/' || prev == '-' || prev == '_' || prev == '.' || prev == ' ' {
				score += 8
			}
			if prevMatch == i-utf8.RuneLen(prev) {
				score += 4
			}
			if i >= base {
				score += 2
			}
			prevMatch = i
			pos = append(pos, i)
			if qi++; qi == len(qr) {
				return score + substringBonus(d.lower, base, q.lower), pos, true
			}
		}
		prev = r
	}
	return 0, pos, false
}

// substringBonus favours names that contain the query as typed, above all
// in the file name and at its start ("guide" → guide.md first). lower is
// the name with ASCII letters lowered (same offsets as the name).
func substringBonus(lower string, base int, q string) int {
	i := strings.Index(lower, q)
	switch {
	case i < 0:
		return 0
	case i == base:
		return 40
	case i > base:
		return 30
	}
	return 20
}

// scored is a document matching the filter, with its score.
type scored struct{ i, s int }

// filterDocs returns the indexes of the documents matching q, best first;
// with an empty query, all of them in order. out and ms are reused buffers.
func filterDocs(docs []doc, q query, out []int, ms []scored) ([]int, []scored) {
	out, ms = out[:0], ms[:0]
	if len(q.runes) == 0 {
		for i := range docs {
			out = append(out, i)
		}
		return out, ms
	}
	var buf [64]int
	for i := range docs {
		if s, _, ok := fuzzy(&docs[i], q, buf[:0]); ok {
			ms = append(ms, scored{i, s})
		}
	}
	slices.SortFunc(ms, func(x, y scored) int {
		if x.s != y.s {
			return y.s - x.s
		}
		if lx, ly := len(docs[x.i].name), len(docs[y.i].name); lx != ly {
			return lx - ly
		}
		return x.i - y.i
	})
	for _, m := range ms {
		out = append(out, m.i)
	}
	return out, ms
}
