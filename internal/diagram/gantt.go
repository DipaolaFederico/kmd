package diagram

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/DipaolaFederico/kmd/internal/styled"
)

// Gantt limits and time range. Times are seconds since the Unix epoch held
// in float64 so durations of any size add without overflowing; results
// outside years 1..9999 are rejected.
const (
	maxTasks = maxNodes
	minTime  = -62135596800.0 // 0001-01-01T00:00:00Z
	maxTime  = 253402300799.0 // 9999-12-31T23:59:59Z
	day      = 86400.0
	// maxFormat bounds dateFormat/axisFormat: time.Parse rescans the layout
	// for every date, and real formats are far shorter.
	maxFormat = 64
)

// ganttTask is one bar (or milestone) of a Gantt chart.
type ganttTask struct {
	name                          string
	section                       int // index into ganttChart.sections, -1 for none
	id                            string
	done, active, crit, milestone bool
	startSpec, endSpec            string
	start, end                    float64
	state                         uint8 // 0 unresolved, 1 resolving, 2 resolved
}

// ganttChart is a parsed Mermaid Gantt chart.
type ganttChart struct {
	title     string
	layout    string // Go time layout of dateFormat
	axis      string // Go time layout of axisFormat, "" for automatic
	inclusive bool   // inclusiveEndDates: end dates include their day
	sections  []string
	tasks     []ganttTask
	ids       map[string]int
}

// parseGantt parses a Mermaid gantt chart and resolves every task's start
// and end time.
func parseGantt(body []string) (*ganttChart, bool) {
	gc := &ganttChart{layout: "2006-01-02", ids: map[string]int{}}
	for _, line := range body {
		kw, rest := splitWord(line)
		switch kw {
		case "title":
			gc.title = cleanLabel(rest)
			continue
		case "dateFormat":
			if len(rest) > maxFormat {
				return nil, false
			}
			l, ok := dayjsLayout(rest)
			if !ok {
				return nil, false
			}
			gc.layout = l
			continue
		case "axisFormat":
			if len(rest) <= maxFormat {
				gc.axis = strftimeLayout(rest)
			}
			continue
		case "inclusiveEndDates":
			gc.inclusive = true
			continue
		case "section":
			gc.sections = append(gc.sections, oneLine(cleanLabel(rest)))
			continue
		case "excludes", "includes", "todayMarker", "tickInterval", "weekday", "weekend",
			"displayMode", "topAxis", "click":
			continue
		}
		if strings.HasPrefix(kw, "accTitle") || strings.HasPrefix(kw, "accDescr") {
			continue
		}
		name, meta, ok := strings.Cut(line, ":")
		if !ok || !gc.addTask(name, meta) {
			return nil, false
		}
	}
	if len(gc.tasks) == 0 {
		return nil, false
	}
	for i := range gc.tasks {
		if !gc.resolve(i) {
			return nil, false
		}
	}
	return gc, true
}

// addTask parses `[tags,] [id,] [start,] end` task metadata.
func (gc *ganttChart) addTask(name, meta string) bool {
	if len(gc.tasks) >= maxTasks {
		return false
	}
	t := ganttTask{name: oneLine(cleanLabel(name)), section: len(gc.sections) - 1}
	items := strings.Split(meta, ",")
	for i := range items {
		items[i] = strings.TrimSpace(items[i])
	}
tags:
	for len(items) > 0 {
		switch items[0] {
		case "done":
			t.done = true
		case "active":
			t.active = true
		case "crit":
			t.crit = true
		case "milestone":
			t.milestone = true
		case "vert":
		default:
			break tags
		}
		items = items[1:]
	}
	switch len(items) {
	case 1:
		t.endSpec = items[0]
	case 2:
		t.startSpec, t.endSpec = items[0], items[1]
	case 3:
		t.id, t.startSpec, t.endSpec = items[0], items[1], items[2]
	default:
		return false
	}
	if t.endSpec == "" {
		return false
	}
	if t.id != "" {
		if _, dup := gc.ids[t.id]; dup {
			return false
		}
		gc.ids[t.id] = len(gc.tasks)
	}
	gc.tasks = append(gc.tasks, t)
	return true
}

