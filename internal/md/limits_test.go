package md

import (
	"time"

	"github.com/DipaolaFederico/kmd/internal/testlimit"
)

// raceFactor scales time limits in tests (race detector, coverage,
// KMD_TEST_TIME_SCALE). A function, not a variable: the coverage mode is
// only known once the tests start, after package variables are set.
func raceFactor() time.Duration { return testlimit.Scale() }
