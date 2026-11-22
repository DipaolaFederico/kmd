package render

import (
	"fmt"
	"io"

	irender "github.com/DipaolaFederico/kmd/internal/render"
)

// ErrUnknownTheme is returned (wrapped) by [LoadTheme] for a name that is
// neither a bundled theme nor a theme file.
var ErrUnknownTheme = irender.ErrUnknownTheme

// Theme is a colour theme: one of the bundled themes or a theme file. It is
// immutable and safe to share between goroutines. A nil *Theme means
// DefaultTheme(true), and so does the zero Theme.
type Theme struct{ t *irender.Theme }

func (t *Theme) internal() *irender.Theme {
	if t == nil {
		return nil
	}
	return t.t
}

// Name returns the theme's name.
func (t *Theme) Name() string {
	if t == nil || t.t == nil {
		return irender.Dark.Name
	}
	return t.t.Name
}

// Dark reports whether the theme is meant for a dark background.
func (t *Theme) Dark() bool { return t == nil || t.t == nil || t.t.Dark }

// DefaultTheme returns the default theme for a dark (tokyo-night) or light
// (tokyo-night-day) terminal background.
func DefaultTheme(dark bool) *Theme {
	t := irender.Light
	if dark {
		t = irender.Dark
	}
	return &Theme{&t}
}

// Themes lists the bundled themes, followed by the names of the *.theme
// files found in dirs (in order, without duplicates). Unreadable
// directories are skipped.
func Themes(dirs ...string) []string {
	names := irender.BuiltinThemes()
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		seen[n] = true
	}
	for _, n := range irender.UserThemes(dirs) {
		if !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	return names
}

// LoadTheme returns the theme called name: a path to a theme file (a name
// ending in ".theme" or containing a path separator), else the first
// <dir>/<name>.theme file found in dirs, else a bundled theme. Theme files
// may inherit from bundled themes and from other theme files in dirs.
//
// Only the named file and files in dirs are read; they must be regular
// files of at most 64 KiB. Because a name can be a path, do not pass
// untrusted input as name: check it against [Themes] first. A name that
// matches nothing returns an error wrapping [ErrUnknownTheme].
func LoadTheme(name string, dirs ...string) (*Theme, error) {
	t, err := irender.LoadTheme(name, dirs)
	if err != nil {
		return nil, err
	}
	return &Theme{t}, nil
}

// ParseTheme reads a theme in theme file syntax from r (at most 64 KiB)
// and names it name. Nothing else is read: the theme may only inherit
// from a bundled theme.
//
// The syntax is "key = value" lines and "#" comments; run
// "kmd --theme-template NAME" to see every key of a bundled theme.
func ParseTheme(name string, r io.Reader) (*Theme, error) {
	data, err := io.ReadAll(io.LimitReader(r, 64<<10+1))
	if err != nil {
		return nil, fmt.Errorf("theme %s: %w", name, err)
	}
	t, err := irender.ParseTheme(name, string(data))
	if err != nil {
		return nil, err
	}
	return &Theme{t}, nil
}
