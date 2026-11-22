package diagram

import (
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// plainLines flattens styled lines to strings.
func plainLines(lines []styled.Line) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		var b strings.Builder
		for _, s := range l {
			b.WriteString(s.Text)
		}
		out[i] = b.String()
	}
	return out
}
