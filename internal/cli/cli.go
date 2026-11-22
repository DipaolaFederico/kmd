// Package cli implements the kmd command line interface.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
	"github.com/DipaolaFederico/kmd/internal/view"
)

// Version is overridden at build time via -ldflags.
var Version = "dev"

// version is Version, or the module version Go recorded in the binary when
// it was built with `go install github.com/DipaolaFederico/kmd@vX.Y.Z`
// (no -ldflags there).
func version() string { return versionFrom(Version, debug.ReadBuildInfo) }

func versionFrom(v string, read func() (*debug.BuildInfo, bool)) string {
	if v != "dev" {
		return v
	}
	if bi, ok := read(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return v
}

// maxInput caps how much is read from a single source (16 MiB): the tree
// and rendered lines take tens of bytes per input byte.
const maxInput = 16 << 20

// maxWidth bounds --width so a typo cannot allocate gigabytes of padding.
const maxWidth = 1000

// defaultMaxWidth keeps prose readable on very wide terminals.
const defaultMaxWidth = 110

// Env is everything Run needs from the outside world, so it can be tested.
type Env struct {
	Args           []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	StdinTTY       bool
	StdoutTTY      bool
	Cols, Rows     int
	CellW, CellH   int // terminal cell size in pixels (0 = unknown)
	// Pager displays content through a pager; nil uses the system pager.
	Pager func(content, command string) error
	// View runs the built-in interactive viewer; nil uses the terminal.
	View func(view.Options) error
	// Interrupt stops --follow when closed (nil: runs until killed).
	Interrupt <-chan struct{}
	// Poll is the --follow polling interval (0 = 250ms).
	Poll time.Duration
}

type options struct {
	width        int
	theme        string
	color        string
	images       string
	hyperlinks   string
	noPager      bool
	noMath       bool
	noDiagrams   bool
	commonmark   bool
	toc          bool
	html         bool
	unsafeHTML   bool
	follow       bool
	view         bool
	listThemes   bool
	template     string
	showConfig   bool
	completion   string
	version      bool
	files        []string
	explicitFlag map[string]bool
}

const usage = `kmd — beautiful Markdown previews in your terminal

Usage:
  kmd [options] [file ...]
  cat README.md | kmd [options]

Display:
  -w, --width N            render width (default: terminal width, max %d)
  -t, --theme NAME         color theme or path to a .theme file (default: auto)
  -c, --color WHEN         auto | always | never
  -i, --images MODE        auto | blocks | kitty | iterm2 | sixel | none
      --hyperlinks WHEN    clickable OSC 8 links: auto | always | never
      --no-math            show LaTeX source instead of rendered formulas
      --no-diagrams        show mermaid/graphviz source instead of diagrams
      --commonmark         strict CommonMark: disable GitHub extensions
  -P, --no-pager           never page long output (no viewer)
      --view               open the viewer even when the document fits

Output modes:
      --toc                print only the table of contents
      --html               export a standalone HTML page (raw HTML and
                           javascript:/file: links are removed)
      --unsafe-html        with --html: keep raw HTML and all URLs (trusted input only)
  -f, --follow             live preview: re-render when the file changes

Themes and configuration:
      --list-themes        list bundled and user themes
      --theme-template T   print theme T as an editable .theme file
      --config             show config locations and effective settings

Other:
      --completion SHELL   print shell completion (bash | zsh | fish)
  -v, --version            print version
  -h, --help               show this help

Files:
  ~/.config/kmd/config            settings (kmd --config shows a template)
  ~/.config/kmd/themes/*.theme    custom themes

Viewer (long documents):
  Tab/Shift-Tab select a link (or click it) · Enter open it · Backspace back
  H back to the first document
  / search · e edit · ? all keys · q quit

Environment:
  KMD_THEME, KMD_PAGER (builtin | command | -), KMD_CONFIG, NO_COLOR,
  COLORFGBG, XDG_CONFIG_HOME
`

func parseArgs(args []string, stderr io.Writer) (options, error) {
	o := options{explicitFlag: map[string]bool{}}
	fs := flag.NewFlagSet("kmd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	str := func(p *string, def string, names ...string) {
		for _, n := range names {
			fs.StringVar(p, n, def, "")
		}
	}
	boolean := func(p *bool, names ...string) {
		for _, n := range names {
			fs.BoolVar(p, n, false, "")
		}
	}
	for _, n := range []string{"w", "width"} {
		fs.IntVar(&o.width, n, 0, "")
	}
	str(&o.theme, "", "t", "theme")
	str(&o.color, "", "c", "color")
	str(&o.images, "", "i", "images")
	str(&o.hyperlinks, "", "hyperlinks")
	str(&o.template, "", "theme-template")
	str(&o.completion, "", "completion")
	boolean(&o.noPager, "P", "no-pager")
	boolean(&o.noMath, "no-math")
	boolean(&o.noDiagrams, "no-diagrams")
	boolean(&o.commonmark, "commonmark")
	boolean(&o.toc, "toc")
	boolean(&o.html, "html")
	boolean(&o.unsafeHTML, "unsafe-html")
	boolean(&o.follow, "f", "follow")
	boolean(&o.view, "view")
	boolean(&o.listThemes, "list-themes")
	boolean(&o.showConfig, "config")
	boolean(&o.version, "v", "version")

	var rest []string
	for i, a := range args {
		if a == "--" {
			rest, args = args[i+1:], args[:i]
			break
		}
	}
	for { // allow flags after file names
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Fprintf(stderr, usage, defaultMaxWidth)
			}
			return o, err
		}
		if fs.NArg() == 0 {
			break
		}
		o.files = append(o.files, fs.Arg(0))
		args = fs.Args()[1:]
	}
	fs.Visit(func(f *flag.Flag) { o.explicitFlag[f.Name] = true })
	o.files = append(o.files, rest...)
	for _, c := range []struct{ name, v string }{{"color", o.color}, {"hyperlinks", o.hyperlinks}} {
		if c.v != "" {
			if err := checkWhen("--"+c.name, c.v); err != nil {
				return o, err
			}
		}
	}
	if o.width < 0 || o.width > maxWidth {
		return o, fmt.Errorf("--width must be between 0 (terminal width) and %d", maxWidth)
	}
	if o.follow && len(o.files) != 1 {
		return o, errors.New("--follow needs exactly one file")
	}
	return o, nil
}

