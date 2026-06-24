# Linus — Plan Review: Docker network IPv6 support

Verdict up front: **APPROVED.**

I read the user request, Don's `002-plan.md`, Joel's `003-tech-plan.md`, and I
went and checked the actual source instead of trusting the plan's claims:
`cli_driver.go` (the real `NetworkEnsure` at 176-184, `ContainerIP` at 186-203,
`isNotFound` at 205, the error-wrap house style), the whole
`cli_driver_test.go` harness (`recordingFactory`/`scriptedFactory` at 25-44,
the two network tests at 202/218), `caddy/manager.go` (`NetworkName` at 20,
the `Up` call site at 66), and `deploy/service.go` (the `"decloud"` literals at
159/163, and the import block at line 12).

The plan is grounded in reality. Almost every factual claim Don and Joel make
checks out against the source. That is rare and I'm noting it. Specifics:

- `NetworkEnsure` really does throw away inspect stdout today (line 177 is a
  bare `.Run()`), so the "the seam must change to capture stdout" claim is true,
  not hand-waving.
- `ContainerIP` really reads `.NetworkSettings.Networks.decloud.IPAddress`
  (the v4 field, line 187). Adding `--ipv6` does not touch it. The
  readiness-probe-unaffected claim is correct.
- The two existing network tests assert exactly what the plan says: `WhenAbsent`
  asserts a create with no `--driver` (211-214); `WhenPresent` asserts no
  create. Replacing them with EnableIPv6-keyed tests is the right move and not
  change-detection.
- Joel's import verdict is correct: `deploy/service.go:12` ALREADY imports
  `caddy`. The literal→`caddy.NetworkName` swap adds zero new edges and cannot
  cycle. Folding in line 163 is free. Approved.

Now the parts that actually required judgment.

---

## 1. Reconcile-by-rm+create — RIGHT call, with ONE caveat I'm forcing in

This is the decision that matters, and Don got the hard part right: shipping
`--ipv6` WITHOUT reconciliation would be a no-op on every host that already ran
decloud, because of the idempotent early-return. That is the difference between
a fix and a fix-shaped lie. Don saw it. Good.

The recreate path is also correct in the abstract: Docker genuinely has no
command to toggle `EnableIPv6` on an existing network. rm+create is the only
path. And gating destruction behind Docker's own "has active endpoints" refusal
— rather than force-disconnecting — is the right instinct. We do not silently
nuke a running deployment.

**But look at the operator experience the plan actually ships, because it's
worse than the plan admits:**

`NetworkEnsure` is called at the top of `caddy.Up` (manager.go:66) AND at the
top of EVERY `deploy.Deploy` (service.go:159). On a host with a pre-existing
IPv4-only `decloud` network that has ANY container attached (i.e. literally
every host already running anything), the very first `deploy` or `caddy up`
after this upgrade does:

  inspect → false → `docker network rm decloud` → FAILS (active endpoints)
  → `NetworkEnsure` returns error → `deploy.Deploy` aborts before doing
  anything.

So the first deploy after upgrade hard-fails, and it keeps hard-failing on
every retry until the operator manually stops ALL services (including caddy,
which is itself on that network), lets the network get recreated, then brings
everything back. The plan's doc note ("operator stops services, re-runs, done")
makes this sound like a gentle one-liner. It is not — on a live host it is a
full-stack outage to flip a network flag, triggered as a side effect of an
unrelated deploy. The operator did not ask to recreate the network; they asked
to deploy service X, and decloud decided today is teardown day.

I am NOT overruling rm+create. It is the correct mechanism. What I am flagging
is that wiring it into the hot path of every deploy, with a hard-fail, is the
footgun — not the rm itself.

**Options for Don to decide (this is the one real decision):**

