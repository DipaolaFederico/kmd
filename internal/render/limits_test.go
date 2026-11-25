package render

import (
	"math/rand"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

// shrinkOneByOne is how table widths used to be fitted: one column of
// width at a time from the widest (the first of equals), down to 4.
func shrinkOneByOne(widths []int, avail int) bool {
	for {
		sum, big := 0, 0
		for j, w := range widths {
			sum += w
			if w > widths[big] {
				big = j
			}
		}
		if sum <= avail {
			return true
		}
		if widths[big] <= 4 {
			return false
		}
		widths[big]--
	}
}

func TestFitColumnsMatchesOneByOne(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for range 5000 {
		n := 1 + rng.Intn(8)
		widths := make([]int, n)
		for j := range widths {
			widths[j] = rng.Intn(40)
		}
		avail := rng.Intn(9 * n * 5)
		want, got := slices.Clone(widths), slices.Clone(widths)
		wantOK, gotOK := shrinkOneByOne(want, avail), fitColumns(got, avail)
		if wantOK != gotOK || wantOK && !slices.Equal(want, got) {
			t.Fatalf("fitColumns(%v, %d) = %v %v, want %v %v", widths, avail, got, gotOK, want, wantOK)
		}
	}
}

// TestWideTableLinear: shrinking the columns of a table far wider than
// the screen is linear in its width (it took one step per column of
// excess width, each over the thousand columns: 8 s for a 16 MiB header).
func TestWideTableLinear(t *testing.T) {
	table := func(cell int) *md.Node {
		head := "|" + strings.Repeat(strings.Repeat("a", cell)+"|", 1000)
		return md.Parse(head+"\n|"+strings.Repeat("-|", 1000)+"\n|x|\n", md.GFM)
	}
	doc := table(4000)
	start := time.Now()
	_ = Render(doc, Options{Width: 80})
	if d := time.Since(start); d > testlimit.Scale()*time.Second/2 {
		t.Errorf("a 4 MB table header took %v", d)
	}
	testlimit.Linear(t, "wide table", 1<<12, func(n int) func() {
		doc := table(n)
		return func() { _ = Render(doc, Options{Width: 80}) }
	})
}

// TestStackedTableLabels: a table drawn as records repeats short labels,
// not whole headers: 20 headers of 2,000 bytes over 1,000 rows printed
// 40 MB (rows × header bytes, from a 40 KB document). Labels are short in
// bytes too: a header of combining marks is one column wide (#72 SR-06).
func TestStackedTableLabels(t *testing.T) {
	table := func(header string, rows int) string {
		return "|" + strings.Repeat(header+"|", 20) + "\n|" + strings.Repeat("-|", 20) + "\n" + strings.Repeat("|x|\n", rows)
	}
	for name, header := range map[string]func(n int) string{
		"long":       func(n int) string { return strings.Repeat("a", n) },
		"zero-width": func(n int) string { return "a" + strings.Repeat("\u0301", n) },
	} {
		src := table(header(2000), 1000)
		start := time.Now()
		out := RenderString(src, Options{Width: 80})
		if d := time.Since(start); d > testlimit.Scale()*time.Second/2 {
			t.Errorf("%s: took %v", name, d)
		}
		if len(out) > 1<<20 {
			t.Errorf("%s: %d bytes of output", name, len(out))
		}
		if !strings.Contains(out, "…: x\n") {
			t.Errorf("%s: no elided label in\n%.500s", name, out)
		}
		// rows × header bytes: both grow with n
		testlimit.Linear(t, "stacked table, "+name+" headers", 1<<14, func(n int) func() {
			doc := md.Parse(table(header(n), n), md.GFM)
			return func() { _ = Render(doc, Options{Width: 80}) }
		})
	}
	label := strings.Repeat("a", maxLabelWidth-1) + "…: x"
	if out := RenderString(table(strings.Repeat("a", 2000), 1), Options{Width: 80}); !strings.Contains(out, "\n  "+label+"\n") {
		t.Errorf("no elided label %q in\n%.500s", label, out)
	}
	if got := RenderString("| h1 | h2 |\n|-|-|\n| x | y |", Options{Width: 8}); !strings.Contains(got, "h1: x") || !strings.Contains(got, "h2: y") {
		t.Errorf("short labels changed:\n%s", got)
	}
}

// TestNaturalWidth: a cell is measured as wide as its widest line laid out
// without a width limit.
func TestNaturalWidth(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	pieces := []string{"a", "bb", " ", "  ", "\n", "é", "漢", "́", "x y", " z "}
	for range 5000 {
		var runs []run
		for range rng.Intn(8) {
			runs = append(runs, run{text: pieces[rng.Intn(len(pieces))], st: Style{Attr: uint8(rng.Intn(2))}})
		}
		want := 0
		for _, l := range wrap(runs, 1<<20) {
			want = max(want, lineWidth(l))
		}
		if got := naturalWidth(runs); got != want {
			t.Fatalf("naturalWidth(%q) = %d, want %d", runsText(runs), got, want)
		}
	}
}

// allocated returns the bytes allocated by f, and its duration.
func allocated(f func()) (uint64, time.Duration) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	f()
	d := time.Since(start)
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc, d
}