// settings is the resolved configuration for one run.
type settings struct {
	opts       render.Options
	pager      string // "builtin" = the interactive viewer
	mdOpt      md.Options
	themeNm    string
	fixedWidth int  // --width / width (0 = follow the terminal)
	maxWidth   int  // cap for the automatic width
	mouse      bool // the viewer reports clicks and the wheel
	alwaysView bool // --view / view = always: viewer even for short documents
}

// builtinPager names the interactive viewer in KMD_PAGER / pager.
const builtinPager = "builtin"

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// isLightBackground interprets $COLORFGBG ("fg;bg"): bg 7 or 9–15 is light.
func isLightBackground(getenv func(string) string) bool {
	fgbg := getenv("COLORFGBG")
	if fgbg == "" {
		return false
	}
	n, err := strconv.Atoi(fgbg[strings.LastIndexByte(fgbg, ';')+1:])
	return err == nil && n != 8 && n >= 7 && n <= 15
}

func resolve(o options, cfg config, env Env) (settings, error) {
	get := env.Getenv
	var s settings

	color := firstNonEmpty(o.color, cfg.Color, "auto")
	s.opts.Color = color == "always" ||
		(color == "auto" && env.StdoutTTY && get("NO_COLOR") == "" && get("TERM") != "dumb")
	ct := get("COLORTERM")
	s.opts.TrueColor = ct == "truecolor" || ct == "24bit"

	links := firstNonEmpty(o.hyperlinks, cfg.Hyperlinks, "auto")
	s.opts.Hyperlinks = links == "always" || (links == "auto" && s.opts.Color && env.StdoutTTY)

	s.fixedWidth = o.width
	if s.fixedWidth == 0 {
		s.fixedWidth = min(cfg.Width, maxWidth)
	}
	s.maxWidth = defaultMaxWidth
	if cfg.MaxWidth > 0 {
		s.maxWidth = cfg.MaxWidth
	}
	s.opts.Width = s.width(env.Cols)

	name := firstNonEmpty(o.theme, get("KMD_THEME"), cfg.Theme, "auto")
	if name == "auto" {
		name = firstNonEmpty(cfg.DarkTheme, "tokyo-night")
		if isLightBackground(get) {
			name = firstNonEmpty(cfg.LightTheme, "tokyo-night-day")
		}
	}
	th, err := render.LoadTheme(name, render.ThemeDirs(get))
	if err != nil {
		return s, err
	}
	s.opts.Theme, s.themeNm = th, name

	mode := firstNonEmpty(o.images, cfg.Images, "auto")
	if mode == "auto" {
		s.opts.Images = termimg.None
		if s.opts.Color && env.StdoutTTY {
			s.opts.Images = termimg.Detect(get, s.opts.TrueColor)
		}
	} else {
		p, err := termimg.ParseProtocol(mode)
		if err != nil {
			return s, err
		}
		s.opts.Images = p
	}
	s.opts.Cell = termimg.Cell{W: env.CellW, H: env.CellH}
	s.opts.ImageRows = cfg.ImageRows

	s.opts.Math = !o.noMath && (cfg.Math == nil || *cfg.Math)
	s.opts.Diagrams = !o.noDiagrams && (cfg.Diagrams == nil || *cfg.Diagrams)
	s.mdOpt = md.GFM
	if o.commonmark {
		s.mdOpt = md.Options{}
	}
	if !s.opts.Math {
		s.mdOpt.Math = false
	}
	s.pager = firstNonEmpty(get("KMD_PAGER"), cfg.Pager, builtinPager)
	if s.pager == builtinPager && !view.Supported {
		s.pager = "less -R"
	}
	if o.noPager || s.pager == "-" {
		s.pager = ""
	}
	s.mouse = cfg.Mouse == nil || *cfg.Mouse
	s.alwaysView = o.view || cfg.View == "always" && !o.noPager
	return s, nil
}

