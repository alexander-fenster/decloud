# Linus Review: M1 Execution Plans (`02-plan.md` + `03-tech-plan.md`)

**Reviewer:** Linus Torvalds (high-level review)
**Reviewing:** Don's `02-plan.md` and Joel's `03-tech-plan.md` for M1 execution.
**Prior approval:** `_tasks/2026-04-26-readme-implementation-planning/07-linus-review-v2.md` (planning round, APPROVED).

---

## VERDICT: REVISIONS REQUESTED

The plans are 90% solid. Don and Joel did real work and faithfully translated the user's two new constraints (unit-tests-only; docs-as-deliverables) without dropping the planning-round commitments. I would not block on the five things they explicitly flagged for me — those are mostly defensible — but the plans have **two real holes I am not willing to wave through** and a third I want logged before Kent starts. Fix these, then ship.

The verdict is REVISIONS REQUESTED, not APPROVED, because Hole #1 (unspecified Lifecycle behavior) is a genuine spec gap that will cause Rob to invent the design at implementation time, which is exactly what we are trying to prevent. Hole #2 (readiness mechanism) is defensible but we need to commit to ONE answer rather than three half-answers.

---

## Answers to the five things Joel asked me to attack

### 1. `Store.RollbackPartialCreate` (Joel §9.5)

**Defensible. Keep it. Improve the name.**

Joel argues the orchestrator would otherwise need to know fsStore's path layout (`config/services/<name>.toml`) to delete the orphan. That argument is correct. The alternative — exposing the config-file-path computation as a separate `Store.ConfigPath(name) string` method and letting the orchestrator call `os.Remove` — would leak more, not less, because then anyone could compute paths and write to them outside Store's control. Joel's shape is right.

