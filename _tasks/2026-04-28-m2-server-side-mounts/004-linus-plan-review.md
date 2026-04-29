# 004 — Linus's plan review (PLAN stage)

Reviewing: `002-don-plan.md` and `003-joel-tech-plan.md`. Branch `feat/m2-server-side-mounts`. M1 shipped. M2 = `--mount` server-side end-to-end.

## TL;DR

This is a good plan. Don traced everything before opening his mouth (10 explicit traces with file:line citations) and Joel did exactly the job he's supposed to do — locked the 12 open decisions with rationale, supplied byte-level wording, and pushed back on Don in two places (Decisions 1 and 4) with arguments that hold up. The atomic-commit story is right. The phantom kill is right. The schema-stability claim is right. The semantic-token-test deletion is right.

I have **two concrete blockers**, **three issues that need a one-line clarification before Kent starts**, and **a handful of nitpicks**. None of these merit a v2 of the tech plan; they're the kind of thing Joel can answer inline in this review file or via a small addendum. Decision below: **APPROVED WITH MINOR REVISIONS**.

The detailed answers to the user's specific 10 questions are interleaved in the issues below where they bite, with explicit pointers.

---

## IDENTIFIED ISSUES

### Issue 1 — `parseMountFlags` runs `ValidateMounts` with `errUsage` wrapping but the inner sentinel is `ErrInvalidMount`

**Problem.** Joel §3.5(d):

```go
if err := registry.ValidateMounts(out, "<command-line>", "<command-line>"); err != nil {
    return nil, fmt.Errorf("--mount: %s: %w", err.Error(), errUsage)
}
```

`ValidateMounts` already wraps with `%w: ... ErrInvalidMount`. The CLI then re-wraps with `errUsage` using `%s: %w`. The chain is now:

```
"--mount: registry: invalid mount: service \"<command-line>\" mount[1] in <command-line>: duplicate container_path \"/x\" (also at mount[0]): usage error"
```

`errors.Is(err, errUsage)` → true (correct, exit 2).
`errors.Is(err, registry.ErrInvalidMount)` → **also true** because the `:%s:` flattens the message but the deeper `fmt.Errorf("%w: ...", ErrInvalidMount, ...)` from `ValidateMounts` is preserved as the `Unwrap()` chain.

Now look at `exit_codes.go`. The case order is:
1. `errors.Is(err, deploy.ErrInterrupted)` →130
2. `errors.Is(err, errUsage)` → 2
3. `errors.Is(err, registry.ErrInvalidMount)` → 10  *(per Joel §3.6)*

Go switch is first-match-wins, so `errUsage` wins → exit 2. Joel's intended behaviour. **Works.** But it works *by accident of case ordering*, which means if anyone ever reorders the cases (alphabetises them, groups by package, etc.), the exit code silently flips from 2 to 10 for the duplicate-target case. That's a latent footgun.

**Impact.** Subtle. Tests will catch it (`TestDeployService_MountFlagInvalidReturnsExitUsageError` will fail), but the failure mode reads "I reordered some cases for cleanliness, suddenly an unrelated test broke." Brittle.

**Options.**
- **Option A (minimal):** add a code comment in `exit_codes.go` above the `errUsage` case noting "this MUST come before `ErrInvalidMount` because `parseMountFlags` wraps both for the CLI duplicate-target path." Pros: documents the constraint. Cons: comments lie eventually.
- **Option B (proper):** in `parseMountFlags`, don't pass through `ValidateMounts`'s wrapped error — extract the underlying `reason` string only:

  ```go
  if err := registry.ValidateMounts(out, "<command-line>", "<command-line>"); err != nil {
      // Strip the ErrInvalidMount wrapping so this error chain only carries
      // errUsage; CLI exit code must be 2 not 10 regardless of case order.
      return nil, fmt.Errorf("--mount: %s: %w", reasonOnly(err), errUsage)
  }
  ```

  where `reasonOnly` unwraps once past `ErrInvalidMount` and returns the inner message. Pros: case-order-independent. Cons: small helper.