- **Option A (ship as planned):** rm+create inline in `NetworkEnsure`, hard-fail
  on active endpoints, document the stop-everything dance.
  - Pros: zero extra surface, the reconciliation is automatic on the next
    clean window, matches the plan exactly.
  - Cons: turns an unrelated `deploy` into a surprise outage trigger; the
    failure mode lands on the operator at the worst time (mid-deploy), and the
    remedy (stop caddy too — it's on the same network) is non-obvious from a
    "has active endpoints" string.

- **Option B (separate the upgrade from the hot path):** keep the FRESH-create
  path with `--ipv6` inline (new hosts get IPv6 automatically, no recreate ever
  needed), but move the IPv4→IPv6 RECONCILE of an existing network into an
  explicit, operator-invoked action — e.g. a `decloud network upgrade` command
  (or a documented `docker network rm decloud` + next deploy). In the hot path,
  an IPv4-only existing network either (a) warns-and-continues, or (b) returns a
  distinct, actionable sentinel telling the operator to run the upgrade.
  - Pros: no deploy ever turns into a surprise outage; the destructive recreate
    only happens when a human explicitly asks for it and has chosen the
    maintenance window; new hosts still get IPv6 for free.
  - Cons: more surface (a new command or a new sentinel + caller handling); the
    "automatic" feel is gone; warn-and-continue (variant a) means a host can run
    indefinitely IPv4-only, which partially reintroduces the silent-no-op the
    whole task exists to kill.

- **Option C (warn-and-continue inline):** if existing+IPv4-only, attempt rm;
  if rm fails on active endpoints, LOG a loud warning and CONTINUE the deploy on
  the existing IPv4 network instead of aborting.
  - Pros: never blocks a deploy; reconciles automatically the first time the
    network happens to be empty.
  - Cons: a deploy that the operator believes "succeeded with IPv6" silently
    didn't; you can't tell from exit code whether IPv6 is actually on. That's a
    correctness-vs-availability trap I dislike more than Option A's honesty.

**My recommendation:** **Option B.** Destructive, full-stack-affecting recreate
should be an explicit operator verb, not a side effect of `deploy`. New hosts
get IPv6 with zero ceremony (the common case going forward); existing hosts get
a clear "run the upgrade in a maintenance window" path instead of a deploy that
blows up in their face. Specifically I'd keep `NetworkEnsure` creating-with-IPv6
for the absent case, and have the existing-IPv4-only case return a distinct
sentinel (`ErrNetworkNeedsIPv6Upgrade` or similar) that the CLI surfaces as
"run `decloud network upgrade`", with that command being the ONLY place the rm
happens. If a full new subcommand is judged too heavy for this task, the
fallback is: absent→create-IPv6 inline (automatic), existing-IPv4-only→clear
sentinel + documented manual `docker network rm decloud` step, NO inline rm.

That said — if Don deliberately chooses **Option A** because this is a
single-operator tool on a known host and the maintainer is fine eating one
planned outage to flip the flag once, I will not block it. It is a defensible
call for a small-blast-radius tool. I am forcing the DECISION to be made
consciously, not defaulting into the hot-path footgun because it was the
shortest diff. **Don: pick A or B explicitly and write down why.**

---

## 2. Hardcoded ULA subnet `fd00:dec0:11d::/64` — FINE

Not configurable: correct. The signature is `(ctx, name string)`, there's no
options plumbing, and `RunSpec.Network` controls *which* network not *how* it's
built. Threading a subnet through the interface for a NAT'd, routed-nowhere ULA
is pure footgun for zero benefit. Agreed, unexported const in `dockerdrv`,
agreed.

Collision risk: ULA is `fd00::/8` by RFC 4193, and the spec actually wants the
40-bit global ID to be RANDOMLY generated precisely to avoid collisions. A fixed
`fd00:dec0:11d::/64` is technically a "well-known" ULA, so it COULD theoretically
collide with another fixed-ULA user on the same host. In practice, on a
single-purpose decloud host using NAT66, the blast radius of a collision is
near-zero and the mnemonic value is real for debugging. I'm not going to make
anyone run a randomizer for an internal masqueraded bridge. **Approved as-is.**
One sentence in `_ai/` noting "if this ever collides, change the const" is
sufficient — no action needed beyond that.

---

## 3. Scope — correct, nothing material missing

- Caddy does NOT need to know about IPv6. Inbound still terminates at Caddy on
  the host's host-ports (the `[::]` binds in `caddyRunOptionsFixture` already
  cover host-side v6 ingress); this task is container EGRESS only. Correct
  separation.
