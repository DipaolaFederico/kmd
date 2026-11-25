package styled

import "testing"

// Elide and ElideLeft keep within n columns as Width measures them, the
// emoji presentation selector included.
func TestElide(t *testing.T) {
	for _, c := range []struct {
		s           string
		n           int
		right, left string
	}{
		{"12345", 5, "12345", "12345"},
		{"123456", 5, "1234…", "…3456"},
		{"日本語", 6, "日本語", "日本語"},
		{"日本語x", 6, "日本…", "…本語x"},
		{"a❤️b", 3, "a…", "…b"},
		{"❤️❤️❤️", 4, "❤️…", "…❤️"},
		{"abc", 1, "…", "…"},
	} {
		if got := Elide(c.s, c.n); got != c.right || Width(got) > c.n {
			t.Errorf("Elide(%q, %d) = %q, want %q", c.s, c.n, got, c.right)
		}
		if got := ElideLeft(c.s, c.n); got != c.left || Width(got) > c.n {
			t.Errorf("ElideLeft(%q, %d) = %q, want %q", c.s, c.n, got, c.left)
		}
	}
}
