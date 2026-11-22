package render

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Palette is the small set of base colors a theme is built from. Every
// rendering role (headings, syntax, math, diagrams…) is derived from it, so
// a custom theme only needs a handful of colors.
type Palette struct {
	Bg, Fg, Bright, Muted, Comment, Border, Surface, OnAccent     Color
	Red, Orange, Yellow, Green, Cyan, Blue, Purple, Teal, Magenta Color
}

// Theme holds the color of every rendering role.
type Theme struct {
	Name string
	Dark bool

	Bg, Fg Color // page colors (used by HTML export and previews)

	H1Fg, H1Bg, H2, H3, H4, H5     Color
	Strong, Code, CodeBg, Link     Color
	Image, Quote, QuoteBar, Border Color
	Muted, Bullet, Number          Color
	TaskDone, TaskOpen, TableHead  Color
	Keyword, String, Num, Comment  Color
	Func, Type, Added, Removed     Color
	Note, Tip, Important, Warning  Color
	Caution                        Color

	MathVar, MathNum, MathOp, MathSymbol, MathFunc       Color
	DiagBorder, DiagNode, DiagEdge, DiagArrow, DiagLabel Color
	DiagTitle                                            Color
	Series                                               [8]Color
}

// Theme derives every role from the palette.
func (p Palette) Theme(name string, dark bool) Theme {
	return Theme{
		Name: name, Dark: dark, Bg: p.Bg, Fg: p.Fg,
		H1Fg: p.OnAccent, H1Bg: p.Blue, H2: p.Purple, H3: p.Cyan, H4: p.Green, H5: p.Yellow,
		Strong: p.Bright, Code: p.Orange, CodeBg: p.Surface, Link: p.Blue,
		Image: p.Teal, Quote: p.Muted, QuoteBar: p.Blue, Border: p.Border,
		Muted: p.Muted, Bullet: p.Purple, Number: p.Blue,
		TaskDone: p.Green, TaskOpen: p.Muted, TableHead: p.Cyan,
		Keyword: p.Purple, String: p.Green, Num: p.Orange, Comment: p.Comment,
		Func: p.Blue, Type: p.Teal, Added: p.Green, Removed: p.Red,
		Note: p.Blue, Tip: p.Green, Important: p.Purple, Warning: p.Yellow, Caution: p.Red,
		MathVar: p.Fg, MathNum: p.Orange, MathOp: p.Cyan, MathSymbol: p.Purple, MathFunc: p.Blue,
		DiagBorder: p.Blue, DiagNode: p.Fg, DiagEdge: p.Muted, DiagArrow: p.Cyan, DiagLabel: p.Yellow, DiagTitle: p.Purple,
		Series: [8]Color{p.Blue, p.Green, p.Orange, p.Purple, p.Cyan, p.Yellow, p.Red, p.Magenta},
	}
}

type colorKey struct {
	key string
	ptr *Color
}

func (p *Palette) keys() []colorKey {
	return []colorKey{
		{"bg", &p.Bg}, {"fg", &p.Fg}, {"bright", &p.Bright}, {"muted", &p.Muted}, {"comment", &p.Comment},
		{"border", &p.Border}, {"surface", &p.Surface}, {"on-accent", &p.OnAccent},
		{"red", &p.Red}, {"orange", &p.Orange}, {"yellow", &p.Yellow}, {"green", &p.Green},
		{"cyan", &p.Cyan}, {"blue", &p.Blue}, {"purple", &p.Purple}, {"teal", &p.Teal}, {"magenta", &p.Magenta},
	}
}

