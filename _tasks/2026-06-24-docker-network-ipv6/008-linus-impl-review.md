# Linus — Implementation Review: Docker network IPv6 support

Verdict up front: **APPROVED.**

I reviewed the actual diff (`git diff main...HEAD`), not the reports' claims
about the diff — code, test, and docs. I also went back to my own plan review
(`004`) to check my two non-blocking notes were honored. They were. This is a
clean, minimal, complete implementation of exactly the approved plan. No drift,
no scope creep, no surprises.

---

## 1. Did the implementation match the approved plan? Yes — byte-for-byte where it mattered.

I approved (twice) a change that was supposed to be: one production line of
flags + a free `stderr=%q` completion, two literal swaps, one extended test,
docs. That is precisely what landed.

`cli_driver.go`:
- `decloudIPv6Subnet = "fd00:dec0:11d::/64"` unexported const in `dockerdrv`,
  placed immediately above `NetworkEnsure`. Correct package, correct visibility.
- The `inspect` early-return (the no-op-on-existing behavior the whole narrowed
  scope hinges on) is **untouched, byte-for-byte**. I checked the diff hunk
  directly — only the create call and its wrap changed; lines 176-179 are not in
  the diff at all. Exactly as promised.
- Create call now: `"network", "create", "--ipv6", "--subnet", decloudIPv6Subnet, name`.
  No `--driver` (default bridge preserved for the readiness probe). No IPv4
  `--subnet` (Docker auto-allocates v4, so `ContainerIP`'s v4 read is unchanged).
  Both were load-bearing decisions in the plan and both are honored.
- Joel's `stderr=%q` refinement landed correctly: `var stderr bytes.Buffer` +
  `cmd.Stderr = &stderr` + `stderr=%q` in the wrap, matching the file's house
  style. `bytes` was already imported (line 4) — no new import, as Rob verified.

`service.go`: the two literals at 159/163 → `caddy.NetworkName`. Value-identical,
cycle-free (`deploy` already imports `caddy`). Done.

The comments are the right kind — they explain WHY (ULA/NAT66, why no auto-
upgrade), not WHAT. No obvious-comment noise. Good.

## 2. Scope — nothing crept in, nothing important left out.

I specifically checked whether the literal cleanup metastasized. It did not.
`service.go` still has `"decloud"` literals at 254/324/387 — and that is
**correct**. I diffed against `main`: those pre-exist and they are `RunSpec.Network`
struct fields (the network a service *joins*, populated from registry config), a
different concern from the network being *ensured*. The plan explicitly scoped
the cleanup to "only the two literals on the touched path," and that is exactly
what was touched. No opportunistic NetworkName audit, no churn. This is the
discipline I asked for in `004` §4. Respected.

The `_tasks/` report files inflate the diff stat (~1000 lines) but that is
workflow bookkeeping, not code. The actual code/test surface is 32 changed lines
across three files. Right-sized.

## 3. My two non-blocking notes from `004` — both honored.

**Note 1: docs must NOT imply decloud auto-upgrades existing networks.** Honored,
and honored well. I read the actual doc diff:
- `install.md` §3.3 has an explicit paragraph: *"Decloud does **not** auto-upgrade
  an IPv4-only network to IPv6."* It states the gotcha (no docker command toggles
  `EnableIPv6`) and frames the `caddy down` → `docker network rm` → `caddy up`
  recipe unambiguously as a **manual, out-of-band operator action during a
  maintenance window**, with the active-endpoints caveat and an outage warning.
- `usage.md` step 0: *"an already-existing network is left untouched."*
- The troubleshooting entry tells the operator that a `false` reading means the
  network predates IPv6 and was *left untouched* — upgrade out-of-band.
Nowhere does the prose suggest decloud does the recreate itself. This is exactly
the honesty I wanted. Kevlin owns the line-by-line hallucination check on these
docs; from the architectural angle, the framing is correct.

**Note 2: do not pad the test surface with the optional create-fails test.**
Honored. Kent extended `WhenAbsent` only (assert `--ipv6` present; assert
`--subnet`'s immediately-following token equals the `decloudIPv6Subnet` const via
the existing `indexOf` helper), left `WhenPresent` untouched, and added NO
create-fails test. The `stderr=%q` path stays covered transitively by the
existing ImagePull-stderr pattern, as agreed. Two-test surface, right-sized for a
one-line change. No padding. Good.

The test reuses the established `indexOf(args, flag)` + `args[idx+1]` idiom and
asserts against the const (single source of truth — a subnet typo fails in
exactly one place). This is a real behavioral assertion (the fresh network
actually requests IPv6 with the ULA subnet), not a change-detector. Correct.

## 4. Anything operational that the plan/review missed, now that the code is real?

Nothing that blocks. I went looking for a bite and didn't find one:

- **Readiness probe** — `ContainerIP` reads the v4 field, untouched by `--ipv6`,
  v4 still auto-allocated. Verified again against the diff. Unaffected, as claimed
  across three reports.
- **The one host-level prerequisite the Go code genuinely cannot guarantee** —
  daemon `ip6tables`/NAT66 being on — is correctly NOT faked in code and IS
  documented as a host prerequisite in `install.md` §3.3 and the troubleshooting
  checklist. That is the right boundary: the code guarantees the one thing it can
  (the create flags); the daemon prereq is ops documentation. The network still
  *creates* IPv6-enabled even on a misconfigured daemon; only egress NAT is
  absent. Honest.
