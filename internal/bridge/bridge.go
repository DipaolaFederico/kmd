// Package bridge lets the kmd command use the public render package's
// Options and Theme while still reaching renderer features that are not
// public API (navigation data for the viewer, slides, sections, unconfined
// images). Package render fills in the functions when it is initialised, so
// the command and the library share one conversion from public options to
// internal ones.
package bridge

import (
	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
)

// Config is what a public render.Options resolves to.
type Config struct {
	Render render.Options
	Parse  md.Options
}

// Resolve converts a public render.Options value.
var Resolve func(opts any) (Config, error)
