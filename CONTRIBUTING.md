# Contributing to kmd

Thanks for your interest in kmd! This document describes how changes get
into the project. Following it keeps reviews fast and the tool fast, small
and safe.

## Ground rules

kmd has three non-negotiable properties. A change that breaks one of them
will not be merged, however useful it is:

1. **Fast** — a typical document renders end-to-end (process start to exit)
   in under 10 ms. `make perf` checks it.
2. **Dependency-free** — standard library only; `go.mod` has no `require`.
   Development tools (staticcheck, gosec, govulncheck) are pinned in the
   `Makefile` and never linked into the binary.
3. **Safe with untrusted documents** — a Markdown file must never be able
   to send control sequences to the terminal, read files other than images
   it references, contact the network, or make kmd use unbounded CPU or
   memory. See the Security section of the README.

## Workflow

The `main` branch is protected: every change goes through a pull request,
must pass CI and needs the maintainer's approval (see `.github/CODEOWNERS`).

1. **Open an issue first** for anything bigger than a small fix, so we can
   agree on the approach before you write code. Bugs: include the smallest
   Markdown that reproduces the problem and your terminal.
2. **Fork** the repository and create a branch from `main`:
   `git switch -c fix/table-alignment`.
3. **Make the change** with tests (see below).
4. **Run the local CI** — it is the same as GitHub's plus the perf gate:
   ```sh
   make ci        # fmt, vet, staticcheck, race tests, coverage, govulncheck,
                  # gosec, build, packaging metadata, perf
   make hooks     # optional: run `make ci` automatically before every push
   ```
5. **Open a pull request** against `main` and fill in the template. Keep PRs
   focused: one logical change per PR.
6. **Review**: CI must be green and the maintainer must approve. Address
   comments with new commits (no force-push during review, so the history
   of the discussion stays readable); the PR is squash-merged.

## Requirements for a change

- **Tests**: new behaviour comes with tests; bug fixes come with a
  regression test that fails without the fix. Total coverage must stay
  ≥ 90 % (`make cover`).
- **Conformance**: the parser must keep passing all 652 CommonMark 0.31.2
  and 24 GFM examples (`make spec`).
- **Untrusted input**: if the change touches parsing or rendering, think
  about hostile input. Add a pathological or fuzz case when relevant
  (`make fuzz`). Anything that grows with input needs a bound.
- **Performance**: if the change is on the hot path, include before/after
  numbers (`make bench`, `make perf`).
- **Documentation**: user-visible changes update `README.md`, the man page
  (`docs/kmd.1`), `kmd --help` and the shell completions
  (`internal/cli/commands.go`) as needed.
- **Style**: `gofmt -s`, clear names, comments that explain *why*. Code,
  comments, commit messages and documentation are written in English.

## Commit messages

Use the imperative mood and a short prefix naming the area:

```
md: handle tabs inside list item continuation
render: keep table borders aligned with wide characters
cli: add --no-pager to the config file
docs: document theme inheritance
```

Explain the reason for the change in the body when it is not obvious.

## Project layout

| Path               | What lives there                                     |
|--------------------|------------------------------------------------------|
| `internal/md`      | CommonMark/GFM parser and HTML exporter              |
| `internal/render`  | terminal layout, themes, syntax highlighting         |
| `internal/diagram` | Mermaid and Graphviz layout                          |
| `internal/tex`     | LaTeX → Unicode                                      |
| `internal/termimg` | image protocols (kitty, iTerm2, sixel, blocks)       |
| `internal/cli`     | flags, config, pager, `--follow`, `--html`           |
| `packaging/`       | Fedora RPM spec and AppStream metadata               |

## Reporting security issues

Do **not** open a public issue. Follow [SECURITY.md](SECURITY.md).

## License

By contributing you agree that your contribution is licensed under the
[MIT License](LICENSE) of this project.
