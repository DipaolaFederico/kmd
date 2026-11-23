package testlimit

import (
	"math"
	"runtime"
	"runtime/debug"
	"testing"
	"time"
)

// Growth-ratio check. A wall-clock limit misses a quadratic whose inner
// loop is an optimised standard-library call (strings.Count, strings.Index:
// SIMD, untouched by coverage instrumentation): at the sizes tests use it
// stays far under any limit. How the time grows when the input grows does
// not depend on the machine: linear work takes ≈ 2× per doubling of the
// input, quadratic work ≈ 4×.
const (
	// minSample is the shortest timed sample: n grows until one run of the
	// work takes this long (or reaches maxN), and shorter runs are repeated.
	minSample = 2 * time.Millisecond
	// samples: the best of this many samples is kept, to ignore
	// preemptions, GC pauses and other processes competing for the CPU.
	samples = 3
	// A doubling of n costing more than suspect is measured again at a
	// larger n, up to rounds times; more than maxDoubling (between linear,
	// 2, and quadratic, 4) fails.
	suspect     = 2.2
	maxDoubling = 2.8
	rounds      = 3
)

// Linear fails t when the time of some work grows clearly faster than
// linearly with n. setup(n) builds an input whose size is proportional to
// n (outside the timing) and returns the work to time on it; the work may
// run many times. maxN bounds n (input caps, memory).
//
// n doubles from 1 until one run takes a few milliseconds, then the work
// is timed at n, 2n and 4n. Only the smaller of the two ratios counts: a
// quadratic slows every doubling, while the memory hierarchy makes linear
// work jump at some sizes, one doubling at a time. A suspect ratio is
// measured again with four times longer runs, hence at a larger n, where a
// quadratic still hidden under the linear part of the work shows (coverage
// instrumentation slows the linear part, not the standard library's
// assembly). The test fails on two ratios above maxDoubling, or on one in
// the last round, so one unlucky measurement does not fail it.
//
// The check is skipped under the race detector: it slows the instrumented
// Go code ten times, so finding the same quadratics would need runs ten
// times longer. The absolute limits still apply under -race, and plain and
// coverage runs (`make cover`) keep the check.
func Linear(t testing.TB, name string, maxN int, setup func(n int) func()) {
	t.Helper()
	if raceFactor > 1 {
		return
	}
	linear(t, name, maxN, setup, minSample)
}

func linear(t testing.TB, name string, maxN int, setup func(n int) func(), base time.Duration) {
	t.Helper()
	var r [2]float64
	n, fails := 0, 0
	for i := range rounds {
		r, n = growth(maxN, setup, base<<(2*i))
		m := min(r[0], r[1])
		if m <= suspect {
			break
		}
		if m > maxDoubling {
			fails++
		}
		if fails == 2 || (i == rounds-1 && m > maxDoubling) {
			t.Errorf("%s: time grows %.1f× and %.1f× per doubling of the input from n=%d (linear ≈ 2×, quadratic ≈ 4×, limit %.1f×)", name, r[0], r[1], n, maxDoubling)
			return
		}
	}
	t.Logf("%s: %.2f× and %.2f× per doubling from n=%d", name, r[0], r[1], n)
}

// growth returns the time ratios from n to 2n and from 2n to 4n, and the
// n it chose.
func growth(maxN int, setup func(n int) func(), base time.Duration) ([2]float64, int) {
	// One thread, and no garbage collection while timing: a collection
	// starting at some size but not at half of it would look like
	// super-linear growth, and the collector's workers on idle cores make
	// the timings erratic. Each sample starts with a collection instead.
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	defer debug.SetGCPercent(debug.SetGCPercent(-1))
	n := 1
	for 8*n <= maxN && once(setup(n)) < base {
		n *= 2
	}
	n = min(n, max(maxN/4, 1))
	var work [3]func()
	var reps [3]int
	for i := range work {
		work[i] = setup(n << i)
	}
	// repeat short work so that a sample lasts at least base
	reps[0] = 1
	if d := once(work[0]); d < base {
		reps[0] = int(base/max(d, time.Microsecond)) + 1
	}
	reps[1], reps[2] = max(reps[0]/2, 1), max(reps[0]/4, 1)
	// interleaved, so that a slow start (CPU frequency, caches) or a busy
	// moment does not favour one size
	var t [3]float64
	for i := range t {
		t[i] = math.Inf(1)
	}
	for range samples {
		for i := range work {
			t[i] = min(t[i], perRun(work[i], reps[i]))
		}
	}
	const tiny = 1e-9 // seconds: no division by zero on a coarse clock
	return [2]float64{t[1] / max(t[0], tiny), t[2] / max(t[1], tiny)}, n
}

func once(work func()) time.Duration {
	start := time.Now()
	work()
	return time.Since(start)
}

// perRun returns the time per run, in seconds, of reps runs.
func perRun(work func(), reps int) float64 {
	runtime.GC()
	start := time.Now()
	for range reps {
		work()
	}
	return time.Since(start).Seconds() / float64(reps)
}
