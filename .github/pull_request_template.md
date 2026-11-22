## What and why

<!-- What does this change do, and why is it needed? Link the issue: "Fixes #123". -->

## How it was tested

<!-- Commands you ran, new tests, before/after output or timings. -->

## Checklist

- [ ] `make ci` passes locally (including the perf gate)
- [ ] New behaviour is covered by tests (coverage stays ≥ 90%)
- [ ] No new dependencies (`go.mod` has no `require`)
- [ ] User-facing changes are documented (`README.md`, `docs/kmd.1`, `kmd --help`)
- [ ] Untrusted input stays safe (no new way for a document to emit control sequences, read files or allocate without bounds)
