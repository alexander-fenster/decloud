# Linus — plan review (journald log driver)

Reviewing `02-plan.md` (Don) and `03-tech-plan.md` (Joel) at HEAD `23c7875`.

I went and checked the codebase myself rather than taking either of you at
your word. Bottom line up front: the plan is sound, the scope is right, and
most of the "decisions" Joel asked me to second-guess are noise. There is
ONE substantive issue worth pushing back on, and a couple of smaller items
I want logged before Kent starts typing.

---

## 1. Strategic decisions — verdict

### 1.1 Always-on, no opt-out flag — CORRECT

I verified this against `_docs/install.md`. The doc literally tells the
operator to run `systemctl enable --now docker` (line 22). Docker runs
under systemd. Journald is the host's log sink. There is no Decloud host
where journald is meaningfully absent — if dockerd is not under systemd,
the operator has not followed the install procedure, and the `docker run`
stderr will tell them so. A config knob here would be dead weight and a
future migration problem. Don is right; Joel agreed; I agree. No flag.

### 1.2 Tag format `decloud/<service>` — CORRECT

I checked: journald stores the tag byte-literal in `CONTAINER_TAG`,
`/` is not special, `journalctl CONTAINER_TAG=decloud/foo` matches exactly
without shell escaping in any common shell. The namespace prefix
`decloud/` also gives operators a clean cross-cut: `journalctl
CONTAINER_TAG=~^decloud/` (regex form) is the "all Decloud-managed
containers" query, which is exactly the kind of thing operators actually
type. The hyphen variant `decloud-<service>` would conflict cosmetically
with the container name (`decloud-foo`), which would confuse the next
person reading a journalctl invocation. Slash is the right choice.

One thing both of you handwaved: I went looking for whether rsyslog
forwarders or other log shippers care about `CONTAINER_TAG=` field
content. Answer: they don't. The standard journal-to-rsyslog imuxsock
path treats `SYSLOG_IDENTIFIER` (which docker-journald sets to the same
tag) as the program name, and `/` is permitted there too. No downstream
shipper this would break.

### 1.3 `Service` field on `RunRequest`/`RunOptions` — CORRECT shape

Joel asked me to evaluate "is there a better way (e.g. derive from
`Name`)?" There is not, and here's why his "kills a smell" argument is
real, not motivated reasoning:

The current code at `cli_driver.go:64` is:

    args = append(args, "--label", "decloud.service="+strings.TrimPrefix(req.Name, "decloud-"))

That is the EXACT same string-stripping derivation we would have to
repeat for the journald tag. So either:

- A. We string-strip twice (once for the label, once for the tag), or
- B. We string-strip once into a local, then use it twice, or
- C. We pass the bare service name explicitly and stop string-stripping.

(A) is duplicate ugliness. (B) is a band-aid — it still leaves the
caller knowing about the `decloud-` prefix shape and the driver inferring
the service name from a string trick. (C) is the only one that makes the
data flow honest: callers KNOW the service name (every call site has it
right there), so they should pass it. The container name (`decloud-foo`)
is presentation; the service name (`foo`) is identity; conflating them
inside the driver was always a smell, and Kevlin already flagged it
once.

Don's choice is correct. Joel's defense of it is correct. Ship as
specified.

### 1.4 Caddy gets `Service: "caddy"` — CORRECT

Verified the caddy call path. `decloud caddy down && up` does wipe logs
today. Tag `decloud/caddy` puts it in the same namespace as user
services without conflict (a user service named `caddy` would
collide on the container name `decloud-caddy` long before the tag
matters, per Joel §7). No issue.

### 1.5 No `decloud logs --history` UX change — CORRECT, scope discipline

Don explicitly defers the `decloud logs --history` wrapper. Joel
restates the defer. I agree. The point of THIS task is to fix the data
loss; the UX layer to surface cross-redeploy history can be a follow-up
once we know operators actually want it (versus just running
`journalctl CONTAINER_TAG=…` directly, which works fine and is what
sysadmins are going to type anyway).

---

## 2. The one substantive disagreement

### 2.1 The unenforced validation regex — Joel's defer is RIGHT but the
mitigation is THIN