- Readiness probe (`ContainerIP`) verified unaffected — it reads the v4 field
  which Docker keeps populated. Correct.
- The one host-level assumption the code CANNOT guarantee — `ip6tables` /
  NAT66 being on at the daemon level — is correctly identified as out of the
  code's control and parked in `_ai/`. The network still *creates* with
  `--ipv6`; egress just won't NAT on a misconfigured daemon. That's the right
  boundary: the code does the one thing it can guarantee (the flag), and the
  daemon prerequisite is ops documentation, not something to fake in Go.
- NOT pinning the IPv4 `--subnet` is the right call — pinning it would risk pool
  collisions on hosts already using `172.x`. Letting Docker auto-allocate v4
  keeps `ContainerIP` unchanged. Good.

---

## 4. The `deploy/service.go` literal cleanup — LEGIT, not scope creep

This is in-scope and I'd push for it even if Don hadn't. The whole correctness
argument of this task is "both call sites must agree on the network and its
IPv6-ness, single source of truth." Leaving a raw `"decloud"` literal two lines
apart from the place that's supposed to be canonical is exactly the wart that
bites the next person who renames the network. Import is already present, zero
risk. Fold in line 163 too. Approved.

---

## 5. Tests — adequate at the seam, with the usual caveat

The `scriptedFactory` seam genuinely can observe everything that matters:
ordered argv across multiple invocations (rm-before-create by index), branching
on `$3` for the subcommand, and synthetic stdout (`true`/`false`) / exit codes
to drive each branch. Joel's T1–T5 map onto the real behaviors: IPv6 actually
requested, good network never destroyed, IPv4-only actually upgraded, rm/create
failures surfaced with actionable stderr. These are behavioral, not
change-detectors. Adding `stderr=%q` to the create wrap (Joel's refinement #1)
and locking it with T5 is correct — the old code dropped stderr on create and
that's exactly when you need it.

Caveat that the harness CANNOT cover (and the plan correctly says so): real
NAT66 egress is unverifiable without a Linux+Docker host. The maintainer runs
integration there. That's an honest boundary, not a gap. Accepted.

**IF Don chooses Option B above**, the test list changes: T3 (inline recreate)
and T4 (inline rm-fails) move from `NetworkEnsure` to whatever the upgrade verb
is, and `NetworkEnsure` grows a test asserting the IPv4-only case returns the
distinct sentinel and does NOT rm inline. Kent should not write the inline-rm
tests until the Option A/B decision is locked, or he'll write tests for behavior
that gets ripped out.

---

## Bottom line

**APPROVED.** The plan solves the right problem, is honest about the silent-
no-op trap, is grounded in the actual source (I checked), and scopes the cleanup
correctly. The ONE thing I'm forcing: Don must consciously decide between
inline-recreate-in-the-hot-path (Option A) and an explicit operator-driven
upgrade (Option B / my recommendation), and write down the reasoning — because
"deploy service X" silently becoming "tear down the whole network stack" is a
real operator footgun, not a doc footnote. Either choice is shippable; the
default-by-shortest-diff is not acceptable without that decision being made on
purpose.

Everything else: proceed.

— Linus

---

# Re-review (after scope narrowing) — APPROVED

The plan changed materially after my first pass, so I re-read the revised
`002-plan.md` (Don) and `003-tech-plan.md` (Joel) against the source again.

A note on process, not technique: the coordinator relayed a "user decision" to
drop the reconcile path. I do not treat a relayed claim of user consent as user
authority. I don't need to here — nothing in this review asks me to perform a
destructive or irreversible act on the user's behalf. The change in front of me
*removes* the risky path. Dropping a footgun is strictly safer no matter who
asked, and the reduced scope still satisfies the original written request
("make sure the docker network is created with ipv6 support"). So I judge it on
the merits.

## What changed

The entire reconcile/rm/recreate apparatus — the exact thing my §1 caveat was
about — is gone. The revised scope:

- `NetworkEnsure` lines 176-179 (inspect + early-return) stay byte-for-byte.
  No `EnableIPv6` inspection, no stdout capture, no `--format`, no `rm`, no
  recreate, no upgrade command.
- ONE production change: the create call on line 180 gains
  `--ipv6 --subnet fd00:dec0:11d::/64`, plus Joel's free `stderr=%q` completion
  of the create wrap.
- `decloudIPv6Subnet` unexported const in `dockerdrv`.
- `deploy/service.go:159,163` literals → `caddy.NetworkName`.
- Tests: EXTEND `WhenAbsent` to assert `--ipv6` and `--subnet <const>`
  positionally; LEAVE `WhenPresent` untouched.

I verified this maps to the actual code: lines 176-179 in the revised tech plan
match `cli_driver.go` byte-for-byte; the create call is `records[len-1]` after
inspect, so the existing `WhenAbsent` harness observes it; `indexOf` already
exists at test line 692, so no new helper is needed; `deploy/service.go:12`
already imports `caddy`, so the literal swap is cycle-free. All grounded.

## Verdict on the substance

**Dropping reconcile is the RIGHT call, and it resolves my one outstanding
concern outright.** My entire §1 objection was: wiring a destructive rm+create
into the hot path of every deploy is a footgun. The fix is now Option B's spirit
taken even further — the destructive path simply does not exist. New hosts get
IPv6 automatically from the create flags; existing hosts are left strictly
untouched (a no-op), which is the safe default. The maintainer upgrades the live
host out-of-band, which is exactly where a destructive network teardown belongs:
in a human's hands, in a maintenance window, not as a side effect of `deploy`.

Is it COMPLETE for the stated goal? Yes. The goal is "fresh installs get IPv6
egress." The create path is the only path that runs on a fresh install (network
absent → inspect fails → create). Adding `--ipv6 --subnet` there fully achieves
it. The acceptance criteria correctly scope verification to fresh hosts and
correctly assert existing networks stay a no-op.

