// Package testlimit scales the wall-clock limits of tests. Those limits
// only exist to catch super-linear behaviour on hostile input, so they are
// generous, but a slow or busy machine (the race detector, coverage
// instrumentation, emulated architectures, distribution build farms) needs
// more:
// KMD_TEST_TIME_SCALE=N multiplies every limit by N. Only tests import it.
package testlimit

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// maxScale keeps a typo from disabling the limits altogether.
const maxScale = 1000

// coverFactor: -covermode=atomic makes every statement an atomic add, about
// 2x slower, and `make cover` runs all packages at once on the same cores.
const coverFactor = 4

// Scale is the factor test time limits are multiplied by. Call it from a
// test, not in a package variable: testing.CoverMode is empty until the
// tests start.
func Scale() time.Duration {
	return scale(os.Getenv("KMD_TEST_TIME_SCALE"), testing.CoverMode() != "")
}

func scale(env string, cover bool) time.Duration {
	f := raceFactor
	if cover {
		f *= coverFactor
	}
	if n, err := strconv.Atoi(env); err == nil && n > 0 {
		f *= min(n, maxScale)
	}
	return time.Duration(f)
}