func (t *Theme) keys() []colorKey {
	k := []colorKey{
		{"background", &t.Bg}, {"text", &t.Fg},
		{"heading1-fg", &t.H1Fg}, {"heading1-bg", &t.H1Bg}, {"heading2", &t.H2}, {"heading3", &t.H3},
		{"heading4", &t.H4}, {"heading5", &t.H5}, {"strong", &t.Strong}, {"code", &t.Code},
		{"code-bg", &t.CodeBg}, {"link", &t.Link}, {"image", &t.Image}, {"quote", &t.Quote},
		{"quote-bar", &t.QuoteBar}, {"rule", &t.Border}, {"secondary", &t.Muted}, {"bullet", &t.Bullet},
		{"list-number", &t.Number}, {"task-done", &t.TaskDone}, {"task-open", &t.TaskOpen},
		{"table-header", &t.TableHead}, {"syntax-keyword", &t.Keyword}, {"syntax-string", &t.String},
		{"syntax-number", &t.Num}, {"syntax-comment", &t.Comment}, {"syntax-function", &t.Func},
		{"syntax-type", &t.Type}, {"diff-added", &t.Added}, {"diff-removed", &t.Removed},
		{"alert-note", &t.Note}, {"alert-tip", &t.Tip}, {"alert-important", &t.Important},
		{"alert-warning", &t.Warning}, {"alert-caution", &t.Caution},
		{"math-variable", &t.MathVar}, {"math-number", &t.MathNum}, {"math-operator", &t.MathOp},
		{"math-symbol", &t.MathSymbol}, {"math-function", &t.MathFunc},
		{"diagram-border", &t.DiagBorder}, {"diagram-node", &t.DiagNode}, {"diagram-edge", &t.DiagEdge},
		{"diagram-arrow", &t.DiagArrow}, {"diagram-label", &t.DiagLabel}, {"diagram-title", &t.DiagTitle},
	}
	for i := range t.Series {
		k = append(k, colorKey{"series" + strconv.Itoa(i+1), &t.Series[i]})
	}
	return k
}

// builtinNames lists the bundled themes (all palettes are open source).
var builtinNames = []string{
	"tokyo-night", "tokyo-night-day", "catppuccin-mocha", "catppuccin-latte", "dracula", "nord",
	"gruvbox-dark", "gruvbox-light", "solarized-dark", "solarized-light", "one-dark", "github-dark", "github-light",
}

// BuiltinThemes returns the names of the bundled themes.
func BuiltinThemes() []string { return append([]string(nil), builtinNames...) }

func pal(dark bool, c ...uint32) (Palette, bool) {
	var p Palette
	for i, k := range p.keys() {
		*k.ptr = RGB(c[i])
	}
	return p, dark
}

