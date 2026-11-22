# kmd

A Markdown previewer for the terminal: **fast** (a few milliseconds),
**beautiful**, and **dependency-free** (Go standard library only).

```sh
kmd README.md              # render (pages through less when it doesn't fit)
kmd -f notes.md            # live preview: re-renders on save
cat doc.md | kmd           # read from stdin
kmd --toc CHANGELOG.md     # table of contents
kmd --html doc.md > d.html # export HTML using the theme colors
kmd -t catppuccin-mocha    # pick a theme
```

## Features

| | |
|---|---|
| **Full CommonMark** | 652/652 examples of the 0.31.2 specification (checked in CI) |
| **GitHub Flavored Markdown** | tables, task lists, strikethrough, extended autolinks, tag filter (24/24 GFM examples), footnotes, `> [!NOTE]` alerts, YAML front matter |
| **Syntax highlighting** | ~25 languages + `diff`, in labeled boxes |
| **Clickable links** | OSC 8 hyperlinks (with the URL shown as a fallback when not on a terminal) |
| **Inline images** | kitty, iTerm2, sixel, or Unicode blocks (any truecolor terminal); local files and `data:` URIs only — nothing is ever downloaded |
| **Diagrams** | Mermaid (flowchart, state, sequence, pie) and Graphviz DOT, drawn with box-drawing characters |
| **Math** | `$…$`, `$$…$$` and ` ```math `: LaTeX → Unicode, with 2D fractions, roots, sums and matrices |
| **Custom themes** | 13 bundled themes + your own in `~/.config/kmd/themes/*.theme` |

## Options

```
Display:
  -w, --width N            render width (default: terminal width, max 110)
  -t, --theme NAME         theme name or path to a .theme file (default: auto)
  -c, --color WHEN         auto | always | never   (honors NO_COLOR)
  -i, --images MODE        auto | blocks | kitty | iterm2 | sixel | none
      --hyperlinks WHEN    auto | always | never
      --no-math            show LaTeX source instead of rendered formulas
      --no-diagrams        show mermaid/dot source instead of diagrams
      --commonmark         strict CommonMark (no GitHub extensions)
  -P, --no-pager           never page long output
Output modes:
      --toc                print only the table of contents
      --html               export a standalone HTML page (safe: raw HTML removed)
      --unsafe-html        with --html: keep raw HTML and every URL
  -f, --follow             re-render whenever the file changes
Themes and configuration:
      --list-themes        bundled and user themes (with color swatches)
      --theme-template T   print theme T as an editable file
      --config             config location, effective settings and a template
Other:
      --completion SHELL   bash | zsh | fish
  -v, --version · -h, --help
```

See `man kmd` for the full reference.

## Themes

Bundled: `tokyo-night` (default dark), `tokyo-night-day` (default light),
`catppuccin-mocha`, `catppuccin-latte`, `dracula`, `nord`, `gruvbox-dark`,
`gruvbox-light`, `solarized-dark`, `solarized-light`, `one-dark`,
`github-dark`, `github-light`. With `theme = auto`, kmd picks a light or dark
theme based on `COLORFGBG`.

A theme is defined by a **17-color palette** from which every role is derived
(headings, code, syntax, math, diagrams, charts). To create your own:

```sh
mkdir -p ~/.config/kmd/themes
kmd --theme-template nord > ~/.config/kmd/themes/mine.theme   # edit colors
kmd -t mine README.md
```

```ini
inherit  = nord         # base theme
blue     = #5e81ac      # change a palette color (re-derives every role)
heading2 = #bf616a      # or override a single role
```

## Configuration

`~/.config/kmd/config` (or `$XDG_CONFIG_HOME/kmd/config`, or `$KMD_CONFIG`):

```ini
theme       = auto
dark-theme  = catppuccin-mocha
light-theme = github-light
max-width   = 100
images      = auto
pager       = less -R
```

Precedence: flags > environment variables (`KMD_THEME`, `KMD_PAGER`,
`NO_COLOR`) > config file.

## Installation

```sh
make install                          # ~/.local/bin/kmd + man page + completions
sudo make install PREFIX=/usr/local
```

Fedora (RPM): see [docs/PACKAGING.md](docs/PACKAGING.md).

## Local CI

```sh
make ci      # every gate, in order; stops at the first failure
make hooks   # run `make ci` before every git push
```

| Stage       | Tool                                                 | Gate                            |
|-------------|------------------------------------------------------|---------------------------------|
| format      | `gofmt -s`                                           | no diff                         |
| lint        | `go vet` + `staticcheck`                             | no findings                     |
| test        | `go test -race` (includes CommonMark/GFM spec)       | all green                       |
| coverage    | `go test -coverprofile`                              | ≥ 90 % (`COVER_MIN`)            |
| security    | `govulncheck` + `gosec`                              | no issues                       |
| build       | `go build -trimpath`, `CGO_ENABLED=0`                | static binary                   |
| packaging   | `groff`, `appstreamcli --pedantic`, completions      | valid Fedora metadata           |
| performance | `scripts/perf.sh` (whole process, 200 runs)          | p95 < 10 ms (`PERF_MAX_MS`)     |

Extras: `make spec` (conformance), `make fuzz` (5 fuzzers), `make bench`,
`make rpm`.

Lint and security tools are installed at pinned versions into `.tools/` (the
first run needs network access). **The binary has no external dependencies**:
`go.mod` has no `require` lines.

## Security

Documents are treated as untrusted (READMEs from cloned repos, downloads,
`curl | kmd`). The design was reviewed and fuzzed against that threat model:

- **Terminal injection**: C0/C1 control characters, DEL and Unicode bidi
  overrides ("Trojan Source") are stripped from all document text on output,
  including characters produced via entities like `&#27;`. OSC 8 link targets
  must be valid UTF-8 without controls or invisible characters. File names in
  error messages are sanitized too.
- **Images**: local regular files and `data:` URIs only, opened non-blocking
  and re-checked on the descriptor; ≤ 20 MiB and ≤ 40 megapixels checked
  *before* decoding; only PNG/JPEG/GIF; nothing is ever downloaded (no
  tracking pixels). Each image is decoded once per document, at most 50
  distinct images and 64 MiB of image data per document.
- **`--html` export**: safe by default, like cmark: raw HTML is omitted and
  `javascript:`, `vbscript:`, `file:` and non-image `data:` URLs are dropped;
  the page carries a restrictive Content-Security-Policy. `--unsafe-html`
  keeps everything (trusted input only).
- **Resource limits**: input ≤ 16 MiB, ≤ 250,000 blocks (a notice is shown
  beyond), container nesting ≤ 100, inline nesting ≤ 100, tables ≤ 1,000
  columns and 524,288 cells, ≤ 100 diagrams/display formulas per document.
  Every construct known to make Markdown parsers quadratic (nested brackets,
  emphasis with links, backtick runs, unterminated comments, link
  parentheses) is covered by pathological tests with time limits.

Images cost what decoding costs: a large PNG adds a few milliseconds
(`-i none` disables images; kitty passes PNGs through without decoding).

## Layout

```
main.go             entry point: terminal and cell size, GC disabled
internal/md         CommonMark/GFM parser → tree; HTML exporter
internal/render     layout, themes, tables, code, extras integration
internal/diagram    Mermaid/DOT → Unicode art (layered layout)
internal/tex        LaTeX → Unicode (linear and 2D)
internal/termimg    images: kitty, iTerm2, sixel, blocks
internal/styled     shared vocabulary (spans + Unicode width)
internal/term       terminal size via ioctl
internal/cli        flags, configuration, commands, pager, follow mode
packaging/          RPM spec and AppStream metadata
docs/kmd.1          man page
```

## License

MIT. The CommonMark/GFM specification examples in `internal/md/testdata`
(CC-BY-SA 4.0) are used by the tests only.