- **Option C (cleaner):** make `ValidateMounts` accept a "wrapping sentinel" parameter, or split into `validateMountsCore` (returns bare error) and `ValidateMounts` (wraps with `ErrInvalidMount`). The CLI calls the core; the loader calls the wrapper. Pros: each surface produces exactly one sentinel. Cons: API noise for one sentinel.

**My take.** Option B. One small helper, robust against future case reordering. The "duplicate-target reachable from CLI" path is the only place the chain carries both sentinels — Joel's plan calls this out in §3.5(d) ("The duplicate-target case is the one ParseMountString cannot detect") and that's the only crack in the otherwise-clean dispatch.

**DON.** Pick Option A or B. If A, I want the comment to literally name the failure mode ("DO NOT reorder; `parseMountFlags` produces a chain that `errors.Is`-matches both sentinels"). C is over-engineered for one sentinel.

---

### Issue 2 — Integration test as **`Driver.Run` direct call** is OK but make sure it actually exercises the M2 code path Rob is shipping

**Problem.** Joel's §4.8 reversed course mid-plan. Don §8 sketched a `decloud deploy service --mount ...` end-to-end test, but Joel concluded the deploy orchestrator is too noisy (build, readiness, Caddy reload, etc.) so the test should call `dockerdrv.NewCLIDriver().Run(ctx, RunRequest{Volumes: [...]})` directly.

This is *defensible* — the question "does real Docker accept our argv with `-v` flags?" is exactly what `Driver.Run` exercises and is exactly the unit-test gap m1x-backlog item 6 named. **However:** it side-steps the entire M2 surface above the driver. The test passes if `cliDriver.Run` plumbs `Volumes` into argv correctly. It says nothing about:

- `parseMountFlags` correctly parsing operator-typed strings
- `toVolumeMounts` correctly converting `registry.Mount` → `dockerdrv.VolumeMount` (the `IsNamed` derivation)
- `service.go:243-251` populating `runReq.Volumes` from `req.Mounts`
- TOML round-trip preserving the populated mounts
- `restoreOldContainer` carrying mounts forward

The argv-shape unit test (`TestCLIDriver_RunPassesVolumeFlags`, §4.5) already locks the argv byte-for-byte. The integration test as Joel rewrote it is **redundant with the unit test plus a real `docker run` invocation.** Real-Docker invocation IS valuable (bind-mount-exists semantics, named-volume-creation semantics), but the test is now narrower than what m1x-backlog item 6 asked for.

The user's question 4 in the review brief: "are we shipping mounts to prod without verifying the full ingress path still works?" Answer: **yes, this plan does ship M2 without ingress verification.** That's intentional per Decision 8 (split into m1x item 10). I think that's fine — ingress-through-Caddy is its own bug class — but I'm flagging it explicitly so we're not surprised.