// width is the render width for a terminal cols wide (0 = unknown).
func (s settings) width(cols int) int {
	switch {
	case s.fixedWidth > 0:
		return s.fixedWidth
	case cols > 0:
		return min(cols, s.maxWidth)
	}
	return 80
}

func readSource(name string, stdin io.Reader) (string, error) {
	var r io.Reader = stdin
	if name != "-" {
		// #nosec G304 -- reading the file the user asked for is the point of the tool.
		f, err := os.Open(filepath.Clean(name))
		if err != nil {
			return "", err
		}
		defer func() { _ = f.Close() }()
		st, err := f.Stat()
		if err != nil {
			return "", err
		}
		if st.IsDir() {
			return "", fmt.Errorf("%s: is a directory", name)
		}
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, maxInput+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxInput {
		return "", fmt.Errorf("%s: larger than %d MiB", name, maxInput>>20)
	}
	return string(data), nil
}

// Run executes kmd and returns the process exit code.
func Run(env Env) int {
	o, err := parseArgs(env.Args, env.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "kmd: %v\nTry 'kmd --help'.\n", err)
		return 2
	}
	switch {
	case o.version:
		fmt.Fprintf(env.Stdout, "kmd %s\n", version())
		return 0
	case o.completion != "":
		return completion(env, o.completion)
	case o.template != "":
		t, err := render.ThemeTemplate(o.template)
		if err != nil {
			fmt.Fprintf(env.Stderr, "kmd: %v\n", err)
			return 2
		}
		_, _ = io.WriteString(env.Stdout, t)
		return 0
	}

	cfg, err := loadConfig(configPath(env.Getenv))
	if err != nil {
		fmt.Fprintf(env.Stderr, "kmd: %v\n", err)
		return 2
	}
	s, err := resolve(o, cfg, env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "kmd: %v\nTry 'kmd --help'.\n", err)
		return 2
	}
	switch {
	case o.listThemes:
		return listThemes(env, s)
	case o.showConfig:
		return showConfig(env, cfg, s)
	}

	if len(o.files) == 0 {
		if env.StdinTTY {
			fmt.Fprintf(env.Stderr, usage, defaultMaxWidth)
			return 2
		}
		o.files = []string{"-"}
	}
	viewable := !o.html && !o.toc && env.StdoutTTY && (s.pager == builtinPager || s.alwaysView)
	if o.follow {
		// live preview in the viewer: links stay navigable, position is kept
		if viewable && o.files[0] != "-" && runViewer(env, o, s, &stdinCache{}, true, nil, 0) == nil {
			return 0
		}
		return follow(env, o, s)
	}

	in := &stdinCache{}
	var res output
	vs := viewerSettings(s, env.Cols)
	if viewable {
		// render once, the way the viewer shows it: a long document goes
		// to the viewer without being rendered again
		res = renderFiles(env, o, vs, in)
	} else {
		res = renderFiles(env, o, s, in)
	}
	long := !o.html && !o.toc && env.StdoutTTY && s.pager != "" && env.Rows > 0 &&
		strings.Count(res.text, "\n") > env.Rows
	if viewable && (s.alwaysView || long && s.pager == builtinPager) {
		if err := runViewer(env, o, s, in, false, pageOf(res), env.Cols); err == nil {
			return res.code
		}
		if s.pager == builtinPager {
			s.pager = "less -R" // no usable terminal: fall back to less
		}
	}
	if viewable && (vs.opts.Images != s.opts.Images || vs.opts.Width != s.opts.Width) {
		// printed (or paged) after all: render with terminal graphics and the
		// normal width; errors were already reported by the first pass
		quiet := env
		quiet.Stderr = io.Discard
		code := res.code
		res = renderFiles(quiet, o, s, in)
		res.code = code
	}
	content, code := res.text, res.code
	if long && !res.graphics {
		pager := env.Pager
		if pager == nil {
			pager = systemPager(env.Stdout, env.Stderr)
		}
		if err := pager(content, s.pager); err == nil {
			return code
		}
	}
	if _, err := io.WriteString(env.Stdout, content); err != nil {
		return 1
	}
	return code
}

