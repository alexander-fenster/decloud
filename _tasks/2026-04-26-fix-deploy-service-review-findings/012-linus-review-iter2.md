# Linus's iteration-2 plan review

**Verdict: APPROVE.**

One line. Right line. Right wording. Right reasoning. Ship it.

---

## What I verified

- `internal/cli/deploy_service.go:55` is in fact still
  `"container listen port (required if --host set)"` in the working tree.
  Joel's claim holds.
- `internal/cli/deploy_service.go:73-75` enforces `f.Port == 0` rejection
  unconditionally. The runtime contract is "always required."
- `internal/cli/deploy_service.go:53` (the `--name` flag) uses the
  `(required, ...)` pattern. Joel's consistency argument is real, not
  invented.

Three surfaces (runtime check, `_docs/usage.md`, `--help`) will tell the
operator the same story after this one line lands. That is the entire
point of the task.

## On Joel's wording defense

I floated `"container listen port"` (no parenthetical) as also acceptable
in `09-linus-review-impl.md`. Joel pushed back with the `--name` flag
consistency argument. He is right and I was being lazy. Mirror the
existing pattern. `(required)` it is.

## On Decision 2 (test rename) and Decision 3 (doc nit)

Don picked Option B (leave the test alone) and "ignore the doc nit."
Both match my recommendation in `09-linus-review-impl.md`. No further
comment.

## What I'm signing off on

The plan does exactly what I asked for and nothing else:

- One production line changes.
- Zero tests change (help string isn't asserted on; a test for it would
  be a change-detector test, which CLAUDE.md bans).
- Zero docs change (`_docs/usage.md` already carries the new contract).
- No scope creep. No "since we're here." No new abstractions.

After Rob's one-line diff and a clean `go test ./... -count=1`, this
task is **DONE**. I will sign off FULLY DONE at the next PLAN gate
provided:

1. Line 55 reads `"container listen port (required)"` exactly.
2. Tree-wide `go test ./... -count=1` is green.
3. `gofmt -l ./internal ./cmd` and `go vet ./...` are empty.
4. No other file changed.

If any of those four go sideways, surface it; do not paper over it.

---

## Verdict

**APPROVED. Execute.**

— Linus