// builtin returns the palette of a bundled theme ("dark"/"light" are aliases).
func builtin(name string) (Palette, bool, bool) {
	var p Palette
	var dark bool
	//                       bg        fg        bright    muted     comment   border    surface   on-accent red       orange    yellow    green     cyan      blue      purple    teal      magenta
	switch name {
	case "tokyo-night", "dark":
		p, dark = pal(true, 0x1a1b26, 0xc0caf5, 0xe9eefe, 0x737aa2, 0x5f6996, 0x3b4261, 0x292e42, 0x1a1b26, 0xf7768e, 0xff9e64, 0xe0af68, 0x9ece6a, 0x7dcfff, 0x7aa2f7, 0xbb9af7, 0x2ac3de, 0xff007c)
	case "tokyo-night-day", "light":
		p, dark = pal(false, 0xe1e2e7, 0x3760bf, 0x1a1b26, 0x6172b0, 0x848cb5, 0xa8aecb, 0xd0d5e3, 0xffffff, 0xf52a65, 0xb15c00, 0x8c6c3e, 0x587539, 0x007197, 0x2e7de9, 0x9854f1, 0x118c74, 0xd20065)
	case "catppuccin-mocha":
		p, dark = pal(true, 0x1e1e2e, 0xcdd6f4, 0xffffff, 0x9399b2, 0x6c7086, 0x45475a, 0x313244, 0x1e1e2e, 0xf38ba8, 0xfab387, 0xf9e2af, 0xa6e3a1, 0x89dceb, 0x89b4fa, 0xcba6f7, 0x94e2d5, 0xf5c2e7)
	case "catppuccin-latte":
		p, dark = pal(false, 0xeff1f5, 0x4c4f69, 0x11111b, 0x6c6f85, 0x8c8fa1, 0xbcc0cc, 0xccd0da, 0xeff1f5, 0xd20f39, 0xfe640b, 0xdf8e1d, 0x40a02b, 0x04a5e5, 0x1e66f5, 0x8839ef, 0x179299, 0xea76cb)
	case "dracula":
		p, dark = pal(true, 0x282a36, 0xf8f8f2, 0xffffff, 0x9aa5ce, 0x6272a4, 0x44475a, 0x343746, 0x282a36, 0xff5555, 0xffb86c, 0xf1fa8c, 0x50fa7b, 0x8be9fd, 0xbd93f9, 0xff79c6, 0x8be9fd, 0xff79c6)
	case "nord":
		p, dark = pal(true, 0x2e3440, 0xd8dee9, 0xeceff4, 0x8a93a6, 0x616e88, 0x434c5e, 0x3b4252, 0x2e3440, 0xbf616a, 0xd08770, 0xebcb8b, 0xa3be8c, 0x88c0d0, 0x81a1c1, 0xb48ead, 0x8fbcbb, 0xb48ead)
	case "gruvbox-dark":
		p, dark = pal(true, 0x282828, 0xebdbb2, 0xfbf1c7, 0xa89984, 0x928374, 0x504945, 0x3c3836, 0x282828, 0xfb4934, 0xfe8019, 0xfabd2f, 0xb8bb26, 0x8ec07c, 0x83a598, 0xd3869b, 0x8ec07c, 0xd3869b)
	case "gruvbox-light":
		p, dark = pal(false, 0xfbf1c7, 0x3c3836, 0x282828, 0x7c6f64, 0x928374, 0xd5c4a1, 0xebdbb2, 0xfbf1c7, 0x9d0006, 0xaf3a03, 0xb57614, 0x79740e, 0x427b58, 0x076678, 0x8f3f71, 0x427b58, 0x8f3f71)
	case "solarized-dark":
		p, dark = pal(true, 0x002b36, 0x839496, 0x93a1a1, 0x657b83, 0x586e75, 0x073642, 0x073642, 0xfdf6e3, 0xdc322f, 0xcb4b16, 0xb58900, 0x859900, 0x2aa198, 0x268bd2, 0x6c71c4, 0x2aa198, 0xd33682)
	case "solarized-light":
		p, dark = pal(false, 0xfdf6e3, 0x657b83, 0x586e75, 0x839496, 0x93a1a1, 0xd6ceb5, 0xeee8d5, 0xfdf6e3, 0xdc322f, 0xcb4b16, 0xb58900, 0x859900, 0x2aa198, 0x268bd2, 0x6c71c4, 0x2aa198, 0xd33682)
	case "one-dark":
		p, dark = pal(true, 0x282c34, 0xabb2bf, 0xe6e6e6, 0x7f848e, 0x5c6370, 0x3e4451, 0x2c313a, 0x282c34, 0xe06c75, 0xd19a66, 0xe5c07b, 0x98c379, 0x56b6c2, 0x61afef, 0xc678dd, 0x56b6c2, 0xc678dd)
	case "github-dark":
		p, dark = pal(true, 0x0d1117, 0xe6edf3, 0xffffff, 0x8d96a0, 0x8b949e, 0x30363d, 0x161b22, 0x0d1117, 0xff7b72, 0xffa657, 0xd29922, 0x7ee787, 0x79c0ff, 0x58a6ff, 0xd2a8ff, 0xa5d6ff, 0xf778ba)
	case "github-light":
		p, dark = pal(false, 0xffffff, 0x1f2328, 0x000000, 0x59636e, 0x6e7781, 0xd1d9e0, 0xf6f8fa, 0xffffff, 0xcf222e, 0xbc4c00, 0x9a6700, 0x116329, 0x0550ae, 0x0969da, 0x8250df, 0x1b7c83, 0xbf3989)
	default:
		return p, false, false
	}
	return p, dark, true
}

