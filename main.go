// Command kmd renders Markdown files as rich previews in the terminal.
package main

import (
	"os"
	"runtime/debug"
	"time"

	"github.com/DipaolaFederico/kmd/internal/cli"
	"github.com/DipaolaFederico/kmd/internal/term"
)

func main() {
	// kmd is short-lived: collect rarely (typical documents never trigger
	// a GC) but keep the collector on so huge inputs cannot grow unbounded.
	debug.SetGCPercent(400)
	debug.SetMemoryLimit(512 << 20)

	cols, rows, outTTY := term.Size(os.Stdout.Fd())
	_, _, inTTY := term.Size(os.Stdin.Fd())
	env := cli.Env{
		Args:      os.Args[1:],
		Stdin:     os.Stdin,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
		Getenv:    os.Getenv,
		StdinTTY:  inTTY,
		StdoutTTY: outTTY,
		Cols:      cols,
		Rows:      rows,
		// bounded wait for terminals that answer neither query; local
		// ones answer in well under a millisecond
		LightBackground: func() (bool, bool) { return term.LightBackground("/dev/tty", 100*time.Millisecond) },
	}
	if pw, ph, ok := term.PixelSize(os.Stdout.Fd()); ok && cols > 0 && rows > 0 {
		env.CellW, env.CellH = pw/cols, ph/rows
	}
	os.Exit(cli.Run(env))
}
