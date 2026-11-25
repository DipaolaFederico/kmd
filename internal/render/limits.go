package render

import "strconv"

// Output limits. A document is laid out one top-level block at a time and
// each block is painted before the next is laid out, so memory holds one
// block's layout and the output. Both are bounded here, ending in a notice:
// the soft memory limit set by main is not a bound (past it the garbage
// collector runs continuously, and the library has none).
const (
	// maxBlockRuns bounds the styled runs (about 64 bytes each) one
	// top-level block lays out: about 64 MiB, hundreds of thousands of
	// lines. A 16 MiB code block of one-character lines would otherwise
	// take gigabytes before the first line is painted.
	maxBlockRuns = 1 << 20
	// maxOutput bounds the bytes of rendered text per Render call; real
	// documents take three to six times their size.
	maxOutput = 64 << 20
)

// charge takes a line of n runs from the block's layout budget, plus a
// run for each container around it (quote bars and list indents are
// prepended to every line), and reports whether it fits. Once a line does
// not fit, nothing more of the block is laid out.
func (r *renderer) charge(n int) bool {
	if r.over {
		return false
	}
	cost := n + 1 + r.nest // the line itself: about one run
	if r.laid+cost > maxBlockRuns {
		r.over = true
		return false
	}
	r.laid += cost
	return true
}

// wrap is wrap within the block's layout budget: lines past it are left
// out, and so is the rest of the block.
func (r *renderer) wrap(runs []run, width int) []rline {
	if r.over {
		return nil
	}
	// a line costs at least two runs, plus one per container
	lines := wrapUpTo(runs, width, (maxBlockRuns-r.laid)/(2+r.nest)+1)
	for i, l := range lines {
		if !r.charge(len(l)) {
			clear(lines[i:]) // release the runs left out
			return lines[:i:i]
		}
	}
	return lines
}

// finish ends the layout of a top-level block: its lines, with a notice
// when the layout budget ran out; the budget starts again for the next.
func (r *renderer) finish(lines []rline) []rline {
	if r.over || r.cut {
		lines = append(lines, rline{{text: "[kmd: the rest of this block is too large to show]", st: Style{Fg: r.th.Muted}}})
	}
	r.laid, r.over, r.cut = 0, false, false
	return lines
}

// truncatedNotice ends output cut at maxOutput.
var truncatedNotice = "[kmd: output truncated after " + strconv.Itoa(maxOutput>>20) + " MiB]"