// stdinCache keeps stdin, which can only be read once ("kmd - -", and the
// viewer re-rendering on resize or when going back).
type stdinCache struct {
	src  string
	read bool
}

// output is the result of rendering the documents of a run.
type output struct {
	text     string
	graphics bool
	code     int
	links    []render.Link  // with s.opts.Navigate
	anchors  map[string]int // with s.opts.Navigate
}

func renderFiles(env Env, o options, s settings, in *stdinCache) output {
	var out strings.Builder
	res := output{}
	if s.opts.Navigate {
		res.anchors = map[string]int{}
	}
	var htmlParts []string
	title := ""
	lines := 0 // lines written so far (offset of the next document's links)
	for _, name := range o.files {
		var src string
		var err error
		if name == "-" && in.read {
			src = in.src
		} else {
			src, err = readSource(name, env.Stdin)
			if name == "-" {
				in.src, in.read = src, true
			}
		}
		if err != nil {
			fmt.Fprintf(env.Stderr, "kmd: %s\n", render.Clean(err.Error()))
			res.code = 1
			continue
		}
		doc := md.Parse(src, s.mdOpt)
		opts := s.opts
		if name != "-" {
			opts.BaseDir = filepath.Dir(name)
		}
		switch {
		case o.html:
			htmlParts = append(htmlParts, md.HTML(doc, md.HTMLOptions{TagFilter: true, Unsafe: o.unsafeHTML}))
			if title == "" {
				title = docTitle(doc, name)
			}
		case o.toc:
			out.WriteString(render.TOC(doc, opts))
		default:
			if len(o.files) > 1 {
				t := render.RenderString("# "+fileTitle(name), opts)
				out.WriteString(t)
				lines += strings.Count(t, "\n")
			}
			r := render.Render(doc, opts)
			res.graphics = res.graphics || r.Graphics
			if s.opts.Navigate {
				res.addNav(r, lines)
				lines += strings.Count(r.Text, "\n")
			}
			out.WriteString(r.Text)
		}
	}
	if o.html {
		res.text = htmlPage(title, strings.Join(htmlParts, "<hr />\n"), s.opts.Theme)
		return res
	}
	res.text = out.String()
	return res
}

// addNav appends a document's links and anchors, shifted to line offset.
func (res *output) addNav(r render.Result, offset int) {
	for _, l := range r.Links {
		for i := range l.Spans {
			l.Spans[i].Line += offset
		}
		res.links = append(res.links, l)
	}
	for k, v := range r.Anchors {
		if _, dup := res.anchors[k]; !dup {
			res.anchors[k] = v + offset
		}
	}
}

func docTitle(doc *md.Node, name string) string {
	for c := doc.FirstChild; c != nil; c = c.Next {
		if c.Type == md.Heading {
			return md.PlainText(c)
		}
	}
	if name == "-" {
		return "kmd"
	}
	return filepath.Base(name)
}

func fileTitle(name string) string {
	if name == "-" {
		return "stdin"
	}
	return strings.NewReplacer("*", "\\*", "_", "\\_", "`", "\\`", "[", "\\[").Replace(filepath.Base(name))
}

