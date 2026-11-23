package testlimit

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestScale(t *testing.T) {
	for env, want := range map[string]int{"": 1, "x": 1, "0": 1, "-3": 1, "4": 4, "999999": maxScale} {
		if got := scale(env, false); got != scaleOf(want) {
			t.Errorf("%q: %d, want %d", env, got, scaleOf(want))
		}
	}
	if got := scale("2", true); got != scaleOf(2*coverFactor) {
		t.Errorf("coverage: %d, want %d", got, scaleOf(2*coverFactor))
	}
	t.Setenv("KMD_TEST_TIME_SCALE", "2")
	want := scaleOf(2)
	if testing.CoverMode() != "" {
		want *= coverFactor
	}
	if Scale() != want {
		t.Error("Scale does not read the environment")
	}
}

func scaleOf(n int) time.Duration { return time.Duration(raceFactor * n) }

// recorder is a testing.TB that records failures instead of failing.
type recorder struct {
	testing.TB
	failed bool
	msg    string
}

func (r *recorder) Helper()             {}
func (r *recorder) Logf(string, ...any) {}
func (r *recorder) Errorf(format string, args ...any) {
	r.failed, r.msg = true, fmt.Sprintf(format, args...)
}

var sink int

// spin does n units of pure-Go work.
func spin(n int) {
	s := 0
	for i := range n {
		s += i ^ s>>3
	}
	sink += s
}

func TestLinear(t *testing.T) {
	linearWork := func(n int) func() { return func() { spin(1000 * n) } }
	r := &recorder{TB: t}
	linear(r, "linear", 1<<20, linearWork, minSample)
	if r.failed {
		t.Errorf("linear work rejected: %s", r.msg)
	}
	quadratic := func(n int) func() {
		return func() {
			for range n {
				spin(n)
			}
		}
	}
	r = &recorder{TB: t}
	linear(r, "quadratic", 1<<20, quadratic, minSample/4)
	if !r.failed || !strings.Contains(r.msg, "quadratic: time grows") {
		t.Errorf("quadratic work accepted: %q", r.msg)
	}
	// work capped by maxN is repeated until it can be timed
	r = &recorder{TB: t}
	linear(r, "small", 64, linearWork, minSample)
	if r.failed {
		t.Errorf("small linear work rejected: %s", r.msg)
	}
	// the exported entry point passes linear work (and skips under -race)
	Linear(t, "exported", 1<<20, linearWork)
}
