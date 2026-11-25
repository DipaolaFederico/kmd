// Package cli implements the kmd command line interface.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/DipaolaFederico/kmd/internal/bridge"
	"github.com/DipaolaFederico/kmd/internal/md"
	"github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
	"github.com/DipaolaFederico/kmd/internal/view"
	api "github.com/DipaolaFederico/kmd/render" // the public package: kmd is its first user
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

// maxInput caps how much is read from a single source.
const maxInput = md.MaxInput

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
	// LightBackground asks the terminal whether its background is light,
	// giving up after silent without an answer (ok false); nil never asks.
	LightBackground func(silent time.Duration) (light, ok bool)
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
	slides       bool
	notes        bool
	listThemes   bool
	template     string
	section      string
	base         *url.URL // --base
	showConfig   bool
	completion   string
	version      bool
	files        []string
	explicitFlag map[string]bool
}

const usage = `kmd — beautiful Markdown previews in your terminal

Usage:
  kmd [options] [file|folder ...]
  cat README.md | kmd [options]

A folder shows its README.md (or index.md), or else a list of every
Markdown file under it, filtered as you type. In the viewer, o opens that
list for the folder kmd was started on (the document's folder, or the
current one) and S searches the text of all its documents.

Display:
  -w, --width N            render width (default: terminal width, max %d)
  -t, --theme NAME         color theme or path to a .theme file (default: auto,
                           light or dark like the terminal; kmd --config says why)
  -c, --color WHEN         auto | always | never
  -i, --images MODE        auto | blocks | kitty | iterm2 | sixel | none
      --hyperlinks WHEN    clickable OSC 8 links: auto | always | never
      --no-math            show LaTeX source instead of rendered formulas
      --no-diagrams        show mermaid/graphviz source instead of diagrams
      --commonmark         strict CommonMark: disable GitHub extensions,
                           emoji shortcodes and [[wiki links]]
      --base URL           resolve relative links of standard input against
                           URL (http/https; nothing is downloaded), e.g.
                           curl -fsSL URL | kmd --base URL
  -P, --no-pager           never page long output (no viewer)
      --view               open the viewer even when the document fits

Output modes:
      --toc                print only the table of contents
  -s, --section NAME       only the section under heading NAME (its #anchor or
                           part of its text), e.g. kmd -s install README.md
      --html               export a standalone HTML page (raw HTML and
                           javascript:/file: links are removed)
      --unsafe-html        with --html: keep raw HTML and all URLs (trusted input only)
  -f, --follow             live preview: re-render when the file changes
      --slides             present the document as slides (split at --- or
                           at level 1-2 headings, revealed in steps at
                           <!-- pause -->); with -f, live; with --html,
                           a page to print as a PDF deck
      --notes              in another terminal: the speaker notes
                           (<!-- notes: ... -->) of the slide on screen

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
  H back to the first document · t contents · [ ] previous/next section
  / search · o find a document · S search all documents · c copy a code
  block · e edit · ? all keys · q quit

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
	str(&o.section, "", "s", "section")
	var baseURL string // validated into o.base below
	str(&baseURL, "", "base")
	boolean(&o.noPager, "P", "no-pager")
	boolean(&o.noMath, "no-math")
	boolean(&o.noDiagrams, "no-diagrams")
	boolean(&o.commonmark, "commonmark")
	boolean(&o.toc, "toc")
	boolean(&o.html, "html")
	boolean(&o.unsafeHTML, "unsafe-html")
	boolean(&o.follow, "f", "follow")
	boolean(&o.view, "view")
	boolean(&o.slides, "slides")
	boolean(&o.notes, "notes")
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
	var err error
	if o.base, err = parseBase(baseURL, o.explicitFlag["base"], o.files); err != nil {
		return o, err
	}
	return o, o.checkModes()
}

// checkModes rejects modes used with files or flags they do not support.
func (o options) checkModes() error {
	switch {
	case o.follow && len(o.files) != 1:
		return errors.New("--follow needs exactly one file")
	case o.slides && (len(o.files) > 1 || o.toc):
		return errors.New("--slides shows one document, without --toc")
	case o.slides && o.html && o.follow:
		return errors.New("--slides --html writes the page once: it cannot follow the file (-f)")
	case o.notes && (len(o.files) != 1 || o.files[0] == "-" || o.slides || o.html || o.toc):
		return errors.New("--notes needs one file and cannot be combined with --slides, --html or --toc")
	}
	return nil
}

// parseBase validates --base (nil when not given): an absolute http(s) URL
// without credentials that is safe to emit as a hyperlink target, for a run
// that reads standard input. The value is not echoed in errors: it may hold
// control characters.
func parseBase(s string, given bool, files []string) (*url.URL, error) {
	switch {
	case !given:
		return nil, nil
	case s == "": // --base "$URL" with $URL unset
		return nil, errors.New("--base needs a URL")
	case !render.SafeURL(s):
		return nil, errors.New("--base is too long or contains control or invisible characters")
	}
	u, err := url.Parse(s)
	// the host is checked decoded too: %E2%80%AE is a bidi override
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || !render.SafeURL(u.Hostname()) || u.User != nil {
		return nil, errors.New("--base must be an http or https URL without a user name, such as https://example.com/docs/README.md")
	}
	if len(files) > 0 && !slices.Contains(files, "-") {
		return nil, errors.New("--base applies to standard input: curl -fsSL URL | kmd --base URL")
	}
	return u, nil
}

// settings is the resolved configuration for one run.
type settings struct {
	opts       render.Options
	pager      string // "builtin" = the interactive viewer
	mdOpt      md.Options
	themeNm    string
	themeSrc   string // what chose the theme with theme = auto ("" otherwise)
	fixedWidth int    // --width / width (0 = follow the terminal)
	maxWidth   int    // cap for the automatic width
	mouse      bool   // the viewer reports clicks and the wheel
	alwaysView bool   // --view / view = always: viewer even for short documents
	ai         aiSettings
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

// parseFGBG interprets $COLORFGBG ("fg;bg"): bg 7 or 9–15 is light. ok is
// false when bg is not a color number (unset, "default").
func parseFGBG(fgbg string) (light, ok bool) {
	n, err := strconv.Atoi(fgbg[strings.LastIndexByte(fgbg, ';')+1:])
	return n != 8 && n >= 7 && n <= 15, err == nil
}

// Sources of the light/dark choice of theme = auto, for kmd --config.
const (
	srcFGBG     = "COLORFGBG"
	srcOSC11    = "terminal background, OSC 11"
	srcNoAnswer = "no answer from the terminal"
	srcNotAsked = "the terminal was not asked"
)

// How long kmd waits for a terminal that does not answer the background
// query. A local terminal answers within a few milliseconds; over SSH the
// answer takes a round trip, and one arriving after kmd gave up would be
// read by the shell as typed keys. (term caps any wait at one second.)
const (
	silentLocal = 100 * time.Millisecond
	silentSSH   = time.Second
)

// lightBackground decides between the light and dark default themes and
// names the source that decided. $COLORFGBG is free to read, so it wins when
// set; otherwise the terminal is asked when ask allows it, else dark.
func lightBackground(env Env, ask bool) (light bool, source string) {
	get := env.Getenv
	if light, ok := parseFGBG(get("COLORFGBG")); ok {
		return light, srcFGBG
	}
	if !ask || env.LightBackground == nil {
		return false, srcNotAsked
	}
	silent := silentLocal
	if get("SSH_CONNECTION") != "" || get("SSH_TTY") != "" {
		silent = silentSSH
	}
	if light, ok := env.LightBackground(silent); ok {
		return light, srcOSC11
	}
	return false, srcNoAnswer
}

// drawsOnTerminal reports whether this run draws in color on the terminal
// it was started from, interactively, and so may query it: never into a
// pipe or a file, without colors, or just to print the usage (no document
// on an interactive standard input). Commands that use the theme without
// a document (--config, --list-themes) are listed here.
func drawsOnTerminal(o options, env Env, color bool) bool {
	return env.StdinTTY && env.StdoutTTY && color && !o.html && env.Getenv("TERM") != "dumb" &&
		(len(o.files) > 0 || o.showConfig || o.listThemes)
}

// autoTheme picks the light or dark theme of theme = auto and names the
// source that decided (see lightBackground); color reports colored output.
func autoTheme(o options, cfg config, env Env, color bool) (name, source string) {
	light, source := lightBackground(env, drawsOnTerminal(o, env, color))
	if light {
		return firstNonEmpty(cfg.LightTheme, "tokyo-night-day"), source
	}
	return firstNonEmpty(cfg.DarkTheme, "tokyo-night"), source
}

// resolve turns flags, configuration and environment into settings. The
// render options are built as public render.Options and converted by the
// public package, exactly as for a library user.
func resolve(o options, cfg config, env Env) (settings, error) {
	get := env.Getenv
	var s settings
	var pub api.Options

	color := firstNonEmpty(o.color, cfg.Color, "auto")
	pub.Color = color == "always" ||
		(color == "auto" && env.StdoutTTY && get("NO_COLOR") == "" && get("TERM") != "dumb")
	ct := get("COLORTERM")
	pub.TrueColor = ct == "truecolor" || ct == "24bit"

	links := firstNonEmpty(o.hyperlinks, cfg.Hyperlinks, "auto")
	pub.Hyperlinks = links == "always" || (links == "auto" && pub.Color && env.StdoutTTY)

	s.fixedWidth = o.width
	if s.fixedWidth == 0 {
		s.fixedWidth = min(cfg.Width, maxWidth)
	}
	s.maxWidth = defaultMaxWidth
	if cfg.MaxWidth > 0 {
		s.maxWidth = cfg.MaxWidth
	}
	pub.Width = min(s.width(env.Cols), api.MaxWidth)

	name := firstNonEmpty(o.theme, get("KMD_THEME"), cfg.Theme, "auto")
	if name == "auto" {
		name, s.themeSrc = autoTheme(o, cfg, env, pub.Color)
	}
	th, err := api.LoadTheme(name, render.ThemeDirs(get)...)
	if errors.Is(err, api.ErrUnknownTheme) {
		err = fmt.Errorf("%w (see kmd --list-themes)", err)
	}
	if err != nil {
		return s, err
	}
	pub.Theme, s.themeNm = th, name

	mode := firstNonEmpty(o.images, cfg.Images, "auto")
	if mode == "auto" {
		mode = "none"
		if pub.Color && env.StdoutTTY {
			mode = termimg.Detect(get, pub.TrueColor).String()
		}
	}
	if pub.Images, err = api.ParseImageMode(mode); err != nil {
		return s, err
	}
	pub.CellWidth, pub.CellHeight = min(max(env.CellW, 0), 512), min(max(env.CellH, 0), 512)
	pub.ImageRows = cfg.ImageRows

	pub.NoMath = o.noMath || (cfg.Math != nil && !*cfg.Math)
	pub.NoDiagrams = o.noDiagrams || (cfg.Diagrams != nil && !*cfg.Diagrams)
	pub.CommonMark = o.commonmark

	c, err := bridge.Resolve(pub)
	if err != nil {
		return s, err
	}
	s.opts, s.mdOpt = c.Render, c.Parse
	s.opts.BaseURL = o.base
	// The user chose to open the document: its images may live anywhere
	// (../assets/x.png, absolute paths), unlike in the library's default.
	s.opts.AnyImagePath = true
	s.pager = firstNonEmpty(get("KMD_PAGER"), cfg.Pager, builtinPager)
	if s.pager == builtinPager && !view.Supported {
		s.pager = "less -R"
	}
	if o.noPager || s.pager == "-" {
		s.pager = ""
	}
	s.mouse = cfg.Mouse == nil || *cfg.Mouse
	s.alwaysView = o.view || cfg.View == "always" && !o.noPager
	s.ai = resolveAI(cfg, env)
	return s, nil
}

// docOptions returns the render options for document name, whose relative
// paths resolve against dir; --base applies only to standard input.
func (s settings) docOptions(name, dir string) render.Options {
	opts := s.opts
	opts.BaseDir = dir
	switch {
	case name != "-":
		opts.BaseURL = nil
	case opts.BaseURL != nil:
		// the document's images are on the web, next to it: draw only
		// data: URIs rather than read files from the current folder
		opts.AnyImagePath = false
	}
	return opts
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

// readSource reads a document; a directory reads as its listing (dir true).
func readSource(name string, stdin io.Reader) (src string, dir bool, err error) {
	var r io.Reader = stdin
	if name != "-" {
		// #nosec G304 -- reading the file the user asked for is the point of the tool.
		f, err := os.Open(filepath.Clean(name))
		if err != nil {
			return "", false, err
		}
		defer func() { _ = f.Close() }()
		st, err := f.Stat()
		if err != nil {
			return "", false, err
		}
		if st.IsDir() {
			src, err := dirListing(name, f)
			return src, true, err
		}
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, maxInput+1))
	if err != nil {
		return "", false, err
	}
	if len(data) > maxInput {
		return "", false, fmt.Errorf("%s: larger than %d MiB", name, maxInput>>20)
	}
	return string(data), false, nil
}

// printErr prints err as kmd's error message. Errors quote arguments, file
// and theme names and configuration values, which can hold anything (Go's
// flag and os errors quote them raw): Clean keeps them from driving the
// terminal.
func printErr(w io.Writer, err error) {
	fmt.Fprintf(w, "kmd: %s\n", render.Clean(err.Error()))
}

// usageErr prints err, a mistake in the command line, and where to look.
func usageErr(w io.Writer, err error) {
	printErr(w, err)
	_, _ = io.WriteString(w, "Try 'kmd --help'.\n")
}

// Run executes kmd and returns the process exit code.
func Run(env Env) int {
	o, err := parseArgs(env.Args, env.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		usageErr(env.Stderr, err)
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
			printErr(env.Stderr, err)
			return 2
		}
		_, _ = io.WriteString(env.Stdout, t)
		return 0
	}

	cfg, err := readConfig(env.Getenv)
	if err != nil {
		printErr(env.Stderr, err)
		return 2
	}
	s, err := resolve(o, cfg, env)
	if err != nil {
		usageErr(env.Stderr, err)
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
	resolveDirs(o.files)
	if o.slides || o.notes {
		return present(env, o, s)
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
	if viewable && (s.alwaysView || singleFolder(o.files) || long && s.pager == builtinPager) {
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

// singleFolder reports whether kmd was given one folder. Folders with a
// README were replaced by it (resolveDirs), so this one opens the file
// picker, even when its listing fits on the screen.
func singleFolder(files []string) bool {
	return len(files) == 1 && files[0] != "-" && isDir(files[0])
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
	headings []render.Heading
	blocks   []render.CodeBlock
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
		doc, base, err := loadDoc(env, name, o.section, s, in)
		if err != nil {
			printErr(env.Stderr, err)
			res.code = 1
			continue
		}
		opts := s.docOptions(name, base)
		switch {
		case o.html:
			htmlParts = append(htmlParts, md.HTML(doc, htmlOptions(o, opts, render.HTMLDiagramWidth)))
			if title == "" {
				title = docTitle(doc, name)
			}
		case o.toc:
			out.WriteString(render.TOC(doc, opts))
		default:
			if len(o.files) > 1 {
				t := render.RenderString("# "+fileTitle(name), opts)
				if s.opts.Navigate { // Text starts with a blank line
					res.headings = append(res.headings, render.Heading{Level: 1, Text: displayName(name), Line: lines + 1})
				}
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
		res.text = render.HTMLPage(title, strings.Join(htmlParts, "<hr />\n"), "kmd "+version(), s.opts.Theme)
		return res
	}
	res.text = out.String()
	return res
}

// htmlOptions are the options of the HTML export of one document (or one
// deck) rendered with ro: safe unless the user asked for --unsafe-html, code
// highlighted and diagrams drawn width columns wide. Code carries the
// document's budgets: make new options for each document.
func htmlOptions(o options, ro render.Options, width int) md.HTMLOptions {
	return md.HTMLOptions{
		TagFilter: true,
		Unsafe:    o.unsafeHTML,
		BaseURL:   ro.BaseURL,
		Code:      render.HTMLCode(ro, width),
	}
}

// addNav appends a document's links and anchors, shifted to line offset.
func (res *output) addNav(r render.Result, offset int) {
	for _, l := range r.Links {
		for i := range l.Spans {
			l.Spans[i].Line += offset
		}
		res.links = append(res.links, l)
	}
	for _, c := range r.Code {
		c.Start += offset
		c.End += offset
		res.blocks = append(res.blocks, c)
	}
	for _, h := range r.Headings {
		h.Line += offset
		res.headings = append(res.headings, h)
	}
	for k, v := range r.Anchors {
		if _, dup := res.anchors[k]; !dup {
			res.anchors[k] = v + offset
		}
	}
}

// loadDoc reads and parses a document (a file, a folder listing or stdin,
// "-") and keeps only the section named sec, if any. base is the directory
// its relative links resolve against.
func loadDoc(env Env, name, sec string, s settings, in *stdinCache) (doc *md.Node, base string, err error) {
	var src string
	folder := false
	if name == "-" && in.read {
		src = in.src
	} else {
		src, folder, err = readSource(name, env.Stdin)
		if name == "-" {
			in.src, in.read = src, true
		}
	}
	if err != nil {
		return nil, "", err
	}
	if doc, err = sectionOf(md.Parse(src, s.mdOpt), name, sec); err != nil {
		return nil, "", err
	}
	switch {
	case folder:
		base = name
	case name != "-":
		base = filepath.Dir(name)
	}
	return doc, base, nil
}

// present runs a presentation (--slides) or its notes window (--notes) and
// returns the exit status.
func present(env Env, o options, s settings) int {
	run, mode := runSlides, "--slides"
	switch {
	case o.notes:
		run, mode = runNotes, "--notes"
	case o.html:
		run = exportSlides
	}
	if !o.html && (!env.StdoutTTY || !view.Supported) {
		fmt.Fprintf(env.Stderr, "kmd: %s needs a terminal\n", mode)
		return 2
	}
	if err := run(env, o, s); err != nil {
		printErr(env.Stderr, err)
		return 1
	}
	return 0
}

// sectionOf returns the section of doc (read from name) under the heading
// named sec, or all of doc when sec is empty.
func sectionOf(doc *md.Node, name, sec string) (*md.Node, error) {
	if sec == "" {
		return doc, nil
	}
	if doc = section(doc, sec); doc == nil {
		return nil, fmt.Errorf("%s: no section matching %q", displayName(name), sec)
	}
	return doc, nil
}

// section extracts the section under the heading named q: the heading whose
// anchor is q (with or without '#'), else the first heading whose text
// contains q, ignoring case.
func section(doc *md.Node, q string) *md.Node {
	slug := render.Slug(strings.TrimPrefix(q, "#"))
	if sec := md.Section(doc, func(h *md.Node) bool { return render.Slug(md.PlainText(h)) == slug }); sec != nil {
		return sec
	}
	lq := strings.ToLower(strings.TrimSpace(q))
	if lq == "" {
		return nil
	}
	return md.Section(doc, func(h *md.Node) bool { return strings.Contains(strings.ToLower(md.PlainText(h)), lq) })
}

func displayName(name string) string {
	if name == "-" {
		return "stdin"
	}
	return name
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

// fileTitle is the heading of one of several files: its name as literal
// text, never Markdown (`www.evil.example.md` is not a link, a newline
// does not start a block).
func fileTitle(name string) string {
	if name == "-" {
		return "stdin"
	}
	return mdEscape(filepath.Base(name))
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
		printErr(env.Stderr, err)
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
		Lines:    strings.Split(strings.TrimSuffix(res.text, "\n"), "\n"),
		Links:    res.links,
		Anchors:  res.anchors,
		Headings: res.headings,
		Code:     res.blocks,
	}
}

// runViewer opens the viewer; first (rendered for a terminal firstCols
// wide) is its first page, or nil to render it.
func runViewer(env Env, o options, s settings, in *stdinCache, watch bool, first *view.Page, firstCols int) error {
	start := ""
	if len(o.files) == 1 && o.files[0] != "-" {
		start = o.files[0]
	}
	load := func(path string, cols int) (*view.Page, error) {
		s := viewerSettings(s, max(cols, 1))
		lo := options{files: o.files}
		if path == "" || path == start {
			lo.section = o.section // the document kmd started with
		} else {
			lo.files = []string{path}
		}
		var errs strings.Builder
		quiet := env
		quiet.Stderr = &errs // errors go to the status bar, not the screen
		res := renderFiles(quiet, lo, s, in)
		if res.code != 0 && len(lo.files) == 1 {
			return nil, errors.New(strings.TrimPrefix(strings.TrimSpace(errs.String()), "kmd: "))
		}
		return pageOf(res), nil
	}
	sel, match, bar := render.ViewerStyles(s.opts.Theme, s.opts.Color, s.opts.TrueColor)
	root := "." // the picker lists the folder kmd was started on, or the working one
	if start != "" {
		root = start
		if !isDir(start) {
			root = filepath.Dir(start)
		}
	}
	opt := view.Options{Load: load, Start: start, Mouse: s.mouse, Watch: watch, Poll: env.Poll,
		Select: sel, Match: match, Bar: bar, First: first, FirstCols: firstCols, Root: root}
	if env.View != nil {
		return env.View(opt)
	}
	return view.Run(opt)
}
