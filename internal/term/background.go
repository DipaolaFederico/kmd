package term

import "bytes"

// Background queries are written to the terminal as one string: OSC 11 asks
// for the background colour, DA1 (primary device attributes) is answered by
// virtually every terminal, so its reply ends the wait at once on terminals
// that ignore OSC 11, and the timeout only matters for the rare ones that
// answer neither.
const backgroundQuery = "\x1b]11;?\x1b\\\x1b[c"

// maxReply bounds how much of the terminal's answer is read: a real reply to
// both queries is well under 100 bytes.
const maxReply = 256

// oscPrefix starts the only reply form accepted for OSC 11.
var oscPrefix = []byte("\x1b]11;rgb:")

// ParseBackground extracts the background colour from a terminal's answer
// to the background query, ignoring anything around it (the DA1 reply, keys
// typed meanwhile). r, g and b are between 0 and 1. The reply must be
// exactly ESC ] 11 ; rgb:R/G/B ended by BEL or ESC \, each channel 1–4 hex
// digits; anything else, or an answer longer than maxReply, is rejected.
func ParseBackground(reply []byte) (r, g, b float64, ok bool) {
	if len(reply) > maxReply {
		return 0, 0, 0, false
	}
	i := bytes.Index(reply, oscPrefix)
	if i < 0 {
		return 0, 0, 0, false
	}
	rest := reply[i+len(oscPrefix):]
	var ch [3]float64
	for k := range ch {
		if k > 0 {
			if len(rest) == 0 || rest[0] != '/' {
				return 0, 0, 0, false
			}
			rest = rest[1:]
		}
		v, n := hexChannel(rest)
		if n == 0 {
			return 0, 0, 0, false
		}
		ch[k], rest = v, rest[n:]
	}
	if !bytes.HasPrefix(rest, []byte("\a")) && !bytes.HasPrefix(rest, []byte("\x1b\\")) {
		return 0, 0, 0, false
	}
	return ch[0], ch[1], ch[2], true
}

// hexChannel reads one colour channel of 1–4 hex digits, scaled to 0–1 as
// XParseColor does (f, ff, fff and ffff are all full intensity). n is the
// number of digits read, 0 when s does not start with 1–4 of them.
func hexChannel(s []byte) (v float64, n int) {
	x := 0
	for n < len(s) && n < 5 {
		d := hexDigit(s[n])
		if d < 0 {
			break
		}
		x = x<<4 | d
		n++
	}
	if n == 0 || n > 4 {
		return 0, 0
	}
	return float64(x) / float64(int(1)<<(4*n)-1), n
}

func hexDigit(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// IsLight reports whether a background colour (channels 0–1) is light: its
// luma (Rec. 709 weights) is above one half, so dark text reads better on it.
func IsLight(r, g, b float64) bool {
	return 0.2126*r+0.7152*g+0.0722*b > 0.5
}

// da1Done reports whether buf holds a complete DA1 reply, ESC [ ? params c,
// which follows the OSC 11 reply when the terminal sends one.
func da1Done(buf []byte) bool {
	i := bytes.LastIndex(buf, []byte("\x1b[?"))
	if i < 0 {
		return false
	}
	for _, c := range buf[i+3:] {
		switch {
		case c == 'c':
			return true
		case c != ';' && (c < '0' || c > '9'):
			return false
		}
	}
	return false
}