func systemPager(stdout, stderr io.Writer) func(string, string) error {
	return func(content, command string) error {
		args := strings.Fields(command)
		if len(args) == 0 {
			return errors.New("no pager")
		}
		path, err := exec.LookPath(args[0])
		if err != nil {
			return err
		}
		// #nosec G204 -- the pager command is chosen by the user (KMD_PAGER / config).
		cmd := exec.Command(path, args[1:]...)
		cmd.Stdin = strings.NewReader(content)
		cmd.Stdout, cmd.Stderr = stdout, stderr
		// Ctrl-C belongs to the pager (less uses it to cancel searches):
		// kmd must not die and hand the prompt back while less owns the tty.
		signal.Ignore(os.Interrupt)
		defer signal.Reset(os.Interrupt)
		if err := cmd.Start(); err != nil {
			return err
		}
		_ = cmd.Wait() // a pager quitting with an error already showed the text
		return nil
	}
}

// follow re-renders a file whenever its size or modification time changes.
func follow(env Env, o options, s settings) int {
	name := o.files[0]
	if name == "-" {
		fmt.Fprintln(env.Stderr, "kmd: --follow needs a file, not stdin")
		return 2
	}
	poll := env.Poll
	if poll == 0 {
		poll = 250 * time.Millisecond
	}
	var lastMod time.Time
	lastSize := int64(-1)
	if _, err := os.Stat(name); err != nil {
		fmt.Fprintf(env.Stderr, "kmd: %s\n", render.Clean(err.Error()))
		return 1
	}
	for {
		// a failed stat is usually an editor replacing the file: keep polling
		if st, err := os.Stat(name); err == nil && (!st.ModTime().Equal(lastMod) || st.Size() != lastSize) {
			lastMod, lastSize = st.ModTime(), st.Size()
			content := renderFiles(env, o, s, &stdinCache{}).text
			if env.StdoutTTY {
				content = "\x1b[H\x1b[2J\x1b[3J" + content
			}
			if _, err := io.WriteString(env.Stdout, content); err != nil {
				return 1
			}
		}
		select {
		case <-env.Interrupt:
			return 0
		case <-time.After(poll):
		}
	}
}

// runViewer shows the documents in the interactive viewer. Pages are
// rendered for the viewer's terminal size, with graphics images drawn as
// Unicode blocks (terminal graphics cannot scroll with the text).
// viewerSettings are the render settings of the viewer on a terminal cols
// wide: navigation data on, terminal graphics drawn as Unicode blocks, and
// never wider than the terminal.
func viewerSettings(s settings, cols int) settings {
	s.opts.Navigate = true
	switch s.opts.Images {
	case termimg.Kitty, termimg.ITerm2, termimg.Sixel:
		s.opts.Images = termimg.Blocks
	}
	if cols > 0 {
		s.opts.Width = min(s.width(cols), max(cols, 20))
	}
	return s
}

// pageOf turns a render into a viewer page.
func pageOf(res output) *view.Page {
	return &view.Page{
		Lines:   strings.Split(strings.TrimSuffix(res.text, "\n"), "\n"),
		Links:   res.links,
		Anchors: res.anchors,
	}
}

// runViewer opens the viewer; first (rendered for a terminal firstCols
// wide) is its first page, or nil to render it.
func runViewer(env Env, o options, s settings, in *stdinCache, watch bool, first *view.Page, firstCols int) error {
	load := func(path string, cols int) (*view.Page, error) {
		s := viewerSettings(s, max(cols, 1))
		files := o.files
		if path != "" {
			files = []string{path}
		}
		var errs strings.Builder
		quiet := env
		quiet.Stderr = &errs // errors go to the status bar, not the screen
		res := renderFiles(quiet, options{files: files}, s, in)
		if res.code != 0 && len(files) == 1 {
			return nil, errors.New(strings.TrimPrefix(strings.TrimSpace(errs.String()), "kmd: "))
		}
		return pageOf(res), nil
	}
	start := ""
	if len(o.files) == 1 && o.files[0] != "-" {
		start = o.files[0]
	}
	sel, match, bar := render.ViewerStyles(s.opts.Theme, s.opts.Color, s.opts.TrueColor)
	opt := view.Options{Load: load, Start: start, Mouse: s.mouse, Watch: watch, Poll: env.Poll,
		Select: sel, Match: match, Bar: bar, First: first, FirstCols: firstCols}
	if env.View != nil {
		return env.View(opt)
	}
	return view.Run(opt)
}
