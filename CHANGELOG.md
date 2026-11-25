# Changelog

All notable changes to kmd are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Entries describe what changes for people using kmd or its `render` package;
see CONTRIBUTING.md ("Changelog") for how to add one.

## [Unreleased]

### Security

- Hostile inline text can no longer make the parser quadratic: nested
  `[^`, emphasis that closes out of order, `www.` autolinks with `_`,
  unclosed `![` before links, delimiter-like lines in long paragraphs and
  trailing `)` after autolinks now take linear time (minutes to hours at
  16 MiB before). Long links, and links made of zero-width characters, no
  longer slow rendering down either.
- A document makes at most 500,000 inline elements (text, emphasis, links,
  code spans, line breaks…); past that the rest is plain text and a notice
  is shown. Runs of `]`, `!`, `<`, `&` and escaped characters no longer
  cost hundreds of bytes each: 1 MiB of `]` took 460 MB of memory, now 20 MB.
- Hostile documents can no longer make kmd use gigabytes of memory or
  minutes of CPU when it lays them out: a top-level block lays out at most
  about a million styled pieces (64 MiB) and the rendered text stops at
  64 MiB per document, each with a notice; wrapped text no longer costs
  dozens of bytes of memory per word.
- Tables too wide for the screen no longer repeat long headers in every
  record: their labels are cut to 32 columns. Narrowing the columns of a
  very wide table, and rows with fewer cells than the header, no longer
  cost time or memory out of proportion to the table.
- Display formulas shown as source no longer take seconds when they hold
  thousands of combining marks.
- Clickable (OSC 8) links and images only lead to web pages, e-mail
  addresses (subject and body only) and Markdown or document files on this
  computer. Other schemes (`ssh:`, `smb:`, `vscode:`, `javascript:`…), paths
  to scripts or `.desktop` files, remote `file://` hosts and extensions
  hidden behind a NUL are no longer clickable in the terminal.
- The viewer's status bar always shows the whole host of a selected web
  link, and links with a user name
  (`https://github.com%2Flogin@evil.example/`) are refused.
- `render`: with `Hyperlinks` and no `BaseDir`, relative links no longer
  reveal the process's working directory.
- Images named several ways in one document (`a.png`, `./a.png`, symbolic or
  hard links to it) are read once, and a document reads at most 64 MiB of image files and
  `data:` URIs in all; decoded pixels are no longer kept after drawing.
- Image paths that name network or device locations (`\\host\share`,
  `//host`, `/\host`, `\\.\pipe\…`, also inside `file:` URLs or
  percent-encoded) are refused before any file access, so on Windows an
  image can no longer make kmd open an SMB connection.
- PNG images are checked chunk by chunk (checksums, nothing after the end)
  before kitty or iTerm2 receive them, and iTerm2 gets JPEG and GIF images
  re-encoded by kmd as PNG instead of the document's bytes.
- Zero-width spaces, word joiners, byte order marks, tag characters and
  other invisible format characters are stripped from rendered text and
  from code copied with `c`, so a command or URL can no longer look like
  another one.
- `--html` drops bidi overrides ("Trojan Source"), invisible format
  characters and carriage returns from text, code, titles and alt text, as
  the terminal output does (`--unsafe-html` keeps them).
- Theme file names can no longer run commands through the bash completion
  or send escape sequences to the terminal in `--list-themes`: only themes
  named with letters, digits, `.`, `-` and `_` are listed.
- Every error message is sanitized, including unknown flags and missing
  files whose names hold escape sequences.
- With several files, each file's heading shows its name as plain text: a
  name such as `www.example.md` is no longer a link.
- kmd no longer reads its config file (and the pager command in it) or
  themes from the current folder when `XDG_CONFIG_HOME` or `HOME` is a
  relative path such as an unexpanded `~/.config`: relative values are
  ignored, as the XDG specification asks, and a relative `KMD_CONFIG` is an
  error.