// TestLayoutBudget: a top-level block lays out at most maxBlockRuns runs,
// then shows a notice. Laid out whole, an 8 MiB code block of one-character
// lines took 2 GB, and a long paragraph in nested quotes minutes (#72
// SR-08). Past the budget, twice the input costs no more memory.
func TestLayoutBudget(t *testing.T) {
	notice := "the rest of this block is too large to show"
	for name, src := range map[string]func(k int) string{
		"code block":       func(k int) string { return "```\n" + strings.Repeat("a\n", k<<22) + "```\n" },
		"code in a list":   func(k int) string { return "- x\n\n  ```go\n" + strings.Repeat("  a+b\n", k<<20) + "  ```\n" },
		"code blocks":      func(k int) string { return "- x\n" + strings.Repeat("  ```\n  a\n  b\n  ```\n", k<<17) },
		"one long line":    func(k int) string { return "```\n" + strings.Repeat("a+", k<<22) + "\n```\n" },
		"nested paragraph": func(k int) string { return strings.Repeat("> ", 30) + strings.Repeat("a ", k<<22) },
		"table":            func(k int) string { return "|a|b|\n|-|-|\n" + strings.Repeat("|x|y|\n", k*120_000) },
	} {
		var used [3]uint64 // by input size
		for k := 1; k <= 2; k++ {
			doc := md.Parse(src(k), md.GFM)
			var out string
			bytes, d := allocated(func() { out = Render(doc, Options{Width: 80, Color: true}).Text })
			if d > testlimit.Scale()*2*time.Second {
				t.Errorf("%s: %v", name, d)
			}
			if !strings.Contains(out, notice) {
				t.Errorf("%s: no notice", name)
			}
			used[k] = bytes
		}
		if used[2] > used[1]+used[1]/4 { // bounded by the budget, not by the input
			t.Errorf("%s: %d MB allocated, %d MB for twice the input", name, used[1]>>20, used[2]>>20)
		}
	}
	src := strings.Repeat("a ", 4<<20) + "\n\nafter\n"
	if out := RenderString(src, Options{Width: 80}); strings.Contains(out, notice) || !strings.Contains(out, "after") {
		t.Errorf("a long paragraph within the budget was cut")
	}
	for name, unit := range map[string]string{"code lines": "a\n", "paragraph words": "a "} {
		testlimit.Linear(t, name, 1<<22, func(n int) func() {
			src := strings.Repeat(unit, n)
			if unit == "a\n" {
				src = "```\n" + src + "```\n"
			}
			doc := md.Parse(strings.Repeat("> ", 20)+src, md.GFM)
			return func() { _ = Render(doc, Options{Width: 80}) }
		})
	}
}

