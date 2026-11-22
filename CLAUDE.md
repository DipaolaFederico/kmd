# kmd — instructions for Claude

Read CONTRIBUTING.md first: kmd must stay fast (< 10 ms end to end),
dependency-free (standard library only) and safe with untrusted documents.

## Before calling any change finished

1. `make ci` passes (format, lint, race tests, coverage ≥ 90 %, govulncheck,
   gosec, packaging, perf).
2. Run **two independent reviews in separate subagents, in parallel**, on the
   diff against `main` (`git diff main...` plus untracked files):
   - **Security review** — against the threat model in CONTRIBUTING.md and
     the README's Security section: terminal escape injection, files read or
     programs launched because of document content, unbounded CPU/memory on
     hostile input, unsafe URLs, external commands.
   - **Performance review** — hot-path cost (rendering a typical document),
     allocations, anything that grows with input without a bound; back
     claims with `make bench` / `make perf` numbers.

   Give each reviewer the diff and the constraints, not your conclusions.
3. Verify each finding, fix the real ones, re-run `make ci`, and report what
   was found and what was fixed or dismissed (with the reason).

User-visible changes also update README.md, `docs/kmd.1`, `kmd --help` and
the shell completions (`internal/cli/commands.go`).
