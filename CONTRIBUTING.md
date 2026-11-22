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

kmd uses **trunk-based development**: `main` is the only long-lived branch
and is always releasable. Work happens in short-lived branches (a few days at
most) that are squash-merged into `main` through a pull request; there are no
`develop` or release branches — releases are tags on `main`, made by CI (see
[Releases](#releases)). `main` is protected: every change needs a pull
request, green CI and the maintainer's approval (see `.github/CODEOWNERS`).

1. **Open an issue first** for anything bigger than a small fix, so we can
   agree on the approach before you write code. Bugs: include the smallest
   Markdown that reproduces the problem and your terminal.
2. **Fork** the repository and create a branch from an up-to-date `main`,
   named `type/short-description` (see [Branch names](#branch-names)):
   `git switch -c fix/table-alignment`.
3. **Make the change** with tests (see below). Keep the branch short-lived:
   rebase on `main` instead of letting it drift; split big work into several
   pull requests that each leave `main` working.
4. **Run the local CI** — it is the same as GitHub's plus the perf gate:
   ```sh
   make ci        # fmt, vet, staticcheck, release scripts, race tests,
                  # coverage, govulncheck, gosec, build, packaging, perf
   make hooks     # optional: run `make ci` automatically before every push
   ```
5. **Open a pull request** against `main`, titled as a Conventional Commit
   (see [Commit messages](#commit-messages)), and fill in the template. Keep
   PRs focused: one logical change per PR.
6. **Review**: CI and the *PR conventions* check must be green and the
   maintainer must approve. Address comments with new commits (no
   force-push during review, so the discussion stays readable); the PR is
   squash-merged and its title becomes the commit on `main`.

Changes that cannot affect kmd (README, this guide, templates, the release
workflows) skip lint, tests and build in CI automatically.

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

Commits follow [Conventional Commits](https://www.conventionalcommits.org):

```
type(scope): summary in the imperative, lowercase

Optional body: why the change is needed, when it is not obvious.

BREAKING CHANGE: what users must change (only for breaking changes).
```

| Type       | For                                                     | Release |
|------------|---------------------------------------------------------|---------|
| `feat`     | a new feature users can use (flag, config key, theme…)  | minor   |
| `fix`      | a bug fix in kmd                                        | patch   |
| `perf`     | a performance improvement                               | patch   |
| `revert`   | undoing an earlier commit                               | patch   |
| `refactor` | restructuring without changing behaviour                | —       |
| `test`     | tests only                                              | —       |
| `docs`     | documentation only                                      | —       |
| `build`    | Makefile, packaging, toolchain                          | —       |
| `ci`       | GitHub Actions and CI scripts                           | —       |
| `chore`    | anything else that is not shipped                       | —       |
| `style`    | formatting only                                         | —       |

A `!` after the type or scope (`feat(cli)!: remove --foo`), or a
`BREAKING CHANGE:` footer, marks a breaking change (major release). The scope
is optional and names the area: `md`, `render`, `view`, `cli`, `diagram`,
`tex`, `termimg`, `term`, `deps`…

```
fix(md): handle tabs inside list item continuation
feat(view): follow links between documents
perf(render): measure widths only up to the last link
docs: document theme inheritance
ci: run lint and tests only when the project changes
```

Since pull requests are squash-merged, **the PR title is the commit that
lands on `main`** and is what the *PR conventions* check validates; commits
inside the branch should follow the same format too.

## Branch names

`type/short-description`, lowercase, with the same types as commits:
`feat/link-navigation`, `fix/table-alignment`, `ci/release-pipeline`,
`docs/theme-guide`. (Dependabot's `dependabot/…` branches are accepted.)

## Releases

Releases are automatic and only happen when **users get something new**:
after a merge to `main` passes CI, a release is published if both

1. the commits since the last release include a `feat`, `fix`, `perf` or
   `revert` (or a breaking change), and
2. the compiled `kmd` binary differs from the last release's (built twice
   with identical settings and compared byte for byte).

Changes to tests, docs, CI, scripts or packaging, and refactors, never
publish a release on their own; they ship with the next one. The version is
the latest `vX.Y.Z` tag plus the largest increment among those commits
([Semantic Versioning](https://semver.org)):

| Commits since the last release            | Version   |
|-------------------------------------------|-----------|
| a breaking change (`type!:` / footer)     | `X+1.0.0` |
| a `feat`                                  | `X.Y+1.0` |
| only `fix`, `perf`, `revert`              | `X.Y.Z+1` |

Each release is a tag `vX.Y.Z` and a GitHub release with binaries for Linux,
macOS, FreeBSD and Windows, the source tarball, `SHA256SUMS` and a signed
build-provenance attestation; its notes list the merged pull requests.

- The *PR conventions* check labels each pull request with the release
  merging it asks for (`release:major`, `release:minor`, `release:patch` or
  `release:skip`). The labels are informational and follow the title: to
  change the release, change the title, not the label.
- Merges are never lost: if several land before a release runs (or CI fails
  on one of them), the next release covers all of them.
- Re-running a release that already succeeded does nothing.
- While kmd is `0.x`, breaking changes may also go out as `feat`; a breaking
  change from `0.x` publishes `1.0.0`.
- The maintainer can also release by hand (*Actions → Release → Run
  workflow*, `patch`, `minor` or `major`), for example after a Go security
  fix that changes no kmd code. Only from `main`; the workflow refuses any
  other user. Manual releases skip both conditions above.
- Verify a download with `sha256sum -c SHA256SUMS --ignore-missing` and its
  origin with `gh attestation verify <file> --repo DipaolaFederico/kmd`.
- The version is stamped into the RPM spec, the man page and the AppStream
  metadata of the published artifacts by `scripts/stamp-version.sh`; the
  copies in the repository are not rewritten (`main` only accepts reviewed
  pull requests, the release workflow cannot push to it).

### The public Go package

`render/` is the only public package; everything else is `internal/` and may
change at any time. From v1.0.0 on, `render` follows semantic versioning:

- Adding an exported identifier, a field to `Options`/`HTMLOptions`, or an
  `ImageMode` is a `feat`. Removing or changing one (a signature, a type, a
  constant's value, a field's meaning or a safe default) is a breaking
  change and needs a new major version, which for a Go module also means a
  `/v2` import path. Avoid it: add a new identifier and deprecate the old
  one instead.
- `render/testdata/api.txt` records the exported API and `make test` (or
  `make api`) compares it. Accept additions with `make api-update`; a
  reported removal or change means the pull request is breaking.
- The rendered output is not part of the API: layout and colour changes are
  `feat` or `fix` as for the command.
- A change only to `render/` that leaves the `kmd` binary byte-identical
  does not publish a release by itself (see condition 2 above); the
  maintainer releases it by hand when library users need it.
- The safe defaults are part of the contract: the zero `Options` must never
  read files, and images stay confined to `BaseDir`.

## Local CI

```sh
make ci      # every gate, in order; stops at the first failure
make hooks   # run `make ci` before every git push
```

| Stage       | Tool                                                 | Gate                                               |
|-------------|------------------------------------------------------|----------------------------------------------------|
| format      | `gofmt -s`                                           | no diff                         |
| lint        | `go vet` + `staticcheck`                             | no findings                     |
| test        | `go test -race` (includes CommonMark/GFM spec)       | all green                       |
| coverage    | `go test -coverprofile`                              | ≥ 90 % (`COVER_MIN`)            |
| security    | `govulncheck` + `gosec`                              | no issues                       |
| build       | `go build -trimpath`, `CGO_ENABLED=0`                | static binary                   |
| packaging   | `groff`, `appstreamcli --pedantic`, completions      | valid Fedora metadata           |
| performance | `scripts/perf.sh` (whole process, 200 runs)          | p50 and exec-adjusted p95 < 10 ms (`PERF_MAX_MS`) |

Extras: `make spec` (conformance), `make fuzz` (5 fuzzers), `make bench`,
`make rpm`.

Lint and security tools are installed at pinned versions into `.tools/` (the
first run needs network access). They are never linked into the binary.

## Project layout

| Path               | What lives there                                     |
|--------------------|------------------------------------------------------|
| `internal/md`      | CommonMark/GFM parser and HTML exporter              |
| `internal/render`  | terminal layout, themes, syntax highlighting         |
| `internal/diagram` | Mermaid and Graphviz layout                          |
| `internal/tex`     | LaTeX → Unicode                                      |
| `internal/termimg` | image protocols (kitty, iTerm2, sixel, blocks)       |
| `internal/styled`  | shared vocabulary: styled spans, Unicode width       |
| `internal/term`    | terminal size and raw mode via ioctl                 |
| `internal/view`    | interactive viewer: links, search, live reload       |
| `internal/cli`     | flags, config, pager, `--follow`, `--html`           |
| `render/`          | public Go package: the stable rendering API          |
| `internal/bridge`  | public options → internal ones, shared with the CLI  |
| `main.go`          | entry point: terminal and cell size, GC tuning       |
| `docs/`            | man page, packaging guide, README screenshot         |
| `scripts/`         | perf gate, release helpers and their tests           |
| `packaging/`       | Fedora RPM spec and AppStream metadata               |

## Reporting security issues

Do **not** open a public issue. Follow [SECURITY.md](SECURITY.md).

## License

By contributing you agree that your contribution is licensed under the
[MIT License](LICENSE) of this project.