You both noticed that the "service names are `[a-z][a-z0-9-]{0,38}`"
claim is a Cobra help-string lie — the regex is not enforced anywhere
in code. I verified: `internal/cli/deploy_service.go:57` is the only
place the regex appears as a string; `registry/store.go:206-226`
`validateForSave` only checks `Name != ""`. There is no actual
char-set enforcement.

Joel correctly chose not to widen scope to add a centralised validator,
and logged a follow-up. I AGREE with the defer; I want to flag one
thing both of you missed:

**Problem**: This task ships a NEW downstream consumer of the
unvalidated service-name string — the journald tag. That consumer joins
the existing ones (container name, label value, mount path components,
TOML filename). Each new consumer increases the cost of NOT having the
validator. Specifically: if a future caller ever passes a service name
with `/`, the tag becomes `decloud/foo/bar`, which journald accepts
literally but which OPERATORS will trip over when they write
`journalctl CONTAINER_TAG=decloud/foo` and get partial results
because there are also `decloud/foo/bar` entries.

**Impact**: Low TODAY (no caller can introduce a bad name today — the
only entry points are the CLI flag and TOML files nobody hand-edits).
Higher tomorrow, especially if someone adds a programmatic deploy API
in a future milestone.

**Options**:

- **Option A (Defer, as Joel proposed)**: Log the follow-up in
  `_ai/m1x-backlog.md`, ship the journald work as scoped. Pros: keeps
  this task one day. Cons: every new downstream consumer raises the
  blast radius slightly. Not a crisis.
- **Option B (Minimal hardening here)**: Add ONLY the most surgical
  check — at the driver level, in the `ErrEmptyService` guard,
  ALSO reject `Service` containing `/`. Two lines:
  ```go
  if req.Service == "" {
      return "", ErrEmptyService
  }
  if strings.ContainsRune(req.Service, '/') {
      return "", ErrInvalidService  // or fold into ErrEmptyService with a wrapped error
  }
  ```
  Pros: closes the specific failure mode this task introduces. Driver
  invariant becomes "the tag is unambiguous for journalctl". Cons:
  tiny scope creep; you also need a sentinel error and one test.
- **Option C (Full centralised validator)**: Add
  `ids.ValidateServiceName(string) error`, wire it into the CLI and
  registry loader. Pros: fixes the root cause. Cons: blows scope
  significantly; touches surfaces Don and Joel haven't inventoried.

**My take**: Option B. We're already adding a driver-level invariant
(`ErrEmptyService`); piggybacking a "no slashes" check on the same
guard costs nothing and surgically defends the new tag invariant
without widening scope to validation across the codebase. Option C is
correct in the long run but doesn't belong in this task.

**DON**: pick A or B. Either is defensible. If you pick A, the
follow-up in `_ai/m1x-backlog.md` MUST be specific enough that the
next maintainer reading the backlog understands the tag-ambiguity
failure mode I described above, not just "centralise the validator".

---

## 3. Joel's three "second-guess me" items — verdict

Joel explicitly asked me to actively second-guess three things (his §14):

### 3.1 Validation gap — covered above in §2

### 3.2 Literal-splice vs. append — JOEL IS RIGHT

Don wrote `append(args, "--log-driver", ...)` after the existing args
literal. Joel wants to splice the four tokens INTO the literal next to
`--name`/`--network`/`--restart`. Same byte-identical argv either way.
Joel's framing is correct: `--log-driver` is a fixed flag like
`--restart`, not a per-loop-iteration flag like `--env`. Reading-pattern
match says it belongs in the literal. Ship Joel's version.

### 3.3 `Service: name` vs. `Service: prev.Config.Name` in lifecycle.Start — JOEL IS RIGHT

Same value in practice (the store keys on filename = `name`). But:
the function already trusts `name` for the container-name derivation
two lines earlier (`containerName := ids.ContainerName(name)`).
Threading the same trust source through both derivations is cleaner
than mixing `name` and `prev.Config.Name`. Ship Joel's version.

These two (3.2, 3.3) are bikeshed-territory but Joel got both right.
Don, you can stop sweating them.

---

## 4. Joel's catches that Don missed — credit where due

### 4.1 `internal/integration/mount_test.go:69` — REAL CATCH

