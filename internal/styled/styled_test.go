package styled

import "testing"

func TestWidth(t *testing.T) {
	tests := []struct {
		s string
		w int
	}{{"abc", 3}, {"日本", 4}, {"é", 1}, {"é", 1}, {"✅", 2}, {"⚠️", 2}, {"a\u200db", 2}, {"\u0085", 0}, {"한", 2}, {"\U0001F600", 2}, {"\U00020000", 2}}
	for _, tc := range tests {
		if got := Width(tc.s); got != tc.w {
			t.Errorf("Width(%q) = %d, want %d", tc.s, got, tc.w)
		}
	}
	if RuneWidth(0x01) != 0 || RuneWidth(0x7f) != 0 || RuneWidth('a') != 1 {
		t.Error("RuneWidth")
	}
	if LineWidth(Line{{"ab", Plain}, {"日", Node}}) != 4 {
		t.Error("LineWidth")
	}
	if Series(9) != Series1 || Series(-1) != Series7 {
		t.Error("Series")
	}
}