**Impact.** If `parseMountFlags` is buggy (e.g., `StringArrayVar` quirk on some pflag version) or `toVolumeMounts` is buggy (e.g., the `IsNamed` derivation doesn't survive TOML round-trip in a way the unit-test fixture missed), the M2 ship breaks for an operator on first deploy. The integration test won't catch it because it bypasses both.

**Options.**
- **Option A:** keep Joel's narrowed test. Argv-shape unit-test + real `docker run` is enough. Pros: simple. Cons: doesn't exercise the full plumbing.
- **Option B:** restore Don's original sketch — `decloud deploy service --mount=...` against real Docker, with a hand-rolled tiny image that passes readiness. The deploy orchestrator's plumbing gets exercised end-to-end. Pros: full coverage. Cons: image build, readiness wait, Caddy reload — more moving parts, more reasons the test fails for non-mount reasons.
- **Option C:** keep Joel's narrowed test BUT add a second integration test that exercises *just* the registry round-trip + CLI parse + service.go population, with a mocked driver. Wait — that's a unit test. The M2 unit-test surface in §4.6 already covers it. So Option C is "trust the unit tests." Pros: zero new code. Cons: re-states A.
- **Option D:** narrow it the way Joel did, but ALSO add a one-line "validate the CLI parser end-to-end" smoke step in the same integration test: shell out to `decloud deploy service --help` and assert the help string parses (no crash, `--mount` flag visible). Catches Cobra registration regressions. Pros: cheap. Cons: barely additive.

**My take.** Option A is fine **provided** the unit-test surface §4.6 actually does what Joel says it does — specifically `TestDeploy_DeployWithMountsPassesVolumesToDriver` matcher checks the converted `Volumes` slice byte-for-byte. If that passes and `TestCLIDriver_RunPassesVolumeFlags` passes and the integration test shows `docker run -v ... cat` works, the chain is locked. Joel's narrowing is reasonable.

**DON.** Confirm you're OK with M2 shipping with **no end-to-end smoke that exercises CLI + loader + driver in one process against real Docker.** If not, we expand to Option B and accept the larger test footprint. My lean: A. The unit-test coverage is dense enough.

---

### Issue 3 — Schema-stability claim verified, but Mount field naming (`HostPath` for named volumes) is uglier than Joel admits

**Problem.** User's question 7. The M1 type:

```go
type Mount struct {
    HostPath      string `toml:"host_path"`
    ContainerPath string `toml:"container_path"`
    ReadOnly      bool   `toml:"read_only"`
}
```

Yes, this matches what M2 needs. Schema-stability promise holds. **But:**

`HostPath` is the *Go field name* and named volumes also live there. So an operator's TOML for a named-volume mount reads:

```toml
[[run.mounts]]
host_path = "mydata"     # a Docker volume name, not a host path
container_path = "/var/lib/app"
```

That's confusing. Joel §1 Decision 3 picked Option B (derive `IsNamed`, don't rename the Go field, keep `host_path` as TOML tag) because rename churn touches every test fixture. He's right that rename is more diff. He's wrong that it's "a small papercut" — every operator who hand-edits a TOML for a named volume will look at `host_path = "mydata"` and think they typed it wrong. Documentation can paper over it; better to not have to.

**The TOML key is locked** by schema-versioning (no rename without a v2 bump). So our choices are:
- Live with `host_path` for named volumes forever.
- Bump to schema_version 2 someday and rename to `source` (semantic break — old binaries reading `source = "mydata"` would fail).
- Add a comment-only doc on `Mount.HostPath` explaining the convention.

**Impact.** Latent operator-confusion when someone hand-edits a TOML for a named volume. M2 shipping doesn't make it worse — it makes it *visible* for the first time (M1 only ever wrote empty `mounts = []`).

**Options.**
- **Option A:** Joel's plan. Derive `IsNamed`. Rely on doc-comment + `_docs/usage.md` to explain. Pros: zero churn. Cons: latent confusion.
- **Option B:** Add the doc-comment Joel mentioned in §3.1 BUT also add a doc-comment on the `host_path` TOML *key* in `_docs/usage.md`'s service-TOML reference (which doesn't exist yet but probably should) explaining "for named volumes, `host_path` holds the volume name." Pros: addresses the user-visible side. Cons: separate doc work.
- **Option C:** Defer the rename to a future schema_version=2 task. Document as m1x-backlog item 12 "rename `Mount.HostPath` → `Mount.Source` (requires schema bump)." Pros: clean ledger entry. Cons: schema_version 2 has a high bar — we wouldn't bump JUST for this.

