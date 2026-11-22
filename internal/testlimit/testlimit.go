// Package testlimit scales the wall-clock limits of tests. Those limits
// only exist to catch super-linear behaviour on hostile input, so they are
// generous, but a slow or busy machine (the race detector, emulated
// architectures, distribution build farms) needs more:
// KMD_TEST_TIME_SCALE=N multiplies every limit by N. Only tests import it.
package testlimit

import (
	"os"
	"strconv"
	"time"
)

// maxScale keeps a typo from disabling the limits altogether.
const maxScale = 1000

// Scale is the factor test time limits are multiplied by.
func Scale() time.Duration { return scale(os.Getenv("KMD_TEST_TIME_SCALE")) }

func scale(env string) time.Duration {
	f := raceFactor
	if n, err := strconv.Atoi(env); err == nil && n > 0 {
		f *= min(n, maxScale)
	}
	return time.Duration(f)
}