- Reloading a document (`-f`, `r` in the viewer) or opening one through a
  link or the file picker never blocks on a FIFO swapped in for it, and
  refuses a symbolic link swapped in to another kind of file; `q`, Ctrl-C
  and termination signals always end the viewer and restore the terminal,
  even while a document is loading.

## [0.17.0] - 2026-11-24

### Added

- `--html` and `--slides --html` highlight code blocks and draw mermaid and
  Graphviz diagrams as in the terminal, in the theme's colors, instead of
  showing their source.

### Fixed

- Printed HTML exports no longer show a scroll bar under code blocks, and
  tabs in code take four columns.

## [0.16.0] - 2026-11-24

### Added

- `--slides` shows a footer instead of the status bar, which no longer
  shows the step: the slide number and the `title` and `author` from the
  front matter. `footer:` in the front matter sets its text and
  `footer: false` (or `off`) hides it.
- `--notes` shows, in a second terminal, the speaker notes of the slide a
  `--slides` presentation of the same file has on screen, and the title of
  the next one, following it as it moves. Notes go in
  `<!-- notes: … -->` comments, which never show on the slide.
- `--slides` draws images with the terminal's graphics protocol (kitty,
  iTerm2, sixel) instead of Unicode blocks, at most as tall as the screen
  and 16 MiB of images per slide. Sixel needs a terminal that reports its
  cell size.
- `--slides --html` exports the deck as one HTML page, with the safe
  defaults of `--html`: 16:9 slides with their footer, one per page when
  printed, so the browser's "Print to PDF" makes a PDF deck.

### Fixed

- The viewer's bottom line no longer overflows the screen when it shows
  emoji followed by a variation selector (in a link target or a footer).

## [0.15.0] - 2026-11-24

### Added

- `--slides` reveals a slide in steps at every `<!-- pause -->` line:
  forward shows the next part, back hides it, and the status bar shows the
  step. A list without blank lines between its items carries on after a
  pause without a gap when it keeps the same marker, so its items can
  appear one at a time.

## [0.14.0] - 2026-11-23

### Added

- `theme = auto` (the default) now asks the terminal for its background
  color (OSC 11) when `COLORFGBG` does not name one, so light terminals that
  do not set it get the light theme. The terminal is only asked when colors
  are on and both standard input and output are the terminal; kmd gives up
  on one that does not answer after 0.1 s (1 s over SSH). Some terminals
  take a few milliseconds to answer (GNOME Terminal about 16 ms): set a
  theme to skip the detection. `kmd --config` shows what chose the theme.

## [0.13.0] - 2026-11-23

## [0.12.0] - 2026-11-23

### Added

- A file picker for folders: `kmd docs/` without a `README.md` lists every
  Markdown file under the folder, filtered as you type (fuzzy), and Enter
  opens it; Backspace returns to the list. `o` opens the picker from any
  document, for the folder kmd was started on.
- Search across documents: `S` (or `Tab` in the picker) searches the text
  of every document in the folder and lists the matches with their file,
  line and section; Enter opens the document at the match. The search runs
  in the background and Esc cancels it.
- `--base URL` resolves the relative links of a document read from standard
  input against the URL it was downloaded from
  (`curl -fsSL URL | kmd --base URL`), in the terminal, the viewer and
  `--html`. Nothing is downloaded: only inline `data:` images are drawn.

### Changed

- A folder without a `README.md` opens the built-in viewer (with the file
  picker) even when its listing fits on the screen; piped output, and an
  external pager set with `pager` or `KMD_PAGER`, are unchanged.

### Security

- File names with invalid UTF-8 in messages (a lone byte such as `0x9b`,
  an 8-bit control sequence for some terminals) are shown with U+FFFD
  instead of reaching the terminal as they are.

## [0.11.0] - 2026-11-23

### Added

- Syntax highlighting for 63 languages (about 25 before), including
  Haskell, Elixir, OCaml, F#, Clojure, R, Julia, Perl, PowerShell, Zig,
  Nix, Terraform, Protocol Buffers, GraphQL, Assembly, Batch, LaTeX, CMake
  and Nginx, with their usual fence names and file extensions (`tf`, `ex`,
  `hs`, `ps1`…). The full list is in the README and the man page.

