package render

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DipaolaFederico/kmd/internal/bridge"
	"github.com/DipaolaFederico/kmd/internal/md"
	irender "github.com/DipaolaFederico/kmd/internal/render"
	"github.com/DipaolaFederico/kmd/internal/termimg"
)

// ErrTooLarge is returned for documents larger than 16 MiB.
var ErrTooLarge = errors.New("render: document larger than 16 MiB")

// DefaultWidth is the width used when [Options.Width] is 0.
const DefaultWidth = 80

// MaxWidth is the largest [Options.Width] accepted.
const MaxWidth = 1000

// Bounds of the image size options: enough for any real terminal, small
// enough that one image cannot take long to scale.
const (
	maxImageRows = 1000
	maxCell      = 512
)

// Options controls [Render]. The zero value renders GitHub Flavored
// Markdown with math and diagrams, 80 columns wide, as plain text: no
// colour, no hyperlinks, no images and no file access.
type Options struct {
	// Width is the number of columns available, margins included: 0 means
	// [DefaultWidth]; at most [MaxWidth].
	Width int
	// Theme colours the output; nil means DefaultTheme(true).
	Theme *Theme
	// Color emits SGR colour and attribute sequences.
	Color bool
	// TrueColor uses 24-bit colours instead of the 256-colour palette.
	TrueColor bool
	// Hyperlinks emits OSC 8 hyperlinks for links.
	Hyperlinks bool

	// Images selects how images are drawn; the default, ImagesNone, shows
	// their alt text. Images are only drawn when Color is set.
	Images ImageMode
	// ImageRows is the maximum image height in rows; 0 means 20; at most
	// 1000.
	ImageRows int
	// CellWidth and CellHeight are the terminal cell size in pixels, used
	// to keep images' aspect ratio; 0 means unknown (8×16 is assumed); at
	// most 512.
	CellWidth, CellHeight int

	// BaseDir is the directory relative links and images resolve against,
	// usually the document's directory. Local images are only read from
	// inside it; with BaseDir == "" only data: URI images are drawn.
	BaseDir string

	// CommonMark parses strict CommonMark 0.31.2, without extensions.
	CommonMark bool
	// NoMath leaves $…$ and $$…$$ as plain text.
	NoMath bool
	// NoDiagrams shows mermaid and Graphviz blocks as code.
	NoDiagrams bool
}

// HTMLOptions controls [HTML]. The zero value produces a safe HTML fragment
// from GitHub Flavored Markdown with math.
type HTMLOptions struct {
	// CommonMark parses strict CommonMark 0.31.2, without extensions.
	CommonMark bool
	// NoMath leaves $…$ and $$…$$ as plain text.
	NoMath bool
	// Unsafe passes raw HTML and every URL through. Only for trusted
	// documents: by default raw HTML is omitted, the GitHub tag filter
	// applies and javascript:, vbscript:, file: and non-image data: URLs
	// are dropped.
	Unsafe bool
	// Standalone wraps the fragment in a complete page coloured by Theme,
	// whose Content-Security-Policy forbids scripts.
	Standalone bool
	// Title is the page title (Standalone); "" uses the first heading.
	Title string
	// Theme colours the page (Standalone); nil means DefaultTheme(true).
	Theme *Theme
}

// ImageMode selects how [Render] draws images.
type ImageMode uint8

// Image modes. The terminal graphics protocols (ImagesKitty, ImagesITerm2,
// ImagesSixel) only work when the output is written straight to a terminal
// that supports them, not through a pager.
const (
	ImagesNone   ImageMode = iota // alt text only
	ImagesBlocks                  // Unicode half blocks with 24-bit colours
	ImagesKitty                   // kitty graphics protocol
	ImagesITerm2                  // iTerm2 inline images
	ImagesSixel                   // DEC sixel graphics
)

var imageProtocols = [...]termimg.Protocol{
	ImagesNone:   termimg.None,
	ImagesBlocks: termimg.Blocks,
	ImagesKitty:  termimg.Kitty,
	ImagesITerm2: termimg.ITerm2,
	ImagesSixel:  termimg.Sixel,
}

// String returns the mode's name: none, blocks, kitty, iterm2 or sixel.
func (m ImageMode) String() string {
	if int(m) < len(imageProtocols) {
		return imageProtocols[m].String()
	}
	return fmt.Sprintf("ImageMode(%d)", uint8(m))
}

