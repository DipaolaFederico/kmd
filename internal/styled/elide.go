package styled

import "unicode/utf8"

// vs16 is the emoji presentation selector: after a one-column base it makes
// the pair two columns wide, as Width counts it. The pair is never split.
const vs16 = 0xfe0f

// Elide shortens s to at most n columns (n ≥ 1), ending it with "…" when
// something was left out. Widths are measured as Width measures them.
func Elide(s string, n int) string {
	keep, w, last, base := len(s), 0, 0, 0 // keep: where to cut so the ellipsis fits
	for i, r := range s {
		start, rw := i, RuneWidth(r)
		if r == vs16 && last == 1 {
			start, rw, last = base, 1, 2 // the pair starts at its base
		} else {
			last, base = rw, i
		}
		if w+rw > n-1 && keep == len(s) {
			keep = start
		}
		if w += rw; w > n {
			return s[:keep] + "…"
		}
	}
	return s
}

// ElideLeft shortens s to at most n columns (n ≥ 1) by dropping its start,
// beginning it with "…" when something was left out: for text whose end
// matters most, such as a host name ("…evil.example").
func ElideLeft(s string, n int) string {
	if Width(s) <= n {
		return s
	}
	w, i := 0, len(s)
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:i])
		rw := RuneWidth(r)
		if r == vs16 && i > size {
			if b, bsize := utf8.DecodeLastRuneInString(s[:i-size]); RuneWidth(b) == 1 {
				rw, size = 2, size+bsize
			}
		}
		if w+rw > n-1 {
			break
		}
		w += rw
		i -= size
	}
	return "…" + s[i:]
}