**My take.** Option B. The doc-comment on the Go field (Joel's plan already adds this in §3.1) plus a one-paragraph explainer in `_docs/usage.md` next to where the `--mount` syntax is documented. Raymond can land both in his step. The `host_path` TOML key is a permanent quirk; explaining it once costs little.

**DON.** Confirm A or B. C is "defer forever" which is fine but means the quirk persists. If you go B, lock the explainer wording so Raymond doesn't drift.

---

### Issue 4 — Decision 4 (Joel's β over Don's α): I agree with Joel

**User's question 3.** Joel overrode Don and chose β (add `Volumes []VolumeMount` to `RunRequest`) over α (consolidate to `RunWithOptions` everywhere).

Don's argument for α: "two run paths is a divergence bug factory." That's the principled position.

Joel's argument for β: blast radius (~20 mock expectations to rewrite), milestone discipline ("make the milestone smaller, not bigger"), and α can be done as no-op cleanup later (m1x item 11).

**Joel is right.** Three reasons:

1. **The two run paths exist for an actual reason today.** Caddy publishes ports; service deploys don't. `Run` and `RunWithOptions` aren't accidentally divergent — they're feature-divergent. Adding `Volumes` to `Run` brings them closer (now the only delta is "service deploys never publish ports / never set labels"), so β IS the cleanup direction. α is "and also do the rest of the cleanup" which is a separate task.

2. **The "no second consumer yet" rule.** RunRequest-becoming-RunOptions IS genuine future work — at M3 host bootstrap or M4 blue/green, when there's a third use site for `docker run` invocations, the convergence path becomes obvious. Doing it preemptively in M2 is over-engineering on speculation.

3. **β has zero net mock churn.** Adding a field to an existing struct doesn't change the gomock interface (verified in Joel §3.11). α changes the interface and forces 20+ test rewrites. The test rewrites would be mechanical but they'd be in the same PR as the actual behaviour change — when something breaks, the bisect lands on a 50-line behaviour change inside a 500-line mechanical refactor. Bad bisect surface.

**No action.** This is a correctly-defended override. m1x item 11 (Joel §1 Decision 9) is the right ledger entry.

**One small friction:** Joel's m1x item 11 description says "~1 hour mechanical work, zero behaviour change." That's optimistic — the rewrite touches ~20 test sites and at least one of them will surface an interaction with `Run`-vs-`RunWithOptions` semantics that needs thinking. Realistic estimate is 2-3 hours. Not blocking.

---

### Issue 5 — Decision 1 (no stat at deploy time): Joel's argument holds, but the operator-UX fallout deserves a sentence in `_docs/usage.md`

**User's question 2.** Joel overrode Don's "stat the bind source" lean with "let Docker speak."

Joel's argument: stat is a TOCTOU race; valid-at-deploy-time can be invalid-at-`decloud start`-time after a host reboot. Loader-time stat punishes the recoverable state where `/mnt/data` hasn't auto-mounted yet. Docker's error at `docker run` time is sufficient.

**This is correct but incomplete.** Docker's error for a missing bind source on Linux is:

```
docker: Error response from daemon: error while creating mount source path '/missing': mkdir /missing: read-only file system.
```

(Or similar — varies by Docker version and host filesystem.) That message is **not** what an operator typing `--mount /tmp/data:/data` and getting back exit 40 wants to see. They want "host path /tmp/data does not exist on this machine."

Joel acknowledged this in §8.3 ("That's enough context to debug. **No change.**"). I disagree with "no change." It's enough context for a Docker-experienced operator; for someone whose first decloud command is `decloud deploy service --mount`, the read-only-filesystem mkdir error is gibberish.

**But:** stat-at-CLI-time is still wrong because of the TOCTOU and the start-after-reboot scenario. So the right answer is to *document* the failure mode, not change the validation.

**Impact.** Operator-UX paper-cut on first-deploy with a typo'd bind source. Fails in a way that makes them blame Docker, not their typo.

**Options.**
- **Option A:** Joel's plan. No stat. No doc. Pros: Joel's TOCTOU argument intact. Cons: confusing first-deploy error.
- **Option B:** Joel's plan + add a sentence to `_docs/usage.md` near `--mount`: "If a bind source path does not exist on the host, `docker run` will fail with a filesystem error referencing the missing path. Decloud does not pre-stat bind sources, because the path may legitimately appear later (e.g. an automounted disk after host reboot)." Pros: operator finding this on Google understands. Cons: more doc.
- **Option C:** Joel's plan + a *warn-only* stat at CLI time (`fmt.Fprintf(os.Stderr, "warning: --mount source %q does not exist on this host\n", src)`). Pros: warns without blocking. Cons: introduces a new "warning" category we don't have today, sets a precedent for warnings everywhere.

**My take.** Option B. C introduces complexity for a corner case; A leaves the operator confused.

**DON.** Pick A or B. If B, lock the wording so Raymond doesn't paraphrase.

---

### Issue 6 — `:ro` only, no explicit `:rw` — Joel rejects `rw` with a specific error message; this is correct

**User's question 6.** Mount syntax is `host:container[:ro]` and `volname:container[:ro]`. The `:rw` case rejects with `"unsupported mode flag \"rw\" (only \"ro\" is supported)"`.

This is right. **Reasons:**

1. **Default is rw.** Saying `:rw` explicitly is redundant; rejecting it explicitly tells the operator "you're confused about the default."
2. **Future-proof.** If we ever add another mode flag (say `:nocopy` for named volumes, which Docker actually has), `:rw` would conflict with the implicit-default rule. Better to reject now than to have to define what `:rw` means later.
3. **Matches Docker's lazy default-emission behaviour.** Docker's `docker inspect` doesn't show `rw` flags; they're absent because they're the default. Decloud matches.

**Disambiguation rule for named-volume vs bind:** "starts with `/` → bind, else named." This is exactly Docker's rule (verified by reading `docker/docker/pkg/volume/volume.go`'s rule for short syntax). A relative-path source like `./data:/x` is rejected as not-matching the named-volume regex (slashes aren't allowed in volume names). The operator gets a clear error.

**Edge case Joel missed:** what about a bind source with a colon in the path? Docker's short-form syntax can't represent it (colon is the delimiter). `--mount /path/with:colon:/data` would split into 3 components → `["a", "b", "ro"]` and only "ro" would survive — actually it'd split into 4 components and fall into the "got 4 components" error. Operator sees error. Acceptable.

**Edge case Joel handled correctly:** comma-in-path. §8.9 — `StringArrayVar` not `StringSliceVar`. Critical and correct.

**No action.** Mount syntax is sound.

---

### Issue 7 — Help-text test deletion is correct (carve-out is exhausted)

**User's question 8.** `TestDeployService_MountFlagHelpReferencesM2` asserts `"M2 only"` substring in help text. Joel deletes it.

The carve-out at `cli-flag-surface-coherence.md:32-42` exists because milestone tokens like `"M2"` participate in a multi-surface contract: the help text says it, the runtime error says it, `_docs/usage.md` says it. When you change the milestone label, you have to change all four together. The semantic-token test catches drift.

**Once M2 ships, there is no milestone token in the help text.** The help text becomes operator-facing prose ("`<host-path>:<container-path>[:ro]`..."). Testing prose is change-detector. So the test goes away.

**The user asks: do we need a *different* token to assert?** I went looking. Candidates:

- `"<host-path>:<container-path>"` — appears in help text, error wording, and `_docs/usage.md`. Could be a token. But it's prose-shaped and the operator-friendly wording is the LOAD-bearing part; locking this in a test would re-create change-detector.
- `"[a-zA-Z0-9][a-zA-Z0-9_.-]+"` (the volume-name regex) — appears in help text and `mount.go` and `_docs/usage.md`. This IS a hard contract — if someone changes the regex without updating the help, drift. But the regex is locked by `volumeNameRE` in `mount.go`; there's no parallel surface that would drift independently.
- `"ro"` (the only mode flag) — appears in help, error, `_docs/usage.md`, and `parseMountString`. Could be a semantic token. But it's a literal; renaming `ro` is a breaking change everywhere; the contract is enforced by Docker compatibility, not by our tests.

**Conclusion: no useful semantic-token survives in M2 help text.** Joel's deletion is correct. The carve-out at `cli-flag-surface-coherence.md:42` becomes historical narration (Joel §11 row 9 has the right wording).

**No action.** Test deletion is justified. Carve-out doc update is in Joel's Raymond-deliverable list.

---

### Issue 8 — Atomic commit plan: two commits (RED/GREEN) is right

**User's question 5.** Joel argued §6 for two commits — Kent's all-tests-RED commit, Rob's all-production-GREEN commit. Don §14 item 9 said "atomic single commit."

These aren't actually in conflict. Don meant "production code flips all surfaces atomically" (you can't ship surface 1 without surface 2). Joel agrees and adds "test changes go in a separate commit because Kent ships before Rob." Both right.

