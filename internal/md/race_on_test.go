//go:build race

package md

// raceFactor scales time limits in tests: the race detector slows code
// down 5–10x, and the limits only exist to catch super-linear behaviour.
const raceFactor = 10