// resolve computes the start and end of task i, first resolving the tasks
// it depends on (the previous task for an implicit start, `after` and
// `until` references). Dependency cycles fail; recursion depth is bounded
// by maxTasks because each task is entered at most once.
func (gc *ganttChart) resolve(i int) bool {
	t := &gc.tasks[i]
	switch t.state {
	case 1:
		return false
	case 2:
		return true
	}
	t.state = 1
	start, ok := 0.0, false
	switch spec := t.startSpec; {
	case spec == "":
		if i == 0 || !gc.resolve(i-1) {
			return false
		}
		start, ok = gc.tasks[i-1].end, true
	case strings.HasPrefix(spec, "after "):
		start, ok = gc.refs(spec[len("after "):], true)
	default:
		start, ok = gc.date(spec)
	}
	if !ok {
		return false
	}
	end := 0.0
	switch spec := t.endSpec; {
	case strings.HasPrefix(spec, "until "):
		end, ok = gc.refs(spec[len("until "):], false)
	default:
		if d, isDur := duration(spec); isDur {
			end = start + d
			break
		}
		end, ok = gc.date(spec)
		if ok && gc.inclusive {
			end += day
		}
	}
	if !ok || end < start || end > maxTime {
		return false
	}
	t = &gc.tasks[i] // the slice is not reallocated, but stay explicit
	t.start, t.end, t.state = start, end, 2
	return true
}

// refs resolves space-separated task ids: the latest end for `after`, the
// earliest start for `until`.
func (gc *ganttChart) refs(list string, after bool) (float64, bool) {
	f := strings.Fields(list)
	if len(f) == 0 {
		return 0, false
	}
	var v float64
	for k, id := range f {
		j, ok := gc.ids[id]
		if !ok || !gc.resolve(j) {
			return 0, false
		}
		x := gc.tasks[j].start
		if after {
			x = gc.tasks[j].end
		}
		if k == 0 || after && x > v || !after && x < v {
			v = x
		}
	}
	return v, true
}

// date parses s with the chart's dateFormat.
func (gc *ganttChart) date(s string) (float64, bool) {
	tm, err := time.Parse(gc.layout, s)
	if err != nil {
		return 0, false
	}
	v := float64(tm.Unix())
	return v, v >= minTime && v <= maxTime
}

// duration parses Mermaid durations such as 3d, 2w, 1.5h, 30m, 10s, 500ms.
func duration(s string) (float64, bool) {
	units := [...]struct {
		suffix string
		secs   float64
	}{{"ms", 0.001}, {"s", 1}, {"m", 60}, {"h", 3600}, {"d", day}, {"w", 7 * day}}
	for _, u := range units {
		num, ok := strings.CutSuffix(s, u.suffix)
		if !ok || num == "" || strings.ContainsAny(num, "eExXpP_+-") {
			continue
		}
		v, err := strconv.ParseFloat(num, 64)
		if err != nil || math.IsInf(v, 0) || math.IsNaN(v) || v*u.secs > maxTime-minTime {
			return 0, false
		}
		return v * u.secs, true
	}
	return 0, false
}

