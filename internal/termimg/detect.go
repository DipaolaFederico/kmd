package termimg

import "strings"

// Detect guesses the best protocol from environment variables only (terminal
// queries are slow and need raw mode). truecolor tells whether 24-bit color
// is available. Inside tmux or GNU screen, Blocks is returned because
// graphics passthrough is unreliable.
func Detect(getenv func(string) string, truecolor bool) Protocol {
	term := strings.ToLower(getenv("TERM"))
	program := strings.ToLower(getenv("TERM_PROGRAM"))

	switch {
	case getenv("TMUX") != "" || getenv("STY") != "" ||
		strings.HasPrefix(term, "tmux") || strings.HasPrefix(term, "screen"):
		return Blocks
	case term == "dumb":
		return None
	}

	switch program {
	case "ghostty", "kitty":
		return Kitty
	case "iterm.app", "wezterm", "vscode", "mintty":
		return ITerm2
	}

	switch {
	case term == "xterm-kitty" || term == "xterm-ghostty" || getenv("KITTY_WINDOW_ID") != "":
		return Kitty
	case term == "mintty" || strings.EqualFold(getenv("LC_TERMINAL"), "iTerm2") ||
		getenv("WEZTERM_PANE") != "":
		return ITerm2
	case getenv("KONSOLE_VERSION") != "":
		return Kitty
	case strings.HasPrefix(term, "foot") || strings.HasPrefix(term, "mlterm") ||
		getenv("WT_SESSION") != "":
		return Sixel
	case truecolor:
		return Blocks
	}
	return None
}