// ParseImageMode parses an image mode name, ignoring case: none, blocks,
// kitty, iterm2 (or iterm) or sixel.
func ParseImageMode(s string) (ImageMode, error) {
	p, err := termimg.ParseProtocol(s)
	if err != nil {
		return ImagesNone, fmt.Errorf("render: unknown image mode %q (want none, blocks, kitty, iterm2 or sixel)", s)
	}
	for m, q := range imageProtocols {
		if q == p {
			return ImageMode(m), nil
		}
	}
	return ImagesNone, fmt.Errorf("render: unknown image mode %q", s) // unreachable: every protocol has a mode
}

func init() {
	bridge.Resolve = func(o any) (bridge.Config, error) { return o.(Options).resolve() }
}

// parseOptions returns the parser options for the CommonMark and NoMath
// switches.
func parseOptions(commonmark, noMath bool) md.Options {
	p := md.GFM
	if commonmark {
		p = md.Options{}
	}
	p.Math = p.Math && !noMath
	return p
}

// resolve converts o to the internal renderer and parser options.
func (o Options) resolve() (bridge.Config, error) {
	switch {
	case o.Width < 0 || o.Width > MaxWidth:
		return bridge.Config{}, fmt.Errorf("render: width %d out of range [0, %d]", o.Width, MaxWidth)
	case int(o.Images) >= len(imageProtocols):
		return bridge.Config{}, fmt.Errorf("render: invalid image mode %d", o.Images)
	case o.ImageRows < 0 || o.ImageRows > maxImageRows:
		return bridge.Config{}, fmt.Errorf("render: image rows %d out of range [0, %d]", o.ImageRows, maxImageRows)
	case o.CellWidth < 0 || o.CellWidth > maxCell || o.CellHeight < 0 || o.CellHeight > maxCell:
		return bridge.Config{}, fmt.Errorf("render: cell size %d×%d out of range [0, %d]", o.CellWidth, o.CellHeight, maxCell)
	}
	w := o.Width
	if w == 0 {
		w = DefaultWidth
	}
	return bridge.Config{
		Render: irender.Options{
			Width:      w,
			Theme:      o.Theme.internal(),
			Color:      o.Color,
			TrueColor:  o.TrueColor,
			Hyperlinks: o.Hyperlinks,
			Images:     imageProtocols[o.Images],
			Cell:       termimg.Cell{W: o.CellWidth, H: o.CellHeight},
			ImageRows:  o.ImageRows,
			BaseDir:    o.BaseDir,
			Math:       !o.NoMath,
			Diagrams:   !o.NoDiagrams,
		},
		Parse: parseOptions(o.CommonMark, o.NoMath),
	}, nil
}

// Render renders the Markdown document src for a terminal. The result is
// a sequence of lines, each ending in a newline; it starts with a blank
// line and has a two-column left margin.
//
// The error is non-nil only for invalid options or a document larger than
// 16 MiB ([ErrTooLarge]): any text is valid Markdown.
func Render(src string, o Options) (string, error) {
	c, err := o.resolve()
	if err != nil {
		return "", err
	}
	if len(src) > md.MaxInput {
		return "", ErrTooLarge
	}
	return irender.Render(md.Parse(src, c.Parse), c.Render).Text, nil
}

// HTML renders the Markdown document src as HTML: a fragment, or a
// complete page with [HTMLOptions.Standalone]. The error is non-nil only
// for a document larger than 16 MiB ([ErrTooLarge]).
func HTML(src string, o HTMLOptions) (string, error) {
	if len(src) > md.MaxInput {
		return "", ErrTooLarge
	}
	doc := md.Parse(src, parseOptions(o.CommonMark, o.NoMath))
	body := md.HTML(doc, md.HTMLOptions{TagFilter: true, Unsafe: o.Unsafe})
	if !o.Standalone {
		return body, nil
	}
	title := o.Title
	if title == "" {
		title = firstHeading(doc)
	}
	return irender.HTMLPage(title, body, "kmd", o.Theme.internal()), nil
}

// firstHeading returns the text of the document's first top-level heading.
func firstHeading(doc *md.Node) string {
	for c := doc.FirstChild; c != nil; c = c.Next {
		if c.Type == md.Heading {
			return strings.TrimSpace(md.PlainText(c))
		}
	}
	return ""
}
