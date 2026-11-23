# Changelog

All notable changes to kmd are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Entries describe what changes for people using kmd or its `render` package;
see CONTRIBUTING.md ("Changelog") for how to add one.

## [Unreleased]

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

[Unreleased]: https://github.com/DipaolaFederico/kmd/compare/v0.13.0...HEAD
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
