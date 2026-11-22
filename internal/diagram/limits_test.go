package diagram

import "github.com/DipaolaFederico/kmd/internal/testlimit"

// raceFactor scales time limits in tests (race detector, KMD_TEST_TIME_SCALE).
var raceFactor = testlimit.Scale()
