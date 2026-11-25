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
// not whole headers: 20 headers of 2,000 bytes over 1,000 empty rows
// printed 40 MB (rows × header bytes, from a 40 KB document).
func TestStackedTableLabels(t *testing.T) {
	head := "|" + strings.Repeat(strings.Repeat("a", 2000)+"|", 20) + "\n|" + strings.Repeat("-|", 20) + "\n"
	start := time.Now()
	out := RenderString(head+strings.Repeat("|\n", 1000), Options{Width: 80})
	if d := time.Since(start); d > testlimit.Scale()*time.Second/2 {
		t.Errorf("took %v", d)
	}
	if len(out) > 2<<20 {
		t.Errorf("%d bytes of output", len(out))
	}
	label := strings.Repeat("a", maxLabelWidth-1) + "…:"
	if !strings.Contains(out, "\n  "+label+"\n") {
		t.Errorf("no elided label %q in\n%.500s", label, out)
	}
	if got := RenderString("| h1 | h2 |\n|-|-|\n| x |", Options{Width: 8}); !strings.Contains(got, "h1: x") || !strings.Contains(got, "h2:") {
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
// then shows a notice. Laid out whole, a 8 MiB code block of one-character
// lines took 2 GB, and a long paragraph in nested quotes minutes.
func TestLayoutBudget(t *testing.T) {
	notice := "the rest of this block is too large to show"
	for name, src := range map[string]string{
		"code block":       "```\n" + strings.Repeat("a\n", 4<<20) + "```\n",
		"code in a list":   "- x\n\n  ```go\n" + strings.Repeat("  a+b\n", 1<<20) + "  ```\n",
		"code blocks":      "- x\n" + strings.Repeat("  ```\n  a\n  b\n  ```\n", 1<<17),
		"one long line":    "```\n" + strings.Repeat("a+", 4<<20) + "\n```\n",
		"nested paragraph": strings.Repeat("> ", 30) + strings.Repeat("a ", 4<<20),
		"paragraph":        strings.Repeat("a ", 4<<20) + "\n\nafter\n",
		"table":            "|a|b|\n|-|-|\n" + strings.Repeat("|x|y|\n", 200_000),
	} {
		doc := md.Parse(src, md.GFM)
		var out string
		bytes, d := allocated(func() { out = Render(doc, Options{Width: 80, Color: true}).Text })
		if d > testlimit.Scale()*2*time.Second {
			t.Errorf("%s: %v", name, d)
		}
		if bytes > 1<<30 { // bounded by the budget, not by the input
			t.Errorf("%s: %d MB allocated", name, bytes>>20)
		}
		if want := name != "paragraph"; strings.Contains(out, notice) != want {
			t.Errorf("%s: notice shown %v, want %v", name, !want, want)
		}
		if name == "paragraph" && !strings.Contains(out, "after") {
			t.Errorf("%s: the next block is missing", name)
		}
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

// TestShortTableRows: a row with fewer cells than the header is drawn as
// if padded with empty cells (the parser no longer makes them).
func TestShortTableRows(t *testing.T) {
	for _, cols := range []int{2, 30} { // a grid, and records
		head := "|" + strings.Repeat(" h |", cols) + "\n|" + strings.Repeat("-|", cols) + "\n"
		short := RenderString(head+"| 1 |\n", Options{Width: 80})
		padded := RenderString(head+"| 1 |"+strings.Repeat(" |", cols-1)+"\n", Options{Width: 80})
		if short != padded {
			t.Errorf("%d columns: short row\n%s\npadded row\n%s", cols, short, padded)
		}
	}
}