// Dark and Light are the default themes.
var (
	Dark  = mustBuiltin("tokyo-night")
	Light = mustBuiltin("tokyo-night-day")
)

func mustBuiltin(name string) Theme {
	p, dark, _ := builtin(name)
	return p.Theme(name, dark)
}

// ParseColor parses "#rrggbb", "rrggbb" or "#rgb".
func ParseColor(s string) (Color, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, fmt.Errorf("invalid color %q (want #rrggbb)", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid color %q (want #rrggbb)", s)
	}
	return RGB(uint32(v)), nil
}

// Hex formats c as #rrggbb.
func (c Color) Hex() string { return fmt.Sprintf("#%06x", uint32(c)&0xffffff) }

// ThemeDirs returns the directories searched for user themes.
func ThemeDirs(getenv func(string) string) []string {
	var dirs []string
	if x := getenv("XDG_CONFIG_HOME"); x != "" {
		dirs = append(dirs, filepath.Join(x, "kmd", "themes"))
	} else if h := getenv("HOME"); h != "" {
		dirs = append(dirs, filepath.Join(h, ".config", "kmd", "themes"))
	}
	return append(dirs, "/usr/share/kmd/themes")
}

// UserThemes lists *.theme files found in dirs (names without extension).
func UserThemes(dirs []string) []string {
	var names []string
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if n, ok := strings.CutSuffix(e.Name(), ".theme"); ok && !e.IsDir() {
				names = append(names, n)
			}
		}
	}
	return names
}

// ErrUnknownTheme is returned for a theme that is neither bundled nor on disk.
var ErrUnknownTheme = errors.New("unknown theme")

// LoadTheme resolves a theme name: a user theme file in dirs, a path to a
// .theme file, or a bundled theme.
func LoadTheme(name string, dirs []string) (*Theme, error) { return loadTheme(name, dirs, 0) }

func loadTheme(name string, dirs []string, depth int) (*Theme, error) {
	if depth > 8 {
		return nil, fmt.Errorf("theme %q: inheritance too deep", name)
	}
	path := ""
	if strings.HasSuffix(name, ".theme") || strings.ContainsRune(name, filepath.Separator) {
		path = name
	} else {
		for _, d := range dirs {
			if p := filepath.Join(d, name+".theme"); fileExists(p) {
				path = p
				break
			}
		}
	}
	if path == "" {
		p, dark, ok := builtin(name)
		if !ok {
			return nil, fmt.Errorf("%w %q (see kmd --list-themes)", ErrUnknownTheme, name)
		}
		t := p.Theme(name, dark)
		return &t, nil
	}
	// #nosec G304 G703 -- theme files are chosen by the user (config dir or explicit path).
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 64<<10 {
		return nil, fmt.Errorf("%s: theme file too large", path)
	}
	base := strings.TrimSuffix(filepath.Base(path), ".theme")
	return parseTheme(base, string(data), func(parent string) (*Theme, Palette, error) {
		if parent == base {
			return nil, Palette{}, fmt.Errorf("theme %q inherits from itself", base)
		}
		if p, dark, ok := builtin(parent); ok {
			t := p.Theme(parent, dark)
			return &t, p, nil
		}
		t, err := loadTheme(parent, dirs, depth+1)
		return t, Palette{}, err
	})
}

func fileExists(p string) bool {
	st, err := os.Stat(p) // #nosec G703 -- probing the user's own theme directories.
	return err == nil && st.Mode().IsRegular()
}