## [0.10.1] - 2026-11-23

### Added

- Packages for Homebrew (`brew install DipaolaFederico/tap/kmd`), Scoop,
  winget and Debian/Ubuntu (`.deb` attached to each release, with the man
  page and shell completions), updated by every release.

### Changed

- Release tags point to a commit that carries the released version in the
  RPM spec, man page and AppStream metadata, so source archives and the Nix
  flake built from a tag report the right version.

## [0.10.0] - 2026-11-22

### Added

- Public Go package `github.com/DipaolaFederico/kmd/render` to render
  Markdown for the terminal or as HTML from other programs, with safe
  defaults (no file reads unless asked, images confined to `BaseDir`).

### Changed

- Building from source requires Go 1.25 or newer.

## [0.9.0] - 2026-11-22

### Added

- GitHub emoji shortcodes such as `:rocket:` (about 800 names).
- Obsidian-style wiki links: `[[Page]]`, `[[Page|label]]` and
  `[[Page#Heading]]` link to `Page.md` next to the document.

## [0.8.0] - 2026-11-22

### Added

- Mermaid Gantt, class and ER diagrams.
- Fedora packages through COPR (`dnf copr enable fdipa/kmd`).

## [0.7.0] - 2026-11-22

### Added

- `--slides` presents a document one slide at a time.

## [0.6.0] - 2026-11-22

### Added

- `c` in the viewer copies a code block to the clipboard (`wl-copy`,
  `xclip`, `xsel`, `pbcopy`, or OSC 52 over SSH).

## [0.5.0] - 2026-11-22

### Added

- Opening a folder renders its `README.md` (or `index.md`), or lists its
  subfolders and Markdown documents.
- Viewer table of contents (`t`) and section jumps (`[` and `]`).

## [0.4.0] - 2026-11-22

### Added

- `-s`/`--section NAME` renders only one section of a document.

### Fixed

- `kmd --version` reports the module version when installed with
  `go install` instead of `dev`.

## [0.3.0] - 2026-11-22

### Added

- Built-in viewer for long documents: links to other Markdown files open
  in place and can be navigated back.

## [0.2.0] - 2026-11-22

### Added

- First public release: CommonMark 0.31.2 (652/652 spec examples) and
  GitHub Flavored Markdown (tables, task lists, strikethrough, autolinks,
  footnotes, alerts, YAML front matter).
- Syntax highlighting for about 25 languages plus `diff`, OSC 8 hyperlinks,
  inline images (kitty, iTerm2, sixel, Unicode blocks), Mermaid (flowchart,
  state, sequence, pie) and Graphviz diagrams, and LaTeX math as Unicode.
- 13 bundled themes and custom palette-based themes; a configuration file.
- `--follow`, `--toc`, `--html` (safe by default), shell completions and a
  man page.

[Unreleased]: https://github.com/DipaolaFederico/kmd/compare/v0.17.0...HEAD
[0.17.0]: https://github.com/DipaolaFederico/kmd/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/DipaolaFederico/kmd/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/DipaolaFederico/kmd/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/DipaolaFederico/kmd/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/DipaolaFederico/kmd/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/DipaolaFederico/kmd/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/DipaolaFederico/kmd/compare/v0.10.1...v0.11.0
[0.10.1]: https://github.com/DipaolaFederico/kmd/compare/v0.10.0...v0.10.1
[0.10.0]: https://github.com/DipaolaFederico/kmd/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/DipaolaFederico/kmd/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/DipaolaFederico/kmd/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/DipaolaFederico/kmd/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/DipaolaFederico/kmd/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/DipaolaFederico/kmd/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/DipaolaFederico/kmd/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/DipaolaFederico/kmd/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/DipaolaFederico/kmd/releases/tag/v0.2.0