Don's §2.2 sweep enumerated four `RunRequest`/`RunOptions` callers.
There are FIVE — Joel found the integration test. Without this catch,
the integration test would compile fine and fail at runtime with
`ErrEmptyService` on the next Linux smoke, with no compile-time canary.
Required. Good catch.

### 4.2 The "validation regex is a help-string lie" catch — REAL CATCH

Don §1.2 leaned on the regex as if it were enforced. Joel checked and
it isn't. This led to the discussion in §2 above. Don, your plan
needs a small correction: the security/safety claim in your §1.2
("Service names are constrained to `[a-z][a-z0-9-]{0,38}`") is
descriptively wrong — they are documentationally suggested to be, not
constrained. Re-word in the next iteration if you keep that paragraph.

---

## 5. Test surface — anything missing?

Joel's §6 is thorough. I want to flag two small additions:

### 5.1 Test that the journald flags are NOT emitted on `docker start`

`docker start` (`cli_driver.go:91`) does not need new flags because
`HostConfig.LogConfig` is sealed at create time. Both of you correctly
state this; neither of you wrote a test that locks it. Add a one-line
assertion to the existing `TestCLIDriver_StartArgs` (at
`cli_driver_test.go:128`) or alongside it:

```go
assert.NotContains(t, records[0].Args, "--log-driver",
    "docker start must not re-emit journald flags; log config is sealed at create time")
```

Pros: catches the future drive-by "consistency" refactor that thinks
`docker start` should also know about logging. Cons: defensive against
something that hasn't broken. I'd still take it — the four-line cost is
low and the regression class is genuine.

**DON**: add this to Joel's §6 test list if you agree.

### 5.2 Caddy-tag-literal test — Joel marked it "optional, recommended"

Joel's §6.2.4 (`TestCLIDriver_RunWithOptionsEmitsJournaldFlagsWithCaddyTag`)
is the byte-level lock on `tag=decloud/caddy` for the `RunWithOptions`
path. Joel called it "optional, recommended." I'm calling it
**required**. The caddy tag is the only literal that differs between
the two paths; without this test, a future refactor that breaks the
caddy tag path specifically (e.g. by hardcoding `decloud/foo` somewhere
shared) would only be caught by integration smoke, not by `go test
./...`. Required. Promote it from "optional" to "must write."

### 5.3 Otherwise — Joel's coverage is good

The empty-Service rejection tests (§6.2.1, §6.2.2) with the
`assert.Empty(t, records)` guard-before-exec check are exactly the
right shape. The tag-literal lock (§6.2.3) is the right shape and
defends against `decloud-<service>`, `{{.Name}}`, and bare `<service>`
all at once. The deploy/lifecycle assertion extensions (§6.3, §6.4)
correctly pin the contract at the right layer. No padding, no theater.

---

## 6. Anything in scope that should be out (or vice versa)?

**In scope, correctly**: the journald flags, the `Service` field, the
empty-Service guard, the `TrimPrefix` removal, the caddy treatment,
the docs touches (usage.md, install.md, the `_ai/decisions/` entry),
the m1x-backlog entry, the integration test edit.

**Out of scope, correctly**: `decloud logs --history`, log rotation,
historical-log migration, centralised name validator (modulo §2.1
discussion), caddy-as-reserved-name blocklist.

**One borderline call**: the `strings.TrimPrefix(req.Name, "decloud-")`
removal. Don put this IN scope (§3.2). Joel agreed. I agree. It's
in-scope because this task is the natural moment to fix it — we're
adding the `Service` field explicitly precisely because the
TrimPrefix is the wrong shape, and leaving it in place would mean
two different paths derive the bare service name two different ways
in the same function. Pulling the change out would be silly.

No scope adjustments required.

---

## 7. Smaller items, not blocking

### 7.1 Don's plan §1.2 wording

Re-word "Service names are constrained to `[a-z][a-z0-9-]{0,38}` (cite,
cite)" to acknowledge it's the DOCUMENTED shape, not the CODE-enforced
shape. Otherwise a future reader uses it as load-bearing.

### 7.2 Don's plan §3.4 has a stale line reference

Don says `lifecycle.go:62-65` for the `exited` branch. I haven't
verified this drifted, but Joel's tech plan re-anchors everything at
HEAD `fb4d026`. If Kent finds the line moved between Joel's read and
the implementation commit, fix the cite per the file preamble in
`cli_driver_test.go:1-4`. This is hygiene, not a bug.

