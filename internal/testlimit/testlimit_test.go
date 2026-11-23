package testlimit

import (
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