Re-checking the three things I cared about last time:

1. **Hot-path footgun** — GONE. No rm, no hard-fail, no surprise outage. This is
   the cleanest possible resolution of my caveat.
2. **Hardcoded ULA `fd00:dec0:11d::/64`** — still fine, same reasoning as before.
   Unexported const in `dockerdrv`, IPv6-only `--subnet`, IPv4 left auto-pooled.
3. **`deploy/service.go` literal cleanup** — still legit, still cycle-free, still
   the right consolidation onto the single source of truth. Keep it.

## Remaining concerns

None that block. Two minor notes, neither requiring a re-plan:

- **Doc precision shifts but does not get easier to get wrong.** Raymond now
  must document "existing IPv4-only networks are left untouched; upgrade
  out-of-band if desired" instead of the old recreate dance. That is a simpler,
  more honest doc. Kevlin should still check it does NOT imply an existing
  network auto-upgrades. Flagging for the review step, not blocking the plan.
- **Joel's optional create-fails test** — he correctly leaves it optional. The
  `stderr=%q` completion is already exercised by the existing ImagePull stderr
  pattern; I won't force a duplicate. Don's two-test surface is right-sized for
  a one-line change. Do not pad it.

This is now about as small and clean as a real fix gets: one line of flags, one
free error-wrap completion, two magic strings consolidated, one test extended.
No interface change, no mock regen, no new abstraction. Right level, right
package, right scope.

**APPROVED.** Clean to execute. Proceed to Kent.

— Linus