### 7.3 Joel's `ErrEmptyService` message wording

Joel made it wordier than Don's: `"dockerdrv: empty Service field
(programmer bug; populate at every Run/RunWithOptions call site)"`. I
have no strong opinion; it's an internal error nobody will ever see
unless they screw up a test fixture. Either ships. Don, pick.

---

## 8. The two unverified-on-this-box claims

I read code on macOS. The user memory note confirms there is no Docker
on this box. Two claims in the plan that cannot be unit-tested here:

1. **Journald accepts `decloud/<service>` as a tag and stores it
   verbatim.** Confirmed against docker docs and `systemd.journal-fields`
   manpage by both of you; documented as a manual Linux smoke
   (acceptance criterion 4); I trust the cross-reference. No code can
   check this.
2. **`docker logs` works against a journald-driven container the same
   as a json-file one.** Confirmed against docker docs (journald is
   listed as one of the dual-read drivers); documented as acceptance
   criterion 3; manual Linux smoke. No code can check this.

Both are fine to ship as documented manual smokes. The unit-test surface
locks the argv shape; only the live host can confirm the journald
behaviour. That asymmetry is correct.

---

## 9. Verdict

The plan is RIGHT. Scope is RIGHT. Test surface is mostly RIGHT.

There is one decision Don needs to make (§2.1, Option A vs B for the
slash-in-Service guard), and two test tweaks I want before execution
starts (§5.1, §5.2). All small. None of them block planning approval
in spirit — they're refinements, not redesigns. But because the workflow
requires explicit re-iteration on changes, I'm flagging NEEDS-CHANGES so
Don and Joel can fold these in before Kent writes tests.

### Items Don must address before Kent starts:

1. **§2.1 — Slash-in-Service guard.** Pick Option A (defer, with a
   sharper backlog entry) or Option B (add `ContainsRune(Service, '/')`
   check at the driver guard alongside the empty check). If Option A,
   the backlog entry MUST name the tag-ambiguity failure mode
   specifically, not generic "centralise validation."
2. **§5.1 — `docker start` negative test.** Add the `assert.NotContains`
   assertion locking that the journald flags do NOT re-appear on
   `docker start`. Joel's §6 list should include it.
3. **§5.2 — Promote Joel's "optional" caddy tag-literal test
   (§6.2.4) to required.**
4. **§7.1 — Don §1.2 wording.** Correct the "constrained to regex"
   claim to "documented as regex (not code-enforced today)" so the
   plan doesn't carry a false safety claim into execution.

### Items I'm endorsing without change:

- Always-on, no flag.
- Tag = `decloud/<service>`.
- `Service` field on both shapes, between `Name` and `Image`.
- `ErrEmptyService` returned, not panicked.
- Literal-splice (Joel §11.1).
- `Service: name` in lifecycle (Joel §11.6).
- Removing the `TrimPrefix(req.Name, "decloud-")` smell as part of this
  task.
- Caddy gets `Service: "caddy"`.
- Integration test edit (Joel §6.6).
- Deferring `decloud logs --history`.

---

## VERDICT: NEEDS-CHANGES