// dayjsLayout converts a dayjs date format (as used by Mermaid's
// dateFormat) into a Go time layout. Only numeric and month-name tokens and
// a few separators are accepted, so no literal can be mistaken for a Go
// layout element.
func dayjsLayout(f string) (string, bool) {
	tokens := [...][2]string{
		{"YYYY", "2006"}, {"YY", "06"}, {"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"M", "1"},
		{"DD", "02"}, {"D", "2"}, {"HH", "15"}, {"H", "15"}, {"hh", "03"}, {"h", "3"},
		{"mm", "04"}, {"m", "4"}, {"ss", "05"}, {"s", "5"}, {"A", "PM"}, {"a", "pm"}, {"ZZ", "-0700"}, {"Z", "-07:00"},
	}
	var b strings.Builder
	for i := 0; i < len(f); {
		matched := false
		for _, t := range tokens {
			if strings.HasPrefix(f[i:], t[0]) {
				b.WriteString(t[1])
				i += len(t[0])
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if !strings.ContainsRune(" -/:.,T", rune(f[i])) {
			return "", false
		}
		b.WriteByte(f[i])
		i++
	}
	return b.String(), b.Len() > 0
}

// strftimeLayout converts a d3 strftime axisFormat into a Go layout, or
// returns "" (automatic labels) for anything it does not understand.
func strftimeLayout(f string) string {
	conv := map[byte]string{
		'Y': "2006", 'y': "06", 'm': "01", 'd': "02", 'e': "_2", 'b': "Jan", 'B': "January",
		'a': "Mon", 'A': "Monday", 'H': "15", 'I': "03", 'M': "04", 'S': "05", 'p': "PM", 'j': "002",
	}
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		c := f[i]
		switch {
		case c == '%' && i+1 < len(f):
			i++
			s, ok := conv[f[i]]
			if !ok {
				return ""
			}
			b.WriteString(s)
		case strings.IndexByte(" -/:.,", c) >= 0:
			b.WriteByte(c)
		default:
			return ""
		}
	}
	return b.String()
}

// tick steps: fixed durations, then calendar months and years.
var (
	tickSecs   = [...]float64{3600, 2 * 3600, 3 * 3600, 6 * 3600, 12 * 3600, day, 2 * day, 7 * day, 14 * day}
	tickMonths = [...]int{1, 2, 3, 6}
	tickYears  = [...]int{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500}
)

// ticks returns axis tick times between lo and hi with labels formatted
// by layout (automatic when ""), spaced at least minGap columns apart for
// a bar area of barW columns.
func ticks(lo, hi float64, barW int, layout string) ([]float64, []string) {
	span := hi - lo
	px := span / float64(barW) // seconds per column
	labelFmt := func(auto string) string {
		if layout != "" {
			return layout
		}
		return auto
	}
	gapFor := func(f string) float64 { return float64(len(time.Unix(int64(lo), 0).UTC().Format(f)) + 2) }
	var ts []float64
	var lf string
	emit := func(t float64) bool {
		if t > hi || len(ts) > barW {
			return false
		}
		if t >= lo {
			ts = append(ts, t)
		}
		return true
	}
	done := false
	for _, s := range tickSecs {
		f := labelFmt("2006-01-02")
		if s < day {
			f = labelFmt("01-02 15:04")
		}
		if s/px < gapFor(f) {
			continue
		}
		lf, done = f, true
		unit := 3600.0
		if s >= day {
			unit = day
		}
		start := math.Floor(lo/unit) * unit
		if s == 7*day || s == 14*day {
			start = math.Floor(lo/day) * day
		}
		for t := start; emit(t); {
			t += s
		}
		break
	}
	if !done {
		t0 := time.Unix(int64(lo), 0).UTC()
		for _, m := range tickMonths {
			f := labelFmt("2006-01")
			if float64(m)*30*day/px < gapFor(f) {
				continue
			}
			lf, done = f, true
			cur := time.Date(t0.Year(), t0.Month(), 1, 0, 0, 0, 0, time.UTC)
			for emit(float64(cur.Unix())) {
				cur = cur.AddDate(0, m, 0)
			}
			break
		}
	}
	if !done {
		t0 := time.Unix(int64(lo), 0).UTC()
		for _, y := range tickYears {
			f := labelFmt("2006")
			if float64(y)*365*day/px < gapFor(f) && y != tickYears[len(tickYears)-1] {
				continue
			}
			lf = f
			cur := time.Date(t0.Year()-t0.Year()%y, 1, 1, 0, 0, 0, 0, time.UTC)
			for emit(float64(cur.Unix())) {
				cur = cur.AddDate(y, 0, 0)
			}
			break
		}
	}
	labels := make([]string, len(ts))
	for i, t := range ts {
		labels[i] = time.Unix(int64(t), 0).UTC().Format(lf)
	}
	return ts, labels
}

// sectionClass colors a section's bars, skipping the red series that marks
// critical tasks.
func sectionClass(i int) styled.Class {
	if i < 0 {
		i = 0
	}
	order := [...]int{0, 1, 2, 3, 4, 5, 7}
	return styled.Series(order[i%len(order)])
}

// render draws the chart: an axis of dates, then one row per task with its
// name on the left and a bar spanning its dates; sections are group rows.
func (gc *ganttChart) render(width int) ([]styled.Line, bool) {
	lo, hi := math.Inf(1), math.Inf(-1)
	nameW := 0
	indent := 0
	if len(gc.sections) > 0 {
		indent = 2
	}
	for _, t := range gc.tasks {
		lo, hi = min(lo, t.start), max(hi, t.end)
		nameW = max(nameW, styled.Width(t.name))
	}
	if hi <= lo {
		hi = lo + day
	}
	nameW = min(nameW+indent+1, max(8, width/3))
	barW := width - nameW
	if barW < 10 {
		nameW = max(4, width-10)
		barW = width - nameW
	}
	if barW < 10 {
		return nil, false
	}
	col := func(t float64) float64 { return (t - lo) / (hi - lo) * float64(barW) }
	pad := func(n int) string { return strings.Repeat(" ", n) }

	var out []styled.Line
	if gc.title != "" {
		out = append(out, styled.Line{{Text: truncate(oneLine(gc.title), width), Class: styled.Title}}, nil)
	}
	// Axis: labels placed left to right without overlapping, then a rule
	// with a tick under every label.
	ts, labels := ticks(lo, hi, barW, gc.axis)
	labelRow := []rune(strings.Repeat(" ", barW))
	rule := []rune(strings.Repeat("─", barW))
	next := 0
	for i, t := range ts {
		c := min(int(math.Round(col(t))), barW-1)
		lw := styled.Width(labels[i])
		if c < next || c+lw > barW {
			continue
		}
		copy(labelRow[c:], []rune(labels[i]))
		rule[c] = '┬'
		next = c + lw + 1
	}
	if s := strings.TrimRight(string(labelRow), " "); s != "" {
		out = append(out, styled.Line{{Text: pad(nameW), Class: styled.Plain}, {Text: s, Class: styled.Muted}})
	}
	out = append(out, styled.Line{{Text: pad(nameW), Class: styled.Plain}, {Text: string(rule), Class: styled.Border}})

	var used [4]bool // planned, active, done, milestone
	section := -1
	for _, t := range gc.tasks {
		if t.section != section {
			section = t.section
			if section >= 0 && gc.sections[section] != "" {
				out = append(out, styled.Line{{Text: truncate(gc.sections[section], width), Class: styled.Title}})
			}
		}
		var line styled.Line
		name := truncate(t.name, nameW-indent-1)
		lead := pad(indent) + name
		cls := sectionClass(t.section)
		switch {
		case t.crit:
			cls = styled.Series6
		case t.done:
			cls = styled.Muted
		}
		var bar string
		c0 := min(int(math.Floor(col(t.start))), barW-1)
		switch {
		case t.milestone:
			c0 = min(int(math.Round(col(t.start+(t.end-t.start)/2))), barW-1)
			bar = "◆"
			used[3] = true
		default:
			c1 := min(max(int(math.Ceil(col(t.end))), c0+1), barW)
			glyph := "█"
			switch {
			case t.done:
				glyph, used[2] = "▒", true
			case t.active:
				glyph, used[1] = "▓", true
			default:
				used[0] = true
			}
			bar = strings.Repeat(glyph, c1-c0)
		}
		if lead != "" {
			line = append(line, styled.Span{Text: lead, Class: styled.Plain})
		}
		line = append(line, styled.Span{Text: pad(nameW + c0 - styled.Width(lead)), Class: styled.Plain})
		line = append(line, styled.Span{Text: bar, Class: cls})
		out = append(out, line)
	}
	if used[1] || used[2] || used[3] {
		var parts []string
		for i, s := range [...]string{"█ planned", "▓ active", "▒ done", "◆ milestone"} {
			if used[i] {
				parts = append(parts, s)
			}
		}
		if legend := strings.Join(parts, "  "); styled.Width(legend) <= width {
			out = append(out, nil, styled.Line{{Text: legend, Class: styled.Muted}})
		}
	}
	return out, true
}
