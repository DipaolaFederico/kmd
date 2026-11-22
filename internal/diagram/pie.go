package diagram

import (
	"math"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// pieChart is a parsed Mermaid pie chart, rendered as horizontal bars.
type pieChart struct {
	title    string
	showData bool
	labels   []string
	values   []float64
}

// parsePie parses `pie [showData] [title X]` followed by `"label" : value`
// lines.
func parsePie(header string, body []string) (*pieChart, bool) {
	p := &pieChart{}
	rest := strings.TrimSpace(strings.TrimPrefix(header, "pie"))
	if strings.HasPrefix(rest, "showData") {
		p.showData = true
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "showData"))
	}
	if kw, t := splitWord(rest); kw == "title" {
		p.title = cleanLabel(t)
	} else if rest != "" {
		return nil, false
	}
	for _, line := range body {
		kw, t := splitWord(line)
		switch {
		case kw == "title":
			p.title = cleanLabel(t)
		case line == "showData":
			p.showData = true
		case strings.HasPrefix(kw, "accTitle") || strings.HasPrefix(kw, "accDescr"):
		case strings.HasPrefix(line, "\""):
			end := strings.IndexByte(line[1:], '"')
			if end < 0 {
				return nil, false
			}
			label := line[1 : 1+end]
			num, ok := strings.CutPrefix(strings.TrimSpace(line[end+2:]), ":")
			if !ok {
				return nil, false
			}
			v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(num), ";")), 64)
			if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) || len(p.values) >= maxSlices {
				return nil, false
			}
			p.labels = append(p.labels, oneLine(cleanLabel(label)))
			p.values = append(p.values, v)
		default:
			return nil, false
		}
	}
	return p, len(p.values) > 0
}

// eighths are the partial block glyphs for 1/8 .. 7/8 of a cell.
var eighths = [...]string{"▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// formatValue prints a slice value compactly.
func formatValue(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if len(s) > 10 {
		s = strconv.FormatFloat(v, 'g', 6, 64)
	}
	return s
}

// render draws one bar per slice: label, bar scaled to the largest value,
// percentage of the total and, with showData, the raw value.
func (p *pieChart) render(width int) ([]styled.Line, bool) {
	total, maxV := 0.0, 0.0
	labelW, valW := 0, 0
	vals := make([]string, len(p.values))
	for i, v := range p.values {
		total += v
		maxV = max(maxV, v)
		labelW = max(labelW, styled.Width(p.labels[i]))
		vals[i] = formatValue(v)
		valW = max(valW, len(vals[i]))
	}
	const pctW = 6
	valCol := 0
	if p.showData {
		valCol = valW + 2
	}
	labelW = min(labelW, max(6, width/3))
	barW := width - labelW - 2 - 1 - pctW - valCol
	if barW < 5 {
		labelW = max(4, labelW-(5-barW))
		barW = width - labelW - 2 - 1 - pctW - valCol
	}
	if barW < 5 {
		return nil, false
	}
	barW = min(barW, 48)
	var out []styled.Line
	if p.title != "" {
		out = append(out, styled.Line{{Text: truncate(p.title, width), Class: styled.Title}}, nil)
	}
	for i, v := range p.values {
		label := truncate(p.labels[i], labelW)
		line := styled.Line{{Text: label + strings.Repeat(" ", labelW-styled.Width(label)+2), Class: styled.Plain}}
		cells := 0
		if maxV > 0 {
			cells = int(math.Round(v / maxV * float64(barW) * 8))
		}
		bar := strings.Repeat("█", cells/8)
		if r := cells % 8; r > 0 {
			bar += eighths[r-1]
		}
		used := (cells + 7) / 8
		if bar != "" {
			line = append(line, styled.Span{Text: bar, Class: styled.Series(i)})
		}
		pct := 0.0
		if total > 0 {
			pct = v / total * 100
		}
		ps := strconv.FormatFloat(pct, 'f', 1, 64) + "%"
		tail := strings.Repeat(" ", barW-used+1+pctW-len(ps)) + ps
		line = append(line, styled.Span{Text: tail, Class: styled.Plain})
		if p.showData {
			line = append(line, styled.Span{Text: strings.Repeat(" ", valCol-len(vals[i])) + vals[i], Class: styled.Muted})
		}
		out = append(out, line)
	}
	return out, true
}