Address §2.1 (Don's decision), §5.1, §5.2, §7.1. Then re-iterate
planning per the workflow. None of the changes are large; this is one
more planning loop, not a redesign.

---

# REVISION 2 — re-review (commit `2ec8d41` tech plan, `e453e43` plan)

Re-read Don's updated plan and Joel's REVISION 2 tech plan end to end.
I'll go item-by-item against my four NEEDS-CHANGES asks from the
previous section, then take a hard look at the new things this iteration
introduced (which is where the danger usually hides).

## R2.1. Original ask coverage

### R2.1.1. §2.1 — slash-in-Service guard (Don's choice)

ASKED: pick Option A or B. If A, sharpen the backlog entry; if B,
add the guard alongside the empty check.

DELIVERED: Don picked Option B explicitly in `02-plan.md` §1.7
("Chose Option B over Option A"). His reasoning ("the invariant is
BORN in this task; the right place to defend it is this task")
matches the argument I made in §2.1. Driver guard shape is in §3.2.
Joel's tech plan threads the change through §1.4 (acceptance criterion
6 split), §3.3 (sentinel block), §5.1 (driver.go declarations), §5.2
(guard shape in `Run` and `RunWithOptions`), §6.2.3 and §6.2.4 (new
rejection tests), §10.9 (gotcha), §11.7 (rationale).

VERDICT: addressed correctly.

### R2.1.2. §5.1 — `docker start` negative assertion

ASKED: add `assert.NotContains(args, "--log-driver")` to
`TestCLIDriver_StartArgs` to lock the invariant that `docker start`
doesn't re-emit log flags.

DELIVERED: Joel §6.2.7 spells it out, against both `--log-driver`
AND `--log-opt`, with the failure-mode message ("HostConfig.LogConfig
is sealed at create time") in both `t` arguments. Acceptance criterion
7 in §1.4 is the spec-level statement. Don §4.1 names the test in his
test list.

VERDICT: addressed correctly. Two `NotContains` (one per flag) is
better than my one-line suggestion; the symmetric pair makes the
invariant story legible.

### R2.1.3. §5.2 — caddy tag-literal test promoted to required

ASKED: Joel's §6.2.4 (now §6.2.6) should be REQUIRED, not "optional,
recommended."

DELIVERED: Joel §6.2.6 is explicit ("now REQUIRED per Linus §5.2 /
Don §4.1"). Don §4.1 lists it in the "required new tests" set.
Section preamble at the top of tech plan documents the promotion.

VERDICT: addressed correctly.

### R2.1.4. §7.1 — Don §1.2 wording correction

ASKED: re-word "Service names are constrained to `[a-z][a-z0-9-]{0,38}`"
to acknowledge it's the documented shape, not code-enforced.

DELIVERED: Don §1.2 now reads "Service names are DOCUMENTED as
`[a-z][a-z0-9-]{0,38}` in a Cobra help string …, but Joel's review
caught that this regex is NOT enforced anywhere in code …". Explicit
acknowledgement. The wording transfer is clean.

VERDICT: addressed correctly.

## R2.2. New decisions this iteration introduced — second-guessing

The user prompt explicitly asked me to scrutinise four new things:
the two-sentinel split, the slash-check location, the new test
bodies, and whether the iteration introduced anything else worth
flagging.

### R2.2.1. Two sentinels (`ErrEmptyService` + `ErrInvalidService`) vs. one wrapped

Joel §3.3 and §11.7 argue for two distinct sentinels. The case rests
on three claims:

1. Tests can `errors.Is`-discriminate (and §6.2.1–§6.2.4 each carry
   an `assert.False(errors.Is(err, <other>))` to lock that).
2. Stack-trace legibility — the sentinel name names the failure mode.
3. Symmetry with `ErrContainerNotFound` / `ErrNoBridgeIP` — one
   sentinel per failure mode is house style.

Is this actually a good idea, or is it over-design?

Argument FOR keeping it as two: the alternative (`ErrInvalidService`
only, with a wrapped reason like `fmt.Errorf("%w: empty", …)` vs
`fmt.Errorf("%w: contains '/'", …)`) saves one identifier at the cost
of forcing every test to either (a) match on the wrapped string, which
is brittle, or (b) match only the outer sentinel, which can't
discriminate. The wrapped-reason approach is the wrong shape when the
two failure modes have different test surfaces (empty is "you forgot
the field"; slash is "the field is invalid upstream"). They are
genuinely different bugs.

Argument AGAINST: in the long-run, if/when the centralised
`ids.ValidateServiceName` validator lands (the §3.1 backlog), the
driver-level slash check becomes a defense-in-depth backstop that
should almost never fire in normal operation. Two sentinels for one
"should never happen" guard is a tiny amount of dead weight.

Verdict: keeping two is correct. The dead-weight is two lines and an
identifier. The optionality it preserves (callers being able to
discriminate, future hardening being able to add more
`ErrInvalidService` triggers without breaking the empty-Service
contract) is worth it. Joel's §11.7 rationale is the right one and
the symmetry argument with existing sentinels seals it. SHIP.

### R2.2.2. Should the slash check be in the driver at all, or upstream?

Joel §10.9 names this explicitly: "Anything broader would be scope
creep without an inventoried surface." The driver's invariant is
narrowly scoped — "the tag is unambiguous for `journalctl
CONTAINER_TAG=` prefix queries." Not "the service name is
syntactically valid."

This is the right place. The driver is the LAST line of defense
between in-process data and the on-host tag literal; the invariant
it defends is one the driver INTRODUCES. Pushing the check upstream
(CLI parser, registry loader, or both) would be correct in the
long run as part of the centralised-validator follow-up — but
upstream checks defending a downstream-introduced invariant is a
fragile contract. If the upstream check is bypassed (a new caller
that doesn't know about it), the bad tag reaches journald. With the
driver-level guard, that can't happen.

The two-layer-defense argument: when the centralised validator
lands, the driver guard becomes defense-in-depth that never fires
in practice — and that's exactly what defensive code should look like
("the cheap check at the boundary catches the bug you didn't know
existed yet"). Two lines and one test pair is a tiny cost for that.

Verdict: driver is the right home for this specific invariant. SHIP.

### R2.2.3. New test bodies — meaningful, or test-the-mock?

Walked §6.2.3 and §6.2.4 carefully.

The load-bearing assertions per test:

1. `require.Error(t, err)` — proves the function failed.
2. `assert.True(t, errors.Is(err, ErrInvalidService))` — proves the
   correct sentinel surfaces.
3. `assert.False(t, errors.Is(err, ErrEmptyService))` — proves the
   sentinels are distinct (the §3.3 / §11.7 promise).
4. `assert.Empty(t, records)` — proves the guard fires BEFORE
   `cmd.Run`.

Assertion (4) is the one that distinguishes a real test from
mock-theatre. If the guard order were wrong (slash check after
`cmd.Run`), `records` would have a `recordedCmd` entry and the test
would fail loudly. This is exactly the right assertion shape — same
pattern as the empty-Service tests, which I already endorsed in my
previous review.

Assertion (3) is the discrimination check that locks the
two-sentinel promise. If a future "cleanup" PR folds the sentinels
into one, this assertion fails. Good — that's the contract we want
locked.

These are NOT test-the-mock. The mock here is the
`recordingFactory`, and the assertion is that the factory was NEVER
invoked. That's a behavioural assertion, not a mock-shape assertion.
SHIP.

### R2.2.4. Cross-check: did the iteration leave any internal inconsistency?

I went looking for drift between Don's plan and Joel's tech plan,
since that's the usual failure mode after two rounds of folding-in.

- Don §1.7 says "Slash-in-`Service` guard is IN SCOPE — alongside
  the empty-`Service` guard, not deferred." Joel §5.2 implements it
  in that order (empty first, slash second). Match.
- Don §3.1 declares two sentinels (`ErrEmptyService` and
  `ErrInvalidService`). Joel §3.3 and §5.1 declare them in the same
  `var (…)` block. Match.
- Don §4.1 lists six new tests and one extension. Joel §6.2 enumerates
  §6.2.1 through §6.2.7 (= six tests + one extension). Match.
- Don §6 acceptance criterion 6 splits empty/slash. Joel §1.4 matches.
- Don §6 acceptance criterion 7 (StartArgs negative). Joel §1.4 #7
  matches.
- Don's §3.4 still cites `lifecycle.go:62-65` for the `exited` branch.
  My previous §7.2 flagged this as a "verify the cite hasn't drifted"
  hygiene item, not a blocker. Joel's §5.4 cites `lifecycle.go:67-75`
  (which is the absent branch, different code path). Both are
  citations into the same function; the drift is hygiene, not
  semantic. Rob can fix in implementation if the cite has moved by
  then. Not blocking.

No internal inconsistency that affects the implementation.

## R2.3. Was anything previously approved silently broken?

Joel's preamble lists exactly the sections that changed
(§1.4, §3.3, §5.1, §5.2, §6.2, §10.9, §11.1, §11.7, §13, §14).
I cross-checked the unchanged-by-claim sections (§2, §4, §7, §8, §9,
§10.1–§10.8, §11.2–§11.6, §12) and they read as before.
The "everything not listed in the preamble is unchanged from
REVISION 1" discipline is correct here.

One thing worth noting: Joel's §6.1.5 ("the helper-based ones")
requires `Service: "x"` to be added to every `RunOptions{}` fixture
in seven helper-based tests. This was already there in REVISION 1,
and §10.4 ("Test fixture proliferation") names it as a landmine.
Kent has a clear "run the fixture sweep, watch them fail with
`ErrEmptyService`, then add the field" recipe in §10.4. Good
operationalisation; nothing new for me to add.

## R2.4. Anything else worth flagging?

### R2.4.1. The two-distinct-sentinels-must-not-match assertions

Joel's §6.2.1 includes `assert.False(t, errors.Is(err, ErrInvalidService))`
in the empty-Service test. §6.2.3 includes the symmetric assertion.
This is belt-and-braces for a property that holds by construction
today (the sentinels are two distinct `errors.New` calls, so
`errors.Is` can't cross-match unless someone wraps them together).

Is this assertion meaningful, or paranoid?

It's defensive against a future refactor that folds the two
sentinels under a shared parent (`var ErrServiceInvariant = …; var
ErrEmptyService = fmt.Errorf("%w: empty", ErrServiceInvariant); …`).
Under that refactor, `errors.Is(err, ErrEmptyService)` against an
`ErrInvalidService`-wrapped value would still return false (different
leaves), but `errors.Is(err, ErrServiceInvariant)` would match both.
The discrimination assertion in §6.2.1–§6.2.4 lock the
two-leaves-no-shared-parent shape. If a future refactor introduces
a shared parent, the discrimination assertion still passes (the
leaves are distinct); if a future refactor accidentally folds the
two leaves into one, the discrimination assertion fails. That's
correct.

Joel: the assertion is right; it's not paranoia, it's a contract
lock. Don't drop it.

### R2.4.2. Order of guards in `Run` / `RunWithOptions`

Joel §5.2 explicitly orders empty-first, slash-second, with
rationale in the comment block ("empty first … is the cheaper one
and the … more common one in test-fixture proliferation"). I
have no quarrel with the order — it doesn't matter for behaviour
(both return before `cmd.Run`), but the rationale Joel gives is
the right one for "code as if the next reader is annoyed."

### R2.4.3. The "discrimination" assertion language in test names/messages

Test names are long (`TestCLIDriver_RunReturnsErrInvalidServiceWhenServiceContainsSlash`).
The messages on `assert.False` calls are wordy ("must NOT match
`ErrEmptyService` — the two sentinels are distinct"). I'd normally
push back on verbosity, but in this case the verbosity is naming
the invariant being defended. When a test fails at 11pm, the
verbose name + message tells the on-call what changed. Acceptable.

## R2.5. Are there any items I left out of the previous review I should
flag now?

I re-read my prior review. Three small items I noted as
"non-blocking" in the previous round are still non-blocking:

- §7.2: stale `lifecycle.go:62-65` line ref in Don's plan. Hygiene
  only; Rob fixes in implementation.
- §7.3: `ErrEmptyService` message wording. Joel picked the wordy
  version (§3.3, §11.2). Fine.
- The two unverified-on-this-box claims (§8 of my previous review):
  journald accepts the tag verbatim, and `docker logs` works against
  journald containers. Both are documented as manual Linux smoke;
  asymmetric test coverage between unit tests and manual smoke is
  correct for these claims. No change.

Nothing missed. Nothing dropped.

## R2.6. Verdict

The iteration delivered exactly what the previous review asked for,
threaded cleanly through Don's plan and Joel's tech plan with no
internal drift. The new decisions this iteration introduced (the
two-sentinel split, the test-discrimination assertions, the guard
order) are all defensible and Joel's rationales are the right ones.
The new test bodies are real behavioural tests, not mock theatre.

Don's plan §1.7 is the kind of in-task reasoning paragraph that ages
well — it states the decision, the alternative, the why, and points
forward to the exact sections that implement it. Joel's §11.7
mirrors that at the tech-plan layer. Future-me reading this in six
months will know what was decided and why without having to spelunk.

No remaining blockers. No remaining concerns worth iterating on
again — the things I'd want to tighten further (centralised
validator, full char-set guard) are correctly out of scope for this
task and correctly logged as follow-ups in `_ai/m1x-backlog.md`.

Kent: write the tests as Joel §6.2 specifies. Rob: implement as Joel
§5 specifies. Raymond: docs as Don §5 specifies. Workflow can move
to EXECUTION.

## VERDICT: APPROVED
