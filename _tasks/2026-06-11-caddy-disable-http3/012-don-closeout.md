# 012 — Don's Close-Out Assessment: Disable Caddy HTTP/3

PLAN-step close-out after EXECUTION. I read the full task history (01 → 011) and then
verified the shipped branch against the real bytes — I did not take any report's word for
anything. Every claim below I checked myself on `task/caddy-disable-http3`.

## Verdict: FULLY DONE.

Don, Joel, and Linus all agree. Linus's high-level execution review (`011`) already
returned FULLY DONE / APPROVED; Kevlin's low-level pass (`010`) returned APPROVE; Joel's
plan was executed without drift. I independently confirm. Proceed to FINALIZATION (Ward
knowledge capture, then squash-merge).

---

## What I verified myself (proof, not trust)

**1. The user's actual request is satisfied — h3 off, h1+h2 only.**
`internal/caddy/generator.go:41-45` emits, before the per-service loop and before any
`len(inputs)` guard:
```
{
    servers {
        protocols h1 h2
    }
}
```
Omitting `h3` from `protocols` is the documented Caddy v2 mechanism to disable HTTP/3.
The block is the first block in the file (Caddy rejects global options after a site
block) and is emitted unconditionally, so the guarantee holds from the empty registry
onward — no "first deploy still advertises h3" window. With h3 disabled, Caddy never
sends `Alt-Svc: h3`, so iPhone Safari is never offered QUIC. That is exactly the request.

**2. Scope held where it had to.**
- `internal/caddy/stub.go` is `:80` plaintext (no TLS) — h3 is physically impossible
  there, correctly left untouched. Not a gap.
- `manager.go` `runOpts()` UDP/443 port maps left published-but-inert. Unpublishing
  requires a `caddy up` container recreate (bigger blast radius than the `caddy reload`
  this fix rides on) for zero functional benefit to the bug. Correctly deferred. Verified
  neither file is in `git diff --name-only main...HEAD`.

**3. Nothing left half-finished. Tests pass. Docs consistent.**
- `go test ./...` → every package `ok` across the whole repo, not just `internal/caddy`.
- Fresh `-count=1 -v` run of the generator suite: all five tests PASS, including the new
  `TestGenerator_DisablesHTTP3` and the renamed `TestGenerator_EmptyInputProducesHeaderAndGlobalBlock`.
- `gofmt -l ./internal/caddy/` → clean.
- The tests are contract tests, not change-detectors: the `protocols h1 h2\n` positive
  assertion pins the directive to terminate at `h2` (catches *any* trailing protocol, not
  just h3); the h3 negatives are scoped to the directive text so a `h3.example.com`
  hostname can't false-positive (the trap Linus flagged in `004`, closed by construction).
- `grep -rniE 'h3|http/3|http3|quic' _docs/ README.md` → every remaining hit is the
  corrected published-but-inert wording. The old `_docs/install.md:56` "open UDP/443 for
  mobile / my phone is slow" lie is gone. README hits are only the "Quick start" heading
  (no h3 claim); the port lists stay because the port is still published. Consistent.

**4. The decision record is honest and defuses the landmine.**
`_ai/decisions/caddy-runs-in-container.md` carries a dated amendment that PRESERVES the
original line-17 "h3 is a mobile benefit" reasoning, records that field experience
reversed it, and tells engineer #8 in plain words NOT to re-enable HTTP/3 to "fix a mobile
regression." This was the single most important non-code deliverable and it is done right.
The M3 `caddy.protocols` config idea is quarantined to the decision record — no invented
flag leaked into operator docs.

## The one thing that has NOT happened — and correctly so

No real `caddy validate` and no iPhone-Safari-over-QUIC test ran here: no Docker on this
box. The unit tests assert emitted bytes only — a proxy, not proof. The team did not
pretend otherwise: Rob, Raymond, Kevlin, and Linus all carry "byte-asserted; pending
operator `caddy validate`," and no doc claims the Caddyfile is "validated." The real
validate + iPhone check is the maintainer's manual deployment-time step on the Linux host.
That is **out of our control and expected**, not a code-review blocker and not silent
follow-up work — it is documented honestly in the reports.

## Remaining work

None inside this task's scope. No follow-up is silently required. (The one genuinely
deferred, separate change — unpublishing UDP/443 — is explicitly recorded in the decision
record as a future task with its own rationale, not an obligation of this fix.)

**FULLY DONE. Hand off to Ward for knowledge capture, then squash-merge into `main`.**

— Don
