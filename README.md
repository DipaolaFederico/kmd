<div align="center">

# kmd

**Markdown previewer for the terminal**

[![Release](https://img.shields.io/github/v/release/DipaolaFederico/kmd?sort=semver&label=release)](https://github.com/DipaolaFederico/kmd/releases/latest)
[![CI](https://github.com/DipaolaFederico/kmd/actions/workflows/ci.yml/badge.svg)](https://github.com/DipaolaFederico/kmd/actions/workflows/ci.yml)
[![Go](https://img.shields.io/github/go-mod/go-version/DipaolaFederico/kmd)](go.mod)
[![Dependencies](https://img.shields.io/badge/dependencies-none-brightgreen)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-Linux%20%7C%20macOS%20%7C%20FreeBSD%20%7C%20Windows-lightgrey)](https://github.com/DipaolaFederico/kmd/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Installation](#-installation) · [Usage](#-usage) · [Viewer](#-viewer) · [Themes](#-themes) · [Configuration](#%EF%B8%8F-configuration) · [Security](#-security)

</div>

kmd renders Markdown files in the terminal with colors, tables, syntax
highlighting, diagrams and math. It is a single static binary written in Go
with no dependencies outside the standard library, and renders a typical
document in under 10 ms, process start included. Documents are treated as
untrusted input.

<p align="center">
  <img src="docs/images/screenshot.png" alt="kmd rendering docs/images/demo.md with the tokyo-night theme" width="720">
</p>

<p align="center"><sub><code>kmd docs/images/demo.md</code> with the <code>tokyo-night</code> theme</sub></p>

## ✨ Features

| | Feature | Details |
|---|---|---|
| 📝 | **CommonMark** | 652/652 examples of the 0.31.2 specification, checked in CI |
| 🐙 | **GitHub Flavored Markdown** | tables, task lists, strikethrough, autolinks, footnotes, `> [!NOTE]` alerts, YAML front matter (24/24 GFM examples) |
| 🎨 | **Syntax highlighting** | about 25 languages plus `diff`, in labeled boxes |
| 🧭 | **Viewer** | long documents open in a built-in viewer; links between Markdown files can be followed and navigated back |
| 🔗 | **Hyperlinks** | clickable OSC 8 links; relative links resolved to absolute `file://` URLs |
| 🖼️ | **Images** | kitty, iTerm2, sixel or Unicode blocks; local files and `data:` URIs only |
| 📊 | **Diagrams** | Mermaid (flowchart, state, sequence, class, ER, Gantt, pie) and Graphviz DOT, drawn with box-drawing characters |
| ∑ | **Math** | `$…$`, `$$…$$` and ` ```math ` blocks rendered as Unicode, including fractions, roots, sums and matrices |
| 🌈 | **Themes** | 13 bundled themes; custom themes from a 17-color palette |
| 🔄 | **Live preview** | `-f` re-renders the file every time it is saved |
| 🎤 | **Presentations** | `--slides` shows a document one slide at a time |
| 🌐 | **HTML export** | standalone page with the theme colors; raw HTML and unsafe URLs removed by default |

## 📦 Installation

### Prebuilt binaries

Every [release](https://github.com/DipaolaFederico/kmd/releases/latest)
includes binaries for Linux (amd64, arm64), macOS (amd64, arm64), FreeBSD
(amd64) and Windows (amd64), a `SHA256SUMS` file and a signed build
provenance attestation. Each archive contains `kmd`, the man page `kmd.1`,
`README.md` and `LICENSE`.

```sh
# Linux amd64; for other platforms change "linux-amd64" (see the release page)
VERSION=$(curl -fsSL https://api.github.com/repos/DipaolaFederico/kmd/releases/latest \
  | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')
BASE=https://github.com/DipaolaFederico/kmd/releases/download/$VERSION
ARCHIVE=kmd-$VERSION-linux-amd64
[ -n "$VERSION" ] &&
  curl -fsSLO "$BASE/$ARCHIVE.tar.gz" -O "$BASE/SHA256SUMS" &&
  sha256sum --check --ignore-missing SHA256SUMS &&
  tar -xzf "$ARCHIVE.tar.gz" &&
  install -Dm755 "$ARCHIVE/kmd" ~/.local/bin/kmd &&
  install -Dm644 "$ARCHIVE/kmd.1" ~/.local/share/man/man1/kmd.1
```

The checksum confirms the download is intact. To also confirm that the
archive was built by this repository's release workflow, verify its build
provenance with the GitHub CLI before the `install` step:

```sh
gh attestation verify "$ARCHIVE.tar.gz" --repo DipaolaFederico/kmd
```

### From source

Requires Go 1.25 or later.

```sh
git clone https://github.com/DipaolaFederico/kmd.git && cd kmd
make install                        # ~/.local/bin/kmd, man page, bash/zsh/fish completions
sudo make install PREFIX=/usr/local # system-wide
```

With Go only: `go install github.com/DipaolaFederico/kmd@latest` (installs
the binary, without the man page or completions).

### 🐧 Fedora

```sh
sudo dnf copr enable dipaolafederico/kmd
sudo dnf install kmd
```

The package (spec and AppStream metadata in [`packaging/`](packaging)) is
built from every release. Its inclusion in the official Fedora
repositories is in progress; see [docs/PACKAGING.md](docs/PACKAGING.md).

### Shell completions

`make install` and the RPM install them. Otherwise:

```sh
kmd --completion bash > ~/.local/share/bash-completion/completions/kmd
kmd --completion zsh  > ~/.zfunc/_kmd     # any directory listed in $fpath
kmd --completion fish > ~/.config/fish/completions/kmd.fish
```

## 🚀 Usage

```sh
kmd README.md                 # render; opens the viewer if it does not fit on screen
kmd docs/*.md                 # several files, one after another
kmd docs/                     # a folder: its README.md, or a list of its documents
cat notes.md | kmd            # read from standard input
kmd -f notes.md               # live preview: re-render on every save
kmd --view short.md           # open the viewer even if the document fits
kmd -P README.md | less -R    # print without the viewer
kmd --toc CHANGELOG.md        # table of contents only
kmd -s install README.md      # only the section under the "Installation" heading
kmd --html doc.md > doc.html  # export a standalone HTML page
kmd --slides talk.md          # present: one slide per "---" (or per # / ## heading)
kmd -t catppuccin-mocha doc.md
kmd -w 80 doc.md              # fixed render width
```

### Common tasks

| Task | Command |
|---|---|
| 📖 Read a project's documentation and follow its links | `kmd README.md`, then `Tab` and `Enter` |
| 🗂️ Browse a folder of notes or documentation | `kmd docs/` |
| ✍️ Preview a file while editing it | `kmd -f draft.md` |
| 📋 Get an overview of a long document | `kmd --toc doc.md` |
| 🎯 Read one section, or print it from a script | `kmd -s install README.md` |
| 📤 Share a rendered document | `kmd --html doc.md > doc.html` |
| 🎤 Present from the terminal | `kmd --slides talk.md` (`-f` reloads on save) |
| 🔌 Render remote Markdown without downloading it to disk | `curl -s https://…/README.md \| kmd` |
| 🖨️ Plain output for scripts or logs | `kmd -c never -P doc.md` |

### Options

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
  -P, --no-pager           never page long output (no viewer)
      --view               open the viewer even when the document fits
Output modes:
      --toc                print only the table of contents
  -s, --section NAME       only the section under heading NAME (anchor or text)
      --html               export a standalone HTML page (safe: raw HTML removed)
      --unsafe-html        with --html: keep raw HTML and every URL
  -f, --follow             live preview: re-render whenever the file changes
      --slides             present the document as slides
Themes and configuration:
      --list-themes        bundled and user themes (with color swatches)
      --theme-template T   print theme T as an editable file
      --config             config location, effective settings and a template
Other:
      --completion SHELL   bash | zsh | fish
  -v, --version · -h, --help
```

The full reference is in the man page: `man kmd`.

## 🧭 Viewer

Documents longer than the terminal open in kmd's viewer. Links to other
Markdown files open in the viewer itself, at the target `#heading` if there
is one, so a folder of documents can be browsed like a wiki. A folder
(`kmd notes/`, or a link to one) opens its `README.md` or `index.md`, or a
list of its Markdown files and subfolders.

| Keys | Action |
|---|---|
| `Tab` / `Shift-Tab`, click | select the next / previous link |
| `Enter` `l` `→` | open the selected link: `.md` files in the viewer; web links, images and PDFs in the desktop's default application |
| `Backspace` `h` `←` · `H` | back one document · back to the first document |
| `j` `k` `Space` `b` `d` `u` `g` `G` | scroll (arrows, PgUp/PgDn, Home/End and the mouse wheel also work) |
| `t` · `[` `]` | table of contents (choose a heading and press Enter) · previous / next section |
| `c` | copy a code block on screen to the clipboard (press again for the next block on screen) |
| `/` `n` `N` | search, next / previous match |
| `e` · `r` · `?` · `q` | edit in `$VISUAL`/`$EDITOR` and reload · reload · help · quit |

The status bar shows the target of the selected link and the documents
visited. Links that leave kmd (web, mail, desktop applications) need two
clicks: the first shows the target, the second opens it.

`c` copies the source of the code block, without the box or the colors.
Locally it uses `wl-copy`, `xclip` or `xsel` (`pbcopy` on macOS); over SSH,
or when none is installed, it asks the terminal through OSC 52, which kitty,
WezTerm, foot, Alacritty, iTerm2, Windows Terminal and tmux support.

Inside the viewer, images are drawn with Unicode blocks, since terminal
graphics protocols cannot scroll with the text. To use an external pager
instead, set `pager = less -R` in the configuration or `KMD_PAGER`.
`mouse = off` leaves the mouse to the terminal for text selection.

### Presentations

`kmd --slides talk.md` splits the document at every `---` line (or, when
there is none, before every `#` and `##` heading) and shows one slide at a
time, centered. `→` `Space` `PgDn` go forward, `←` `PgUp` back, `g` `G` to
the first and last slide; `j` `k` scroll a slide taller than the screen.
Links to web pages and files open as in the viewer. With `-f`, the slides
are re-rendered every time the file is saved.

```markdown
# Release 2.0

Highlights of the quarter

---

## New features

- Section jumps in the viewer
- Presentations
```

## 🎨 Themes

| Dark | Light |
|---|---|
| `tokyo-night` (default) | `tokyo-night-day` (default) |
| `catppuccin-mocha` | `catppuccin-latte` |
| `gruvbox-dark` | `gruvbox-light` |
| `solarized-dark` | `solarized-light` |
| `github-dark` | `github-light` |
| `dracula`, `nord`, `one-dark` | |

`kmd --list-themes` shows each theme with color swatches. With
`theme = auto`, kmd chooses the light or dark default based on `COLORFGBG`.

Each theme is defined by a 17-color palette from which every element
(headings, code, syntax, math, diagrams) is derived. To create a theme:

```sh
mkdir -p ~/.config/kmd/themes
kmd --theme-template nord > ~/.config/kmd/themes/mine.theme
kmd -t mine README.md
```

```ini
inherit  = nord         # start from an existing theme
blue     = #5e81ac      # change a palette color; every derived role follows
heading2 = #bf616a      # or override a single role
```

## ⚙️ Configuration

kmd reads `~/.config/kmd/config` (or `$XDG_CONFIG_HOME/kmd/config`, or the
path in `$KMD_CONFIG`). `kmd --config` prints the file in use, the effective
settings and a template.

```ini
theme       = auto
dark-theme  = catppuccin-mocha
light-theme = github-light
max-width   = 100
images      = auto
pager       = builtin   # or "less -R", or "-" to never page
mouse       = true      # click links and scroll with the wheel in the viewer
view        = auto      # "always": use the viewer even for short documents
```

Precedence: command-line flags, then environment variables (`KMD_THEME`,
`KMD_PAGER`, `NO_COLOR`), then the configuration file.

## 🔒 Security

Documents are treated as untrusted (READMEs from cloned repositories,
downloads, `curl | kmd`). The design was reviewed and fuzzed against that
threat model:

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
- **Viewer**: a linked document is only read when the user follows the link.
  Web and `mailto:` links, and local images, PDFs and other data files, are
  handed to `xdg-open`/`open` as a single argument (never through a shell);
  links to programs, scripts or executable files are refused. Symlinks are
  resolved and checked on the real file, which must be a regular file of the
  same kind (`notes.md → ~/.ssh/id_rsa` is refused); `mailto:` links keep
  only `subject` and `body`. OSC 8 targets become absolute `file://` URLs
  only for Markdown and document files, so a terminal click never runs a
  script or `.desktop` file. Local `.html` and `.svg` files are not handed to
  the browser (their scripts would run). A click on a link that leaves kmd
  (web, mail, desktop apps) first selects it and shows its target; a second
  click or Enter opens it.

Images cost what decoding costs: a large PNG adds a few milliseconds
(`-i none` disables images; kitty passes PNGs through without decoding).

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## 🤝 Contributing

Contributions are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) describes the
workflow, the local CI (`make ci`) and the source layout.

## 📄 License

MIT. The CommonMark/GFM specification examples in `internal/md/testdata`
(CC-BY-SA 4.0) are used by the tests only.