**Two nits:**
- The name `RollbackPartialCreate` reads like it understands transactions, which it does not — it just deletes the config file. Call it `DeleteConfigOnly(ctx, name)` or `DeleteOrphanConfig(ctx, name)`. The orchestrator decides when "partial create" applies; the Store's job is just "delete config without touching secrets." Rename in implementation. Not a blocker.
- The interface gains a method that has exactly one caller (the deploy orchestrator's step-7b error branch). If we ever grow a second store backend (we will not in M1, probably never), the new backend has to implement this for one call site. Acceptable.

**No revision required on this.**

### 2. Readiness via `docker run --rm --network=decloud curlimages/curl:8.5.0 ...` (Joel §9.4)

**This is the FIRST real hole.** Joel's §9.4 contradicts itself in 30 lines: it specifies an `httpProbe` struct doing direct HTTP probes to `http://<containerName>:<port>...`, then in the next paragraph admits that won't work because the host can't resolve Docker DNS, then proposes either `docker exec ... wget` or a one-shot curl image, then settles on the curl image, then flags it for me. The result is a section that describes three possible designs and commits to none.

Real problems with the curl-image approach:
- **Third-party Docker Hub image at deploy-time first use.** `curlimages/curl:8.5.0` is a fine image but it is not ours. Docker Hub rate-limits anonymous pulls. If Docker Hub is down at the moment of the operator's first deploy, the deploy fails at the readiness step with a misleading error. We do not control the supply chain of our own readiness probe. This is the kind of dependency that gets you a 3am page during the one outage that matters.
- **Cold-start latency.** First use pulls ~10MB; subsequent uses are cached. The operator's first deploy has unexplained extra latency.
- **Implicit assumption that image is pulled before timeout.** What's the readiness timeout default? 60s. What if Docker Hub is slow today and the pull alone takes 45s? Now we have 15s to actually probe.

**Options:**
- **Option A (Joel's curl image):** Pros — works, simple, no host-side networking dance. Cons — third-party supply chain, first-use latency, no control.
- **Option B (`docker exec <containerName> ...`):** Probe from inside the container itself. Pros — no extra image, no network resolution problem, no Docker Hub. Cons — requires `wget` or `curl` to be in the user's image. Many minimal images (alpine without `apk add curl`, distroless, scratch) don't have either. We would document the requirement. **This pushes the supply-chain problem onto the user, where it belongs — they chose their base image.**
- **Option C (host-side probe via published port):** Add `-p 127.0.0.1:<random>:<containerPort>` for the probe duration only, probe `http://127.0.0.1:<random>/<path>`, then... no, you can't `docker run` and then add a port mapping later. Would require running with the published port permanently, contradicting "no `-p`" (Joel §11.2). Reject.
- **Option D (host-side probe via `docker inspect` to learn the container IP):** Get the container's IP on the `decloud` bridge network from `docker inspect <name>`, probe `http://<ip>:<port>...` directly from the host. Pros — no extra image, no exec dependency on user's image, deterministic. Cons — relies on the bridge network being reachable from the host's network namespace, which it IS for the default `bridge` driver but is documented as unreliable for some custom networks. We use a default-driver bridge (`docker network create decloud` with no `--driver` flag means default `bridge`), so this works. **This is what I would have written.**
- **Option E (HEALTHCHECK in Dockerfile + `docker inspect --format '{{.State.Health.Status}}'`):** Use Docker's own health check primitive. Pros — built into Docker, no extra processes, the user already has the mechanism if they declared HEALTHCHECK. Cons — requires HEALTHCHECK in Dockerfile (which the README mentions as one of the two readiness paths), so we'd be requiring it. M1 also wants HTTP-path-based readiness for users who didn't declare HEALTHCHECK. Hybrid possible.

**My recommendation: Option D.** Probe from the host using the container's bridge IP discovered via `docker inspect`. No third-party image. No supply-chain risk. Works on the maintainer's macOS dev box where Docker Desktop's bridge is reachable from the host. Joel can hide this behind the existing `Driver` interface as `Driver.ContainerIP(ctx, name) (string, error)` and the readiness probe becomes the simple `httpProbe` Joel started with.

Joel's response in the v2 plan §9.4 — "the cleanest M1 solution is the one-shot" — is wrong. The cleanest solution is the one that doesn't pull a third-party image into your deploy-critical path.

**DON: pick Option D or accept Option B (docker exec, document the user-image requirement). Joel updates §9.4 with the chosen single answer.**

### 3. Container port not published to host (Joel §11.2)

**Defensible — but you're conflating two things.** Caddy reaching the container on the shared network is correct (it's the design point of the README). But you're using "no `-p`" to also justify not having any host-side debug access. Those are different problems.

When an operator wants to debug a service that Caddy is failing to route to (say, the readiness probe is flaky and they want to manually `curl` from the host), the answer right now is `docker exec <name> wget -q -O- http://localhost:<port>/`. That's fine. **As long as it's documented in the operator-facing doc.** Raymond's `_docs/operator/usage.md` §6 ("Common errors") needs one paragraph: "If you need to probe a container directly, `docker exec <name> ...` — we deliberately do not publish container ports to the host because Caddy is the only ingress."

**No code change. Doc requirement. Add to Raymond's deliverables in `02-plan.md` §2.2.2.6.**

### 4. Mockgen layout asymmetry (Joel §5.1)

**Smell, but acceptable.** The asymmetry is real: every other interface lives next to its mock under `<pkg>/mocks/`, but `ServiceDeployer` and `Lifecycle` mocks live under `internal/cli/mocks/` because CLI is the only consumer.

Joel's defense: "co-locating with the only consumer beats locating with the interface when there's exactly one consumer." That is a tradeoff, not a wrong answer. Two arguments against:

- **The rule "mock lives next to interface" is one rule. The rule "mock lives next to interface unless the interface has exactly one consumer in which case it lives next to the consumer" is two rules.** Future maintainers (including me, six months from now) will look at `internal/cli/mocks/mock_deployer.go` and wonder why this mock didn't follow the convention. They have to read the comment in the generated file to find out. Mild cost, ongoing forever.
- **What happens when the second consumer arrives?** If in M2 we add a `decloud bootstrap` command that exercises a deploy, suddenly there are two consumers and the mock has to move (or be duplicated). Schedule the move now or accept duplicate mocks later.

Counter: if we never get a second consumer (likely!), Joel's layout is the right answer.

**My take: Option B (Joel's): co-locate mock with consumer, document the deviation in a one-line comment in the generated mock file's header.** This is a 60/40 call; either way is defensible. Joel decided; I accept. **No revision.**

### 5. LICENSE + CI deferral (Don §3, Joel §3)

**LICENSE deferral: accept reluctantly. CI deferral: this is the SECOND real hole, given the user constraint.**

**LICENSE:** Joel is right that "the user has not greenlit a license choice and I will not pick one for them." Fine. But the module path `github.com/alexander-fenster/decloud` with `go install` as the documented install path means anyone who follows the `_docs/operator/installation.md` instructions is downloading code without a license. Default copyright applies; technically the operator does not have permission to do anything with the binary they just installed. The maintainer should be told this in writing so they can decide. **Don/Joel: add ONE sentence to `_docs/operator/installation.md` saying "Note: this repository does not yet declare a license; ask the maintainer if you intend to use the binary in a context that requires explicit license grant." That covers us. Then defer the actual LICENSE file.**

**CI:** Joel argues "M1 acceptance gate is `go test ./...` on the maintainer's macOS box." Fine for the M1 acceptance run. **But:**
- The user explicitly said "I will test it on a real system after M1 is done." That means the user picks up the binary post-M1 and starts running it on a real Linux host. They are not Rob. They will not re-run `go test ./...` after every change Rob makes between "Rob's last test pass" and "user picks it up." If Rob makes a change after Kent's last test run and breaks a test, the only thing catching that is the next person who runs `go test ./...` — which might be the user, in production, after a confusing failure.
- The unit-tests-only constraint AMPLIFIES this risk. Without integration tests, the unit tests are the ONLY automated signal that anything works. Without CI, there is NO automated signal — only the discipline of whoever last ran the tests.
- Joel's argument that "we have not confirmed a public GitHub repo" is wrong: the `git remote -v` of this repo, or just the user's verbal answer, settles it in 30 seconds. The maintainer is `alexander-fenster`; the module path is `github.com/alexander-fenster/decloud`; this is on GitHub.

**Options:**
- **Option A (Joel's defer):** Pros — saves writing 20 lines of YAML. Cons — no automated signal between M1 ship and user's real-system test; user discovers regressions manually.
- **Option B (minimal CI):** Add `.github/workflows/test.yml` that runs `go test ./...` on push and PR to `main`. Twenty lines of YAML. No matrix. No nightly. No release pipeline. Just "did the tests pass." Pros — closes the regression-window between Rob's last manual run and the user's real-system test. Cons — assumes GitHub, which is a 30-second confirmation away.
- **Option C (defer with mandatory pre-handoff signoff):** Defer CI, but require Rob to attach `go test ./... -v -count=1` output to his step-3b report so we have a frozen test-pass receipt at handoff time. Pros — no CI infra. Cons — only catches regressions BEFORE handoff, not between handoff and user-test-time.

**My recommendation: Option B.** Twenty lines of YAML, two-minute job, eliminates the "we shipped untested code because nobody re-ran the tests" failure mode. The user explicitly told us they're not going to be running unit tests; that means SOMEBODY else has to be the one running them on every change, and CI is the obvious answer.

**DON: confirm with the user (one question: "is this hosted at github.com/alexander-fenster/decloud and may I add a GitHub Actions workflow for `go test ./...`?") and pick A or B. If A, then mandate Option C as the consolation prize.**

---

## Holes I found on my own initiative

### Hole #1 (BLOCKER): The Lifecycle methods have no specified behavior

This is the biggest miss in the execution plan. Joel §9.1 and §8.3 declare a `Lifecycle` interface with seven methods (`Unregister`, `Start`, `Stop`, `Restart`, `Status`, `Logs`, `CaddyReload`), and §13.6 includes ONE test (`TestUnregister_DelegatesToLifecycle`) that asserts the CLI command calls the interface method. **Nowhere in the entire 03-tech-plan does anyone specify what those methods actually DO inside `*serviceDeployer`.**

Specifically:
- **`Unregister`** — does it Stop the container? Remove it? Then `Store.Delete` (secrets-first per §3.3 of plan-v2)? Then regenerate Caddyfile? Then `caddy reload`? In what order? With what rollback semantics if `caddy reload` fails after the container is already removed? **Nothing in 03-tech-plan tells Rob.**
- **`Stop`** — Stop the container, fine. But does it update `state.State` in the registry? If you `decloud stop foo` and then `decloud status foo`, does status report "stopped" because the container is gone, or because we recorded it? If we don't record it, the next `decloud start foo` has to figure out from `docker inspect` what to start, which it can do, but the design needs to be stated.
- **`Start`** — Start what? The previously-deployed image? Re-`docker run` with the captured env from the secrets file? What if the image is no longer in the local cache (M6 image-GC removed it)? Does Start fail or does it rebuild?
- **`Restart`** — `docker restart` (preserves container) or stop+remove+run-fresh (recreates container)? These have different consequences for in-memory container state.
- **`Status`** — what fields are populated, where do they come from (registry vs `docker inspect` vs both)?
- **`Logs`** — the easy one. `docker logs` with follow/tail. Joel sketched the options struct but not the impl.
- **`CaddyReload`** — does it regenerate the Caddyfile from the registry first, or just call `caddy reload` on whatever's on disk? README implies regenerate-then-reload; the tech plan doesn't say.

The §13.5 `internal/deploy` test list has zero `TestLifecycle_*` tests. The §13.6 `internal/cli` list has only delegation tests ("the CLI calls the right method on the mock"). So Rob writes seven method bodies with no spec and no tests, then Linus discovers the design at code-review time. That's exactly the workflow CLAUDE.md is designed to prevent.

This is a real gap. M1 acceptance criterion #6 in plan-v2 §7.1 explicitly requires `decloud status foo`, `decloud logs foo`, `decloud stop foo`, `decloud start foo`, `decloud restart foo`, `decloud unregister foo` to "behave per their names" — and `unregister` is specified to remove both registration files in the §3.3 order, regenerate the Caddyfile, and reload. None of that detail made it into 03-tech-plan.

**Options:**
- **Option A (Joel writes a §9.6 spec):** Add a new section to 03-tech-plan that specifies each Lifecycle method's behavior — input, output, side effects, rollback, error wrapping. ~150 lines. **DO THIS.**
- **Option B (Defer Lifecycle to a follow-up task):** Cut from M1, ship just `deploy service` + `caddy reload` + `unregister`. Cons: M1 acceptance criterion #6 is broken; the user can't actually use the system end-to-end. Reject.
- **Option C (Wing it):** Let Rob design the methods at implementation time. Cons: "no coding at top level" is the project's foundational rule. Reject.

**My recommendation: Option A. Block M1 execution until Joel writes §9.6 with at minimum the Unregister and Restart specs (the ones with non-trivial ordering); Stop/Start/Status/Logs can be sketched briefly because they're mostly mechanical.**

**DON: this is the blocker. Joel needs to add a §9.6 to 03-tech-plan before Kent starts.**

### Hole #2 (BLOCKER): Caddy reload failure after a successful new container — what does the operator see?

Joel §9.2 step 8c: "If reload fails, log warning, exit `errCaddyReload`, don't roll back. New container is up; routing is degraded. Operator fixes Caddy by hand."

That is correct policy. But the implementation of "operator fixes Caddy by hand" is undefined. Specifically:
- The Caddyfile on disk has been UPDATED (§9.2 step 8b, atomic write). It reflects the new state. The operator knows this how?
- The previous Caddyfile is GONE (atomic write replaced it). Caddy is still serving requests against its in-memory config (which is the OLD file). When Caddy restarts (e.g. host reboot), it loads the NEW file from disk. If the new file is what reload rejected, Caddy fails to start. Now we have an outage on the next reboot, hours or days after the failed deploy, with no obvious causal link.
- The `decloud caddy reload` subcommand exists — does it just call `caddy reload` against the current on-disk file (which Caddy already rejected once), or does it regenerate from the registry first? If the former, the operator's "fix" is to fix the Caddyfile by hand and then pray nobody redeploys. If the latter, the operator's "fix" is to remove the broken registration and re-run.

**This needs a concrete recovery story documented in `_docs/operator/usage.md` §7 ("Common errors").**

**Options:**
- **Option A (Document recovery procedure):** "If `caddy reload` fails after a successful container deploy, the Caddyfile on disk reflects the new state but Caddy is running the old config. To recover: (1) inspect `/opt/declouding/config/caddy/Caddyfile` and the Caddy error log to find the syntax issue; (2) if the issue is in a specific service's stanza, `decloud unregister <name>` will remove that stanza and regenerate; (3) re-run `decloud caddy reload`." Pros: honest, gives the operator a path. Cons: requires the operator to understand Caddy syntax.
- **Option B (Pre-validate before write):** Run `caddy validate --config /tmp/<generated>.tmp` BEFORE the atomic-rename in step 8b. If validate fails, abort the deploy at step 8b with `errCaddyReload`, leaving the OLD Caddyfile on disk untouched. Pros: never produces a broken on-disk state. Cons: requires `caddy` to be on PATH (it already is, per installation step 3), adds a subprocess invocation.
- **Option C (Keep last-known-good Caddyfile):** Atomic-rename the existing Caddyfile to `Caddyfile.lkg` before writing the new one. On reload failure, atomic-rename `Caddyfile.lkg` back to `Caddyfile`. Pros: on-disk state is always reload-able. Cons: another file to manage; race conditions if two deploys happen concurrently (which we don't support in M1, but M3+ might).

**My recommendation: Option B (pre-validate) PLUS Option A (document the recovery).** Pre-validation closes the "broken file on disk" failure mode entirely, and it's three lines of Go (call `caddy validate --config <tmp>`, error if non-zero). The doc path is a backstop for cases where validate passes but reload fails for runtime reasons (port already bound, etc).

**DON: pick. This is a small fix but it has to be a deliberate decision.**

### Hole #3 (NOT a blocker, but log it): `state/deploys/<name>/<deploy-id>/source.tar.gz` is in plan-v2 §6.2 but absent from 03-tech-plan

Don's plan-v2 §6.2 includes a `state/deploys/<name>/<deploy-id>/source.tar.gz` directory in the M1 disk layout, with the comment "source bundle at deploy time, for forensics" and "auto-included in M6 backups." Joel's `06-tech-plan-v2.md` §6.x defines `Paths.DeploysDir` pointing at `state/deploys`. But the M1 execution plan (`03-tech-plan.md`):
- §2 file tree: no mention of any source-bundling code.
- §9 deploy step sequence: ends at Caddy reload; no tarball-write step.
- §13 test plan: no test for state/deploys/ population.

So either (a) this was silently dropped from M1, or (b) Joel forgot. Since neither plan-v2 §7.1 acceptance criteria nor 02-plan §6 DONE criteria mention the tarball, I lean (a) — implicit deferral to M2 or later. **But the deferral should be explicit in 02-plan §3 alongside LICENSE and CI**, with a note "the `state/deploys/` tree is created (mode 0755) by the install steps in `_docs/operator/installation.md` but populated by no M1 code; M6 backups will sweep an empty tree harmlessly."

If Don actually wants the tarball in M1, that's a §9 step Joel needs to add. **My take: defer is fine. Just be explicit.**

**DON: confirm deferral; Joel adds one row to the §3 deferrals table in 02-plan.**

### Hole #4 (NOT a blocker, design intent check): `internal/envcap` test execution and `set -a` semantics

Don §2.1 says "the `internal/envcap` tests run against the real `/bin/bash` on the maintainer's box — these ARE unit tests." Fine. But CLAUDE.md item 4 says "use Gomock if a mock is needed." There's no mock for bash — there's just real bash. That means:
- **The tests are not hermetic across machines.** Different `/bin/bash` versions (3.2 on macOS default, 5.x on Linux) will exhibit slightly different behavior. The portability fix in plan-v2 §4.1 covers the common cases but Linus-review-v2 already flagged three borderline cases (`set +a`, arrays, readonly). Without integration tests AND without a mocked bash, the only thing catching a future regression is the maintainer running `go test ./internal/envcap/...` on both macOS AND Linux. Without CI (Hole "5" above), only macOS gets exercised regularly.
- **`set -a` interaction with the user's `env.sh`.** Plan-v2 §4.1 says we set `set -a` before sourcing. If the user's script does `set +a` (per Linus-v2 borderline case 1), variables after that point are silently dropped. The unit test in §13.2 needs to explicitly test this case so the failure mode is locked in by a test, not just documented.

**Options:**
- **Option A (accept):** Trust that Kent's §13.2 reference to "06-tech-plan-v2 §3.5 (full table — Kent copies the table into test names)" includes the `set +a` case. Pros: zero new work. Cons: I haven't read §3.5 to verify, and "Kent copies the table" is a chain-of-handoff that can lose entries.
- **Option B (require explicit cases):** 03-tech-plan §13.2 enumerates by name: `TestEnvcap_SetAOff_VariablesDropped`, `TestEnvcap_ArrayDeclaration_OnlyFirstElementCaptured`, `TestEnvcap_ReadonlyConflict_FailsWithSetE`. Pros: explicit, no handoff loss. Cons: 6 lines of writing.

**My take: Option B, three test names listed in §13.2.**

### Hole #5 (NOT a blocker, but worth noting): The `internal/cli` deployer factory test seam

Joel §8.2 uses `var deployerFactory = buildProductionDeployer` as a package-global override seam. Tests reassign it. **This is not safe under `t.Parallel()`.** If two CLI tests run in parallel and both reassign `deployerFactory`, they race. Joel's test list (§13.6) doesn't say whether tests run parallel; CLAUDE.md doesn't mandate it; the standard library-style is "tests are sequential by default."

**Options:**
- **Option A (accept):** Don't call `t.Parallel()` in any `internal/cli` test. Document the constraint in a comment near `deployerFactory`. Pros: simplest. Cons: future maintainer adds `t.Parallel()` and gets a flaky test that only fails on busy CI.
- **Option B (proper DI):** Pass `deployerFactory` through `rootContext` instead of a package global. `NewRootCmd(opts ...RootOption)` with a functional-options pattern; tests construct with their mock. Pros: thread-safe, no globals, idiomatic. Cons: refactor, ~30 lines.
- **Option C (Mutex on the global):** sync.Mutex around reassign. Pros: minimal. Cons: still global, still ugly.

**My take: Option B if Rob has bandwidth, Option A with a comment if not.** This is a minor design point; flagging so Rob doesn't blindly add `t.Parallel()` later.

---

## Things 02-plan and 03-tech-plan got RIGHT (rare praise)

- **Don's framing of "Raymond MUST NOT invent a phantom systemd unit for `decloud` itself" (§2.2.1).** That is exactly the kind of trap a doc-writing agent falls into. Naming it explicitly in the plan kills the failure mode.
- **Joel's §9.2 rollback table** — one row per step, with explicit "what rolls back if THIS step fails." Eight steps, eight failure modes, eight rollback strategies. The distinction between forward-only-after-step-7b and rollback-on-failure-before-7b is the kind of design clarity I rarely see at this stage. Step 7b's "ErrPartialWrite triggers RollbackPartialCreate THEN container rollback" vs "any other Save error triggers ONLY container rollback" is correctly captured as two separate test cases in §13.5.
- **Joel's §11.3 "Why this shape is the right answer for Knuth-review."** Calling out the three subtle decisions (env map ordering, --env vs --env-file, stdout/stderr handling) where Rob might second-guess saves a Knuth round-trip. Each one has a defended answer. Good preemptive design work.
- **Joel's §5 mockgen layout decision PLUS the explicit `-source=` justification (§5.4)** — knowing the chicken-and-egg between source-mode and reflect-mode generation is the kind of detail that bites you on day one of a fresh `go generate` and is hard to debug. Calling it out in the plan eliminates the bite.
- **The two user-facing docs in 02-plan §2.2 with concrete contents.** Raymond doesn't have to invent the structure. Don wrote the doc outline; Raymond fills it in. Reduces the "Raymond hallucinates an architecture" failure mode that Kevlin then has to catch.
- **Don's M1 DONE criteria (§6) — twelve numbered items, each testable.** Every iteration round, when Don re-checks DONE, he can walk down this list. Excellent.

---

## Summary of revisions requested

To go from REVISIONS REQUESTED to APPROVED, fix:

**Blocking:**
1. **Hole #1** — Joel adds §9.6 to 03-tech-plan specifying Lifecycle method behavior (Unregister, Restart most important; Stop/Start/Status/Logs/CaddyReload sketched briefly). §13.5 gains `TestLifecycle_*` tests, at minimum one per method covering happy-path + one failure branch for Unregister.
2. **Hole #2** — Don picks between pre-validation (Option B) and recovery-doc (Option A) for `caddy reload` failure handling. Joel updates §9.2 step 8 accordingly.
3. **Joel-flagged item #2 (readiness)** — Don picks Option D (host-side probe via `docker inspect` IP discovery) or Option B (`docker exec` with documented user-image requirement). Joel rewrites §9.4 with the chosen single answer; the contradictory three-option text goes away.
4. **Joel-flagged item #5 (CI)** — Don confirms with the user whether to add minimal `.github/workflows/test.yml`. If no, Rob attaches `go test ./... -v -count=1` output to his step-3b report.

**Non-blocking (do these in the same revision pass):**
5. **Hole #3** — `state/deploys/` tarball: Don decides defer-or-implement; if defer, add to 02-plan §3 deferrals table.
6. **Hole #4** — Joel adds three explicit test names for env-capture borderline cases to §13.2.
7. **Hole #5** — Joel notes in §8.2 that `deployerFactory` is not parallel-safe (or refactors to functional options if Rob has bandwidth).
8. **Joel-flagged item #3 (no port published)** — Raymond's `_docs/operator/usage.md` §7 gets one paragraph on `docker exec` for direct probing.
9. **Joel-flagged item #5 (LICENSE)** — Raymond's `_docs/operator/installation.md` gets one sentence about no-license-yet.
10. **Joel-flagged item #1 (`RollbackPartialCreate`)** — rename to `DeleteOrphanConfig` in implementation. Tiny.

After these revisions land, I'll re-review and approve. Ship the second-round 03-tech-plan back to Linus before Kent starts coding.

End of review.
