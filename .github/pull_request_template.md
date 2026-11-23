## What and why

<!-- What does this change do, and why is it needed? Link the issue: "Fixes #123". -->

## How it was tested

<!-- Commands you ran, new tests, before/after output or timings. -->

## Checklist

- [ ] Title is a Conventional Commit (`feat(scope): …`, `fix: …`, `docs: …`) and the branch is `type/short-description` — see CONTRIBUTING.md
- [ ] `make ci` passes locally (including the perf gate)
- [ ] New behaviour is covered by tests (coverage stays ≥ 90%)
- [ ] No new dependencies (`go.mod` has no `require`)
- [ ] User-facing changes are documented (`README.md`, `docs/kmd.1`, `kmd --help`)
- [ ] `feat`/`fix`/`perf`/`revert`: an entry under `## [Unreleased]` in `CHANGELOG.md` (or the `changelog:skip` label if users would not notice)
- [ ] Untrusted input stays safe (no new way for a document to emit control sequences, read files or allocate without bounds)