// TestOutputBudget: a document renders at most maxOutput bytes, then a
// notice.
func TestOutputBudget(t *testing.T) {
	src := strings.Repeat("```\n"+strings.Repeat("a\n", 1<<20)+"```\n\n", 6)
	out := RenderString(src, Options{Width: 80, Color: true})
	if len(out) > maxOutput+4096 {
		t.Errorf("%d bytes of output", len(out))
	}
	if !strings.HasSuffix(out, truncatedNotice+"\n\n") {
		t.Errorf("no notice at the end: %q", out[max(len(out)-200, 0):])
	}
}

// TestShortTableRows: a row with fewer cells than the header is drawn
// padded in a grid, and with only its own cells as a record: the parser
// no longer makes padding cells, and records of padding printed a line per
// missing cell (#72 SR-28).
func TestShortTableRows(t *testing.T) {
	head := func(cols int) string {
		return "|" + strings.Repeat(" h |", cols) + "\n|" + strings.Repeat("-|", cols) + "\n"
	}
	short := RenderString(head(2)+"| 1 |\n", Options{Width: 80})
	if padded := RenderString(head(2)+"| 1 | |\n", Options{Width: 80}); short != padded {
		t.Errorf("grid: short row\n%s\npadded row\n%s", short, padded)
	}
	records := RenderString(head(30)+"| 1 |\n| 2 |\n", Options{Width: 80})
	if n := strings.Count(records, "h:"); n != 2 {
		t.Errorf("records: %d labels, want one per cell:\n%s", n, records)
	}
	src := head(1000) + strings.Repeat("|\n", 600)
	start := time.Now()
	out := RenderString(src, Options{Width: 80})
	if d := time.Since(start); d > testlimit.Scale()*time.Second/2 {
		t.Errorf("records of empty rows: took %v", d)
	}
	if len(out) > 64*len(src) {
		t.Errorf("records of empty rows: %d bytes of output from %d", len(out), len(src))
	}
}

// TestInlineBuiltWithinBudget: the runs of a paragraph count against the
// layout budget as they are made: once a block has spent its budget, a
// paragraph later in it costs nothing.
func TestInlineBuiltWithinBudget(t *testing.T) {
	spent := "- x\n\n  ```\n" + strings.Repeat("  a\n", 1<<19) + "  ```\n"
	para := "\n  " + strings.Repeat("*a* b ", 1<<16) + "\n" // 131,072 runs
	cost := func(src string) uint64 {
		doc := md.Parse(src, md.GFM)
		bytes, _ := allocated(func() { _ = Render(doc, Options{Width: 80}) })
		return bytes
	}
	if extra := int64(cost(spent+para)) - int64(cost(spent)); extra > 4<<20 {
		t.Errorf("a paragraph past the budget cost %d MB", extra>>20)
	}
}

// TestTOCBudget: the outline is laid out within the layout budget and
// painted as it goes, like the document: a heading of millions of words
// is laid out in linear time, and the headings after it still show.
func TestTOCBudget(t *testing.T) {
	doc := md.Parse("# "+strings.Repeat("a ", 4<<20)+"\n\n## b\n", md.GFM)
	start := time.Now()
	out := TOC(doc, Options{Width: 80})
	if d := time.Since(start); d > testlimit.Scale()*time.Second {
		t.Errorf("took %v", d)
	}
	if !strings.HasSuffix(out, "1.1  b\n") {
		t.Errorf("the next heading is missing: %q", out[max(len(out)-300, 0):])
	}
	testlimit.Linear(t, "outline", 1<<22, func(n int) func() {
		doc := md.Parse("# "+strings.Repeat("a ", n), md.GFM)
		return func() { _ = TOC(doc, Options{Width: 80}) }
	})
}