// parseTheme reads "key = value" lines: optional "inherit", "dark",
// palette keys (which re-derive every role) and role overrides.
func parseTheme(name, src string, resolve func(string) (*Theme, Palette, error)) (*Theme, error) {
	type kv struct {
		k, v string
		line int
	}
	var entries []kv
	parent, darkSetting := "tokyo-night", ""
	for i, line := range strings.Split(src, "\n") {
		line = stripComment(line)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("theme %s line %d: expected key = value", name, i+1)
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		switch k {
		case "inherit":
			parent = v
		case "dark":
			darkSetting = v
		case "name":
		default:
			entries = append(entries, kv{k, v, i + 1})
		}
	}
	base, pal, err := resolve(parent)
	if err != nil {
		return nil, err
	}
	t := *base
	palChanged := false
	var p Palette
	if pal != (Palette{}) {
		p = pal
	}
	pk := p.keys()
	for _, e := range entries {
		for _, k := range pk {
			if k.key == e.k {
				c, err := ParseColor(e.v)
				if err != nil {
					return nil, fmt.Errorf("theme %s line %d: %v", name, e.line, err)
				}
				*k.ptr = c
				palChanged = true
			}
		}
	}
	if palChanged {
		if pal == (Palette{}) {
			return nil, fmt.Errorf("theme %s: palette keys need a bundled theme to inherit from", name)
		}
		t = p.Theme(name, base.Dark)
	}
	t.Name = name
	switch darkSetting {
	case "":
	case "true", "yes":
		t.Dark = true
	case "false", "no":
		t.Dark = false
	default:
		return nil, fmt.Errorf("theme %s: dark must be true or false", name)
	}
	tk := t.keys()
outer:
	for _, e := range entries {
		for _, k := range pk {
			if k.key == e.k {
				continue outer
			}
		}
		for _, k := range tk {
			if k.key == e.k {
				c, err := ParseColor(e.v)
				if err != nil {
					return nil, fmt.Errorf("theme %s line %d: %v", name, e.line, err)
				}
				*k.ptr = c
				continue outer
			}
		}
		return nil, fmt.Errorf("theme %s line %d: unknown key %q", name, e.line, e.k)
	}
	return &t, nil
}

// ThemeTemplate returns a commented .theme file for a bundled theme, ready
// to be copied into ~/.config/kmd/themes and customized.
func ThemeTemplate(name string) (string, error) {
	p, dark, ok := builtin(name)
	if !ok {
		return "", fmt.Errorf("%w %q", ErrUnknownTheme, name)
	}
	t := p.Theme(name, dark)
	var b strings.Builder
	fmt.Fprintf(&b, "# kmd theme — save as ~/.config/kmd/themes/<name>.theme and use: kmd -t <name>\n")
	fmt.Fprintf(&b, "inherit = %s\ndark = %v\n\n# Palette: every role below is derived from these colors.\n", name, dark)
	for _, k := range p.keys() {
		fmt.Fprintf(&b, "%-10s = %s\n", k.key, k.ptr.Hex())
	}
	b.WriteString("\n# Roles: uncomment to override a single element.\n")
	for _, k := range t.keys() {
		fmt.Fprintf(&b, "# %-16s = %s\n", k.key, k.ptr.Hex())
	}
	return b.String(), nil
}

// StripComment removes a trailing "# comment": a '#' at the start of the
// line or preceded by whitespace and followed by whitespace (so "#rrggbb"
// color values are kept).
func StripComment(line string) string {
	for j := 0; j < len(line); j++ {
		if line[j] != '#' {
			continue
		}
		if j == 0 && (len(line) == 1 || line[1] == ' ' || line[1] == '\t' || line[1] == '#') {
			return ""
		}
		if j > 0 && (line[j-1] == ' ' || line[j-1] == '\t') && (j+1 == len(line) || line[j+1] == ' ' || line[j+1] == '\t') {
			return line[:j]
		}
	}
	return line
}

func stripComment(line string) string { return StripComment(line) }