**Two-commit shape:**
- Commit A (Kent): all test changes. Suite is RED.
- Commit B (Rob): all 10 production files. Suite is GREEN.

This is the standard test-first discipline. CLAUDE.md §"WORKFLOW - STEP 3" calls for separate Kent and Rob commits, so the workflow already mandates this.

**One concern:** the RED commit is technically a broken state. If anyone bisects across it, they hit a failing test. CLAUDE.md doesn't explicitly forbid this; the M1 tasks have done it. **Accepted as workflow norm.** No action.

**My take:** Joel's two-commit plan is correct. Don's "atomic" applies to the *production* commit; Kent's commit is correctly separate. Both planners align on substance even if the wording diverged.

---

### Issue 9 — Phantom kill (env-file hardening) is justified

**User's question 1.** Don §1a traced env-file hardening to a residue from the M3a-bundle. Joel concurred.

I re-read `internal/envcap/capture.go`, `_ai/envcap-portable-bash.md`, and `_ai/m1x-backlog.md` items 1-8. There is no concrete env-capture work hiding under the "hardening" label. The phrase came forward from the resequence task as boilerplate.

**The kill is correct.** Don's §1a trace is thorough; Joel verified.

**One nit:** Joel's docs sweep updates `_ai/MEMORY.md:7` and `_ai/decisions/m1-scope.md:32` to strip the phrase. This is fix-while-fresh applied correctly.

