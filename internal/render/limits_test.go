package render

import (
	"math/rand"
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
