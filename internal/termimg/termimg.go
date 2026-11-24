// Package termimg renders local raster images (PNG, JPEG, GIF) inside a
// terminal using Unicode half blocks or one of the terminal graphics
// protocols (kitty, iTerm2, sixel).
//
// Images are never fetched from the network: only local files and base64
// data: URIs are accepted. Every input is treated as untrusted: sizes and
// dimensions are validated from the image header before any pixel is
// decoded.
//
// # Output convention
//
// Render returns a Frame whose Lines the caller prints one per line, each
// after the same left margin of spaces and followed by a newline:
//
//   - Blocks: one line per cell row; every line is exactly Frame.Cols columns
//     wide and ends with an SGR reset. Safe inside `less -R`.
//   - Kitty: the whole image is sent with C=1 (the cursor does not move) on
//     Lines[0]; Lines[1:] are empty strings that reserve the remaining rows,
//     so after the caller's newlines the cursor sits below the image.
//   - ITerm2 and Sixel: a single line. The terminal itself moves the cursor
//     to the last text row covered by the image; the caller's newline then
//     moves below it. The caller must not add placeholder lines.
//
// The graphics protocols (Kitty, ITerm2, Sixel) do not survive a pager such
// as less: callers must write directly to the terminal when they are used.
package termimg

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"
)

// Protocol selects how images are drawn.
type Protocol uint8

// Supported protocols.
const (
	None   Protocol = iota // show alt text only
	Blocks                 // Unicode half blocks (▀) with 24-bit fg/bg colors
	Kitty                  // kitty graphics protocol
	ITerm2                 // iTerm2 inline images (OSC 1337)
	Sixel                  // DEC sixel graphics
)

var protocolNames = [...]string{None: "none", Blocks: "blocks", Kitty: "kitty", ITerm2: "iterm2", Sixel: "sixel"}

// String returns the canonical lower-case protocol name.
func (p Protocol) String() string {
	if int(p) < len(protocolNames) {
		return protocolNames[p]
	}
	return fmt.Sprintf("Protocol(%d)", uint8(p))
}

// ParseProtocol parses a protocol name (case-insensitive): none, blocks,
// kitty, iterm2 (alias iterm) or sixel. "auto" is not accepted: callers
// resolve it with Detect.
func ParseProtocol(s string) (Protocol, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return None, nil
	case "blocks":
		return Blocks, nil
	case "kitty":
		return Kitty, nil
	case "iterm2", "iterm":
		return ITerm2, nil
	case "sixel":
		return Sixel, nil
	}
	return None, fmt.Errorf("termimg: unknown image protocol %q (want none, blocks, kitty, iterm2 or sixel)", s)
}

// Cell is the pixel size of one terminal cell. Zero (or negative) values mean
// unknown, in which case an 8×16 cell is assumed.
type Cell struct{ W, H int }

// Default cell metrics used when the terminal does not report pixel sizes.
const (
	defaultCellW = 8
	defaultCellH = 16
)

// Frame is a rendered image. See the package documentation for how Lines
// must be printed. Cols is the visible width in columns of the image.
type Frame struct {
	Lines []string
	Cols  int
	Rows  int // terminal rows the image covers (more than len(Lines) for ITerm2 and Sixel)
}

// Clear returns the sequence that removes every image p has drawn on the
// screen when clearing the text does not ("" when it does): kitty keeps
// its images apart from the text.
func Clear(p Protocol) string {
	if p == Kitty {
		return "\x1b_Ga=d,d=A,q=2\x1b\\" // A: free the image data too
	}
	return ""
}

// Errors returned by Load and Render.
var (
	ErrRemote      = errors.New("remote images are not fetched")
	ErrOutside     = errors.New("termimg: image outside the document directory")
	ErrTooLarge    = errors.New("termimg: image too large")
	ErrUnsupported = errors.New("termimg: unsupported image format")
	ErrNotRegular  = errors.New("termimg: not a regular file")
	ErrDataURI     = errors.New("termimg: malformed data URI")
	ErrNilImage    = errors.New("termimg: nil image")
)

// Image is a validated image ready to render. Its pixels are decoded lazily,
// at most once, and only when a protocol needs them. An Image is safe for
// concurrent use.
type Image struct {
	data   []byte // original encoded bytes
	format string // "png", "jpeg" or "gif"
	w, h   int    // pixel size from the header

	once   sync.Once
	pixels image.Image
	err    error
}

// Format returns the image format: "png", "jpeg" or "gif".
func (img *Image) Format() string { return img.format }

// Size returns the image dimensions in pixels.
func (img *Image) Size() (w, h int) { return img.w, img.h }

// decode decodes the pixels once. Dimensions were validated by Load, so the
// allocation is bounded.
func (img *Image) decode() (image.Image, error) {
	img.once.Do(func() {
		img.pixels, img.err = decodeBytes(img.data, img.format)
	})
	return img.pixels, img.err
}

// Render returns the lines to print for img fitted within maxCols columns and
// maxRows rows (a non-positive limit means unlimited). The aspect ratio is
// preserved and the image is never enlarged beyond its natural size, i.e. one
// image pixel per screen pixel given cell. Protocol None returns an empty
// Frame: the caller shows the alt text instead.
func Render(img *Image, p Protocol, maxCols, maxRows int, cell Cell) (Frame, error) {
	if img == nil {
		return Frame{}, ErrNilImage
	}
	switch p {
	case Blocks:
		return renderBlocks(img, maxCols, maxRows, cell)
	case Kitty:
		return renderKitty(img, fit(img.w, img.h, maxCols, maxRows, cell))
	case ITerm2:
		return renderITerm2(img, fit(img.w, img.h, maxCols, maxRows, cell)), nil
	case Sixel:
		return renderSixel(img, fit(img.w, img.h, maxCols, maxRows, cell))
	case None:
		return Frame{}, nil
	}
	return Frame{}, fmt.Errorf("termimg: unknown protocol %v", p)
}
