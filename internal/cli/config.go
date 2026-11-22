package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

// config holds settings from the config file; empty values mean "default".
type config struct {
	path       string
	Theme      string
	DarkTheme  string
	LightTheme string
	Width      int
	MaxWidth   int
	Color      string
	Images     string
	ImageRows  int
	Hyperlinks string
	Pager      string
	Math       *bool
	Diagrams   *bool
	Mouse      *bool
	View       string
}

// configPath returns $KMD_CONFIG, or $XDG_CONFIG_HOME/kmd/config, or ~/.config/kmd/config.
func configPath(getenv func(string) string) string {
	if p := getenv("KMD_CONFIG"); p != "" {
		return p
	}
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "kmd", "config")
	}
	if h := getenv("HOME"); h != "" {
		return filepath.Join(h, ".config", "kmd", "config")
	}
	return ""
}

const configTemplate = `# kmd configuration (key = value). Command-line flags take precedence.
# theme       = auto            # auto | any name from: kmd --list-themes
# dark-theme  = tokyo-night     # used by theme = auto on dark terminals
# light-theme = tokyo-night-day # used by theme = auto on light terminals
# width       = 0               # fixed render width (0 = terminal width)
# max-width   = 110             # cap for the automatic width
# color       = auto            # auto | always | never
# images      = auto            # auto | blocks | kitty | iterm2 | sixel | none
# image-rows  = 20              # maximum image height in terminal rows
# hyperlinks  = auto            # auto | always | never
# pager       = builtin         # long documents: builtin (viewer that follows
#                               # links), a command such as "less -R", or "-"
# mouse       = true            # builtin viewer: click links, scroll with the wheel
# view        = auto            # auto: viewer for long documents | always
# math        = true            # render LaTeX formulas
# diagrams    = true            # render mermaid / graphviz diagrams
`

func loadConfig(path string) (config, error) {
	c := config{path: path}
	if path == "" {
		return c, nil
	}
	// #nosec G304 -- the config path comes from the user's environment.
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 64<<10+1))
	if err != nil {
		return c, err
	}
	if len(data) > 64<<10 {
		return c, fmt.Errorf("%s: config file too large", path)
	}
	for i, line := range strings.Split(string(data), "\n") {
		line = render.StripComment(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return c, fmt.Errorf("%s:%d: expected key = value", path, i+1)
		}
		if err := c.set(strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)); err != nil {
			return c, fmt.Errorf("%s:%d: %v", path, i+1, err)
		}
	}
	return c, nil
}

func parseBool(v string) (*bool, error) {
	switch strings.ToLower(v) {
	case "true", "yes", "on", "1":
		b := true
		return &b, nil
	case "false", "no", "off", "0":
		b := false
		return &b, nil
	}
	return nil, fmt.Errorf("invalid boolean %q", v)
}

func (c *config) set(k, v string) error {
	var err error
	atoi := func() int {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			err = fmt.Errorf("invalid number %q for %s", v, k)
		}
		return n
	}
	switch k {
	case "theme":
		c.Theme = v
	case "dark-theme":
		c.DarkTheme = v
	case "light-theme":
		c.LightTheme = v
	case "width":
		c.Width = atoi()
	case "max-width":
		c.MaxWidth = atoi()
	case "image-rows":
		c.ImageRows = atoi()
	case "color":
		c.Color, err = v, checkWhen(k, v)
	case "hyperlinks":
		c.Hyperlinks, err = v, checkWhen(k, v)
	case "images":
		c.Images = v
		if v != "auto" {
			_, err = termimg.ParseProtocol(v)
		}
	case "pager":
		c.Pager = v
		if v == "" {
			c.Pager = "-"
		}
	case "math":
		c.Math, err = parseBool(v)
	case "diagrams":
		c.Diagrams, err = parseBool(v)
	case "mouse":
		c.Mouse, err = parseBool(v)
	case "view":
		c.View = v
		if v != "auto" && v != "always" {
			err = fmt.Errorf("invalid view %q (auto | always)", v)
		}
	default:
		return fmt.Errorf("unknown setting %q", k)
	}
	return err
}

func checkWhen(k, v string) error {
	switch v {
	case "auto", "always", "never":
		return nil
	}
	return fmt.Errorf("invalid %s %q (want auto, always or never)", k, v)
}

// describe returns the effective settings for `kmd --config`.
func (c config) describe(eff map[string]string) string {
	keys := make([]string, 0, len(eff))
	for k := range eff {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-12s %s\n", k, eff[k])
	}
	return b.String()
}