**No action.** Phantom is dead.

---

### Issue 10 — `store.go:69` empty-name papercut: in scope is right

**User's question 9.** Don and Joel both flagged `store.go:69` uses `cfg.Name` (which can be empty if TOML omits the `name = ""` line) instead of the `name` parameter (which is always populated from the filename).

Joel handled this in §3.4 — the new `ValidateMounts(cfg.Run.Mounts, name, cfgPath)` call uses the parameter `name`, fixing the papercut as a side effect of the rewrite.

**This is fix-while-fresh applied correctly.** The line is being rewritten anyway; using the better identifier costs zero diff and improves operator-debug context. Joel's call is right.

**However:** the `cfg.Name` lookup is *also* used at `store.go:73` (the strategy rejection block) and isn't being touched in M2. So Joel's "fix while fresh" is partial — the same papercut survives in the next block.

**Options.**
- **Option A:** fix `cfg.Name` → `name` in BOTH the mount block and the strategy block. Pros: complete. Cons: the strategy block isn't otherwise touched; this expands the diff.
- **Option B:** fix only the mount block (Joel's plan). Strategy block keeps `cfg.Name`. Pros: minimal scope. Cons: same papercut, half-fixed.
- **Option C:** punt both — don't fix the mount block either; pass `cfg.Name`. Pros: zero scope creep. Cons: ships M2 with the same loose end Joel correctly wants to close.

**My take.** Option A. While we're touching the file, fix the strategy block too. It's three characters (`cfg.Name` → `name`). Truly fix-while-fresh.

**DON.** Pick A or B. If A, also update the test (`TestStore_LoadRejectsNonEmptyMounts` is being deleted; the strategy-rejection test isn't being touched and would benefit from a TOML fixture that omits `name = ""` to verify the parameter-based identifier — but that's now an additive subtest, not a deletion).

---

### Issue 11 — `CurrentSchemaVersion` constant + Mount IsNamed empty-defensive: minor robustness nits

**Sub-nit 1:** Joel §3.1 puts `IsNamed` in `types.go` and adds a `"strings"` import there. Fine. But `Mount` validation lives in `mount.go`. Mixing methods on Mount across two files is a small structural smell.

Move `IsNamed` to `mount.go`. Same package, same struct, the validation logic and the type-method live together. Joel's plan as written compiles fine; this is purely cosmetic. **Don't gate the plan on this.**

**Sub-nit 2:** Joel §8.7 — `IsNamed()` returns false on empty source defensively. That's correct for "is this a named volume?" but somewhat misleading because an empty source isn't a *bind* either. The defensive answer is technically right (false-by-default protects against false-positive named-volume claims) but the function shouldn't be called with an empty Mount in production. A cleaner contract: `IsNamed()` is undefined behaviour for `Mount{}` — but since it's a method, you can't enforce that. Live with it.

**No action.** Both sub-nits are below the bar for blocking the plan.

---

### Issue 12 — Volume-name regex allows volume names that are valid by Docker today but might be invalid in some Docker versions

**Problem.** Joel §3.2 picks `^[a-zA-Z0-9][a-zA-Z0-9_.-]+$` — minimum 2 chars. Joel §8.1 verifies this matches Docker's `volume.IsValidName`.

I cross-checked. Docker's regex (in `docker/docker/pkg/volume/local/local.go` and earlier in `pkg/volume/volume.go` history): `[a-zA-Z0-9][a-zA-Z0-9_.-]+`. Yes, 2+ chars, exactly Joel's regex.

**One Docker quirk Joel didn't flag:** very old Docker versions (pre-1.13) accept single-char volume names. `volume create x` works on some daemons. We're targeting modern Docker (M1 baseline is whatever ships with Ubuntu 22.04+), so single-char rejection is fine, but if an operator has an existing single-char volume from a legacy setup and wants to bind it via `--mount`, they get rejected by our regex. They'd have to rename the volume or hand-edit the TOML (which the loader would also reject).

**Impact.** Theoretical. No reasonable operator names a Docker volume `x`.

**No action.** Joel's regex is right. If this ever bites someone, the fix is to widen the regex.

---

### Issue 13 — Mock regeneration claim ("Net mock impact: zero files") needs a sanity check

**Problem.** Joel §3.11 claims no mock regen is needed because the changes are struct-field additions, not interface-method changes. Verified from the diff list:

- `RunRequest` gains `Volumes` field — no interface change. ✓
- `deploy.Request` gains `Mounts` field — no interface change. ✓
- `registry.Service` shape unchanged. ✓

But: **mockgen's source-mode regen will produce identical output regardless.** Rob's `go generate ./...` should produce a no-op diff. If it doesn't, something Joel didn't anticipate is changing. That's a workflow check, not a plan flaw.

**No action.** Joel's call is correct; Rob will verify by running `go generate ./...` and getting an empty diff.

---

### Issue 14 — `TestStore_LoadAcceptsEmptyMountsArray` survives untouched: confirm

**Problem.** Joel §4.2 says: "**KEEP unchanged:** `TestStore_LoadAcceptsEmptyMountsArray` (lines 251-259). Still meaningful — empty mounts must still be a valid round-trip."

Verified by my grep earlier. The test exists at `store_test.go:251-259` and asserts an empty mounts array round-trips cleanly. Under M2, this test still passes (empty is still valid). **Joel's call is right.**

**One small friction:** the test asserts `assert.Empty(t, svc.Config.Run.Mounts)`. Under M2, an empty TOML `mounts = []` would deserialize to `nil` (no element), and `assert.Empty` passes for both `nil` and `[]Mount{}`. So the test is robust. ✓

**No action.**

---

## Worst architectural decision in this plan

**There isn't one.** Both planners did their jobs.

If forced to identify the *thinnest* part: **Issue 1 (the dual-sentinel chain in `parseMountFlags`).** It works by accident of case ordering in `exit_codes.go` and a future refactor could silently flip the exit code. It's a one-line fix (Option B above). The fact that this is the worst thing in the plan tells you the plan is sound.

A close runner-up: **Issue 2 (the integration test narrowing).** Joel rewrote it mid-plan to a `Driver.Run`-direct test, which is narrower than what Don's §8 sketched. The narrow version is cheaper but skips the CLI/loader/orchestrator path. If something breaks at deploy time on the maintainer's homelab, the narrow integration test won't catch it.

Neither rises to "stupid."

---

## Cross-checks I performed

For my own due diligence (and so future-Linus can verify):

1. **M1 `Mount` schema shape.** Read `internal/registry/types.go:52-63`. `Mount{HostPath, ContainerPath, ReadOnly}` is exactly the shape M2 needs. Schema-stability claim verified.
2. **`ErrMountsNotSupported` deletion sites.** `git grep -F ErrMountsNotSupported` returns 8 production sites + Don/Joel/me references in `_tasks/`. Joel's §5 enumeration matches.
3. **`StringSliceVar` vs `StringArrayVar`.** Joel §8.9 caught this. I verified by checking `--host` (legitimately uses StringSliceVar; hostnames can't have commas) vs `--mount` (must be StringArrayVar; paths can have commas). Critical fix.
4. **Atomic-commit five-surface flip.** Mapped Joel §6 to the five surfaces in `cli-flag-surface-coherence.md`: runtime check, error message, help text, `_docs/usage.md`, semantic-token test. All five flip together. ✓
5. **Phantom env-file hardening.** Re-read `internal/envcap/capture.go` and `_ai/envcap-portable-bash.md`. No hidden work. Phantom kill verified.
6. **`name` parameter in scope at `store.go:Load`.** Verified: `func (s *fsStore) Load(ctx context.Context, name string)` — `name` is in scope at line 68 where the rewrite happens. Joel §3.4's claim holds.
7. **`exit_codes.go` case ordering.** Verified Issue 1's footgun: `errUsage` case is at line 39, `ErrInvalidMount` at line 41. Order matters. Issue 1 is real.

---

## DECISION: APPROVED WITH MINOR REVISIONS

Joel/Don, before Kent starts, lock these in a small addendum (call it `005-don-joel-addendum.md` or just amend the tech plan):

1. **Issue 1 — Dual-sentinel chain.** Pick Option A (comment in `exit_codes.go`) or Option B (extract reason-only in `parseMountFlags`). My lean: B. Locks behaviour against case reordering.

2. **Issue 5 — No-stat operator UX.** Decide: ship Joel's plan A (no doc) or plan B (one paragraph in `_docs/usage.md` explaining the "we let Docker speak" choice). My lean: B. One paragraph saves one frustrated operator from a Google search.

3. **Issue 10 — `cfg.Name` → `name` papercut in strategy block too.** Pick Option A (fix both blocks) or B (Joel's mount-block-only). My lean: A. The diff is three characters.

The rest is fine. Joel made the right calls on Decisions 1 and 4 (overrides of Don). Don made the right calls on phantom-kill, integration-test-bundling, semantic-token-deletion, and atomic-commit. The plan stands up to scrutiny.

Once Don/Joel respond on the three above, **Kent can start.** No PLAN v2 needed; an addendum is sufficient.

---

## Files reviewed (absolute paths)

Planning:
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/001-user-request.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/002-don-plan.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`

Code (sampled for fact-checking):
- `/Users/fenster/dev/decloud/internal/registry/types.go`
- `/Users/fenster/dev/decloud/internal/registry/errors.go`
- `/Users/fenster/dev/decloud/internal/registry/store.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service.go`
- `/Users/fenster/dev/decloud/internal/cli/exit_codes.go`
- `/Users/fenster/dev/decloud/internal/cli/deploy_service_test.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/driver.go`
- `/Users/fenster/dev/decloud/internal/dockerdrv/cli_driver.go`
- `/Users/fenster/dev/decloud/internal/deploy/service.go`
- `/Users/fenster/dev/decloud/internal/deploy/lifecycle.go`

Cross-references:
- `/Users/fenster/dev/decloud/_ai/cli-flag-surface-coherence.md`
- `/Users/fenster/dev/decloud/_ai/decisions/schema-versioning.md`
- `/Users/fenster/dev/decloud/_ai/m1x-backlog.md`
- `/Users/fenster/dev/decloud/_ai/decisions/m1-scope.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-26-m1-implementation/021-don-final-signoff.md`
- `/Users/fenster/dev/decloud/_tasks/2026-04-28-milestone-resequence/004-linus-plan-review.md`
