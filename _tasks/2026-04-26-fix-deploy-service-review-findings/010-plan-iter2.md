# Don's iteration-2 plan: tuck the loose thread, ship

Both reviewers (Kevlin in `08-kevlin-review.md`, Linus in
`09-linus-review-impl.md`) returned the same single blocker, independently
verified. The architecture, layering, scope discipline, test design, and
doc coherence are right. One CLI/docs loose thread snuck in during the
fix for Findings 1-3. We tuck it in, re-run the tree, and ship.

I am not re-litigating anything that's already approved. Iteration 1 is
99% correct. This is iteration 2, and it is one line.

---

## Decisions

### Decision 1 — Blocking: fix the stale `--port` flag help (TAKE IT)

`internal/cli/deploy_service.go:55` currently reads:

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required if --host set)")
```

After Finding 3 landed, `--port=0` is rejected universally
(`deploy_service.go:73-75`). `_docs/usage.md` already says `Required: yes`.
The CLI's own `--help` still tells operators "(required if --host set)".

That is the **same class of bug** this whole task is fixing — CLI surface
contradicting documented contract. Shipping it would mean we partially
fixed "docs lie about flag contract" while introducing "help lies about
flag contract." That cannot ship. Both reviewers correctly blocked on
this.

**Fix (one line):**

```go
cmd.Flags().IntVar(&f.Port, "port", 0, "container listen port (required)")
```

This matches:
- the runtime check (`f.Port == 0` rejects unconditionally)
- the manual (`_docs/usage.md` `Required: yes`)
- the error string (`"--port is required"`)

Three surfaces, one story. Done.

### Decision 2 — Non-blocking: leave `TestDeployService_HostWithoutPortReturnsExitUsageError` as-is

Linus laid out three options (A: rename, B: leave, C: delete). He recommended
B; Kevlin agreed; I agree. The test still asserts a real-world misuse case
(operator passes `--host` but forgets `--port`), and the assertion
(`ExitCodeFor(err) == ExitUsageError`) is still correct under the new
generic-port-required path. The name is mildly inaccurate; that is harmless.

- **Renaming** costs reviewer cycles for zero behavioral gain.
- **Deleting** loses a real-world scenario test.
- **Leaving** preserves the scenario; future maintainers refactoring
  validation still trip the test if they break "host set but port missing."

**Decision: leave it.** No rename. No delete. Zero churn.

### Decision 3 — Non-blocking: ignore the `_ai/cobra-init-pattern.md` pseudo-Go nit

Kevlin's Issue 3 / Linus's Issue 3. Trivial doc nit (`Init(string) {}`
isn't valid Go syntax in a "do not collapse to" warning). Raymond can
fix when next touching the file. Not blocking. Not in this iteration.

---

## Execution

This is one production line. No tests to update; no docs to update; no
new packages touched.

### Step E1 — Rob applies the one-line fix

File: `internal/cli/deploy_service.go`, line 55.

Change `"container listen port (required if --host set)"` → `"container listen port (required)"`.

Nothing else in the file changes. Diff is exactly one line.

### Step E2 — Rob re-runs the verification tree

- `go test ./... -count=1` — must remain green tree-wide
- `gofmt -l internal cmd` — must remain empty
- `go vet ./...` — must remain empty

No test asserts on the help string, so test results should be identical
to iteration 1's run. If anything changes color, stop and surface it —
something else regressed.

### Step E3 — PLAN re-review (Don/Joel/Linus)

After Rob's one-line diff:
- Don confirms the fix matches this plan
- Joel confirms no tech-plan delta
- Linus signs off architecturally (he already pre-signed: "Fix that, then
  we're done")

If all three agree the task is FULLY DONE, advance to FINALIZATION
(Ward learnings, Andy agent-update consideration, then commit and push).

---

## What this plan does NOT do

- No code review of the iteration-1 diff. Both reviewers already did
  that work. Their findings stand; we are acting on them.
- No new tests. The help-text fix has no behavioral surface to test
  against — it's a string in a Cobra flag declaration. Test discipline
  per CLAUDE.md: do not write change-detector tests.
- No re-touching of Finding 1 / 2 / 3 production diffs. They are right.
- No re-touching of `_docs/usage.md` or `_ai/*.md`. They are right.
- No rename/delete of any existing test (Decision 2).
- No fix to the `_ai/cobra-init-pattern.md` pseudo-Go nit (Decision 3).
- No scope creep. Three findings stayed three findings; this iteration
  closes the cosmetic loose thread that snuck in during the fix.

---

## Acceptance criteria for iteration 2

1. `internal/cli/deploy_service.go:55` reads
   `"container listen port (required)"` (or whatever Rob applies that
   matches this plan exactly).
2. `go test ./... -count=1` is green tree-wide.
3. `gofmt -l internal cmd` is empty.
4. `go vet ./...` is empty.
5. Diff vs main: iteration-1 production/test/doc files unchanged from
   iteration 1, plus exactly one extra line in `deploy_service.go:55`.
6. Don/Joel/Linus all sign off on FULLY DONE.

---

## Why this is right, not just working

The Safari lesson applies. We didn't ship Safari with one animation
timing wrong because "the rest of the browser works fine." We fixed the
animation. Same shape here: iteration 1 is 99% right, and shipping the
last 1% wrong means a future operator running `decloud deploy service
--help` gets a different story than `_docs/usage.md` tells. That is
exactly the bug class we are fixing. We do not introduce a fresh
instance of the bug class while fixing it.

One line. Re-run the tree. Ship.

— Don