- **Integration/egress verification** — unverifiable on the Docker-less dev box;
  the maintainer runs `curl -6` from a container on the Linux host. That is the
  acceptance step that proves the actual goal. It is an honest boundary the unit
  tests cannot cross, not a gap. The scripted-factory test proves argv
  construction, which is all it can prove, and it does that correctly.

One thing I'll name so it is on the record (not a defect, not a required change):
the doc recipe says `docker network rm decloud` "fails if endpoints remain" and
tells the operator to stop services first. That is accurate. It is also exactly
the full-stack-outage dance I flagged in my *first* plan review (`004` §1) as the
reason inline auto-recreate was a footgun. The implementation correctly does NOT
do that automatically — it only documents it as a deliberate human action. That
is the resolution I wanted. Mentioning it only so nobody later mistakes the
documented manual recipe for an endorsement of wiring it into the hot path. Do
not.

## 5. Credit where due.

This is about as small and clean as a real fix gets, and every report's factual
claims checked out against the actual diff when I verified them — the early-return
really is untouched, `bytes` really was already imported, the other literals
really do pre-exist, build and gofmt are clean. The chain (Don narrowed, Joel
expanded, Kent pinned the contract, Rob implemented to it, Raymond documented the
no-auto-upgrade boundary plainly) held discipline and did not gold-plate a
one-line change. That is the correct outcome and it is rarer than it should be.

---

## Bottom line

**APPROVED.** No required changes.

The implementation matches the approved plan exactly, with zero drift and zero
scope creep. Both of my non-blocking notes from `004` were honored: the docs do
not imply auto-upgrade, and the test surface was not padded. Fresh installs get
IPv6 egress via the create flags; existing networks are a strict, untouched
no-op. Architecturally complete for the stated goal, right level, right package,
right scope.

The only verification this change cannot do itself — real NAT66 egress on a live
Linux+Docker host — is correctly delegated to the maintainer's integration step
and is the gate before this is considered field-proven.

— Linus

---

# Final re-review (after Kevlin's CHANGES REQUESTED) — FINAL SIGN-OFF

I was wrong in §2 above, and I'm correcting the record before signing off.

**What I got wrong.** In §2 I waved off the three `Network: "decloud"` literals at
`service.go:254/324/387` as "pre-existing, from registry config, a different
concern." I re-read the actual source this time instead of pattern-matching on the
field name. Ground truth:

- `:254` — hardcoded literal on a `RunRequest`/run struct field. Not config.
- `:324` — hardcoded literal inside the `registry.RunSpec{}` that is then
  `Store.Save`'d. This line literally **WRITES** the network value INTO the
  registry. It is the source of the persisted value, not a read of it. My "from
  registry config" claim was exactly backwards.
- `:387` — hardcoded literal in `restoreOldContainer`'s rollback `RunRequest`.

All three were the **same** magic-string concern as the 159/163 pair I praised
consolidating. There was no principled line separating "swap these two" from
"leave those three." Kevlin's CHANGES REQUESTED was correct and my §2 reasoning
was a false premise. Credit to Kevlin and to Don for catching it; I missed it.

**The resolution is correct and complete.** I re-read `git diff main...HEAD` for
`service.go`: all five sites (159, 163, 254, 324, 387) now use `caddy.NetworkName`.
`git grep '"decloud"' internal/deploy/service.go` returns nothing. `go build ./...`
clean, `gofmt -l` clean. Every swapped value was `"decloud"`, so the change is
strictly behavior-neutral — pure consolidation onto the single source of truth,
which is the whole correctness premise of this task ("both call sites must agree
on the network's identity"). Now ALL sites agree, and there is no remaining magic
string for the next person to forget when the network is renamed. This is the
state §2 *should* have demanded.

**On the `:387` redeploy-semantics question — Don's OUT OF SCOPE ruling is right,
and I agree.** `restoreOldContainer` restarts the *previous* container on
rollback, and one could argue it should reattach to `prev.Config.Run.Network`
(the network that container was actually deployed on) rather than the current
`caddy.NetworkName`. That is a real, interesting question — but it is **pre-
existing**: the line was a hardcoded `"decloud"` before this task and is a
hardcoded-equivalent `caddy.NetworkName` after. The swap does not introduce,
worsen, or touch that semantic question; it only removes a magic string. Whether
rollback should honor the previous deploy's recorded network is a separate
concern with its own correctness reasoning (and frankly, since the network name
is a single global constant today, it is moot until a multi-network world exists).
Folding it into an IPv6-egress + literal-cleanup task would be exactly the scope
creep I spend my reviews stamping out. Correct boundary. Parking it is the right
call; if anyone cares, it is a future task, not this one.

**Verdict: APPROVED / FULLY DONE.** The implementation is complete and correct for
the stated goal: fresh installs create the `decloud` network IPv6-enabled (ULA +
NAT66 egress), existing networks are an untouched no-op, the network identity is
consolidated onto `caddy.NetworkName` with zero remaining magic strings, docs are
honest about no-auto-upgrade, tests pin the create contract, build/gofmt/test all
green. No remaining blockers. The only thing left is the maintainer's real-host
`curl -6` egress check, which is an integration gate, not a code defect.

— Linus
