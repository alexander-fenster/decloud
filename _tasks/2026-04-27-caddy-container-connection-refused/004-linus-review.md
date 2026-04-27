# 004 — Linus Review: containerised Caddy plan

Author: Linus Torvalds (high-level review agent)
Date: 2026-04-27
Reviews: `002-don-plan.md` (Don) and `003-joel-tech-plan.md` (Joel).

---

## TL;DR

The diagnosis is right. The fix is right. Joel's decomposition is mostly right. There are roughly **three things worth fighting about** before anyone writes a line of code, and the rest is fine. I'm going to spend most of this document on the three things, because if we get those wrong we ship a worse architecture than the bug we're fixing.

**Verdict: REVISIONS REQUIRED.** Not because either of you produced bad work — you didn't — but because there is one straight-up scope error (§11.3 readiness loop), one scope-creep that needs to be cut (`--image` flag, Viper wiring, TOML config), and one underspecified case that will bite the operator (admin-API boundary across `caddy reload` after `caddy down`). Everything else is non-blocking; I list those at the bottom as nits.

---

## 1. Did Don pick the right fix?

Yes. The rejection of Candidates A (hard-code IPs) and B (publish container ports) is sound and well-argued. I want to walk through the alternatives Don did NOT enumerate, because if any of them were better we should know now, not after we ship Candidate C.

### 1.1 Alternatives Don missed (and why C still wins)

**`host.docker.internal` / extra_hosts.** Reverses the arrow — Caddy on host, container reaches host. Useless here; we need host→container by container name, not container→host by alias. Reject.

**`--network host` for Caddy (containerised but in host netns).** Container shares host's network namespace, which means it ALSO does NOT use Docker's embedded DNS. Same bug, different paint. Reject.

**`--network container:decloud-<service>`.** Caddy joins one specific service's netns. Catastrophically wrong: would force a fixed coupling between Caddy and a single service container, breaking the moment a second service deploys. Reject.

**Sidecar pattern (one Caddy per service container).** Real architectural option for some platforms but it's "Kubernetes" thinking on a single-host MVP. We have one ingress; one Caddy. Reject for M1.

**Run Caddy on the host but inject `decloud-<name>` into `/etc/hosts` after each deploy.** This actually works. It's also disgusting: races against ACME-driven cert reloads, requires Caddy reload after every host-IP rotation, and means the Decloud orchestrator now writes to `/etc/hosts` which is a hard "no" on operator-trust grounds. Reject.

**Run Caddy on the host with `--resolvers 127.0.0.11` in the Caddyfile.** Doesn't work — `127.0.0.11` is Docker's embedded DNS server, but it ONLY answers queries from containers in the user-defined network. The host's resolver, even if pointed at it, gets nothing. Verified by Docker's source: the embedded resolver lives in the container's netns. Reject.

**Make `decloud-<service>` a real DNS name on the host's resolver (`dnsmasq`/`unbound`).** Adds a host-local DNS server as a Decloud dependency. Bigger surface than C. Reject.

So Don's choice IS the right one. The list above belongs in the decision record (`_ai/decisions/caddy-runs-in-container.md`) — Raymond, when you write it, name and reject these too. "Why is Caddy in a container?" is exactly the question a future reviewer will ask, and "because every alternative is worse" is a more durable answer than "because Don decided so."

### 1.2 Editorial note on Don's §6 self-flagellation

Don, you flagged this in §6.1 ("we had this exact bug coming and didn't see it") and §6.2 ("the whole 'Caddy on the host' arc was an unforced error"). You're right. That said, I want to be specific about what was an unforced error and what was a reasonable trade:

- The architectural arc — Caddy on host vs. Caddy in container — was a reasonable trade at M1 design time. Containerising adds the volume-mount question, the bind-mount-source question, and the cross-namespace `caddy reload` question (which is exactly what we're spending this task on). Punting was defensible.

- The unforced error was NOT writing down "Caddy lives on the host; nothing else does; this asymmetry has implications" as a Decision in `_ai/decisions/`. The tech-plan §9.4 quote you pulled (`_tasks/2026-04-26-m1-implementation/03-tech-plan.md:784`) **identified the embedded-DNS gap** for the readiness probe and patched readiness via `Driver.ContainerIP`, but the realisation didn't propagate to Caddy because the thinking was inline in a tech plan, not promoted to a decision doc.

So the lesson is narrower than "we should never have put Caddy on the host." The lesson is: **when a tech plan corrects an assumption mid-stream, that correction is a Decision, and Decisions go in `_ai/decisions/` so future reviewers can audit "does my new code respect this?".** Ward, when you do the knowledge-librarian pass on this task, please add a note to that effect to the decisions-discipline doc.

OK, on to the real revisions.

---

## 2. Issues requiring revision

### Issue 1: The §11.3 readiness loop is wrong scope and probably wrong outright.

**Problem.** Joel proposes adding a 1-second poll loop inside `Manager.Up` to wait for Caddy's admin API after `docker run` returns. Five attempts at 200ms. The justification is "first deploy after `caddy up` could race the admin API startup."

**Why this is wrong:**

1. **It's a bug-prevention measure for a bug that doesn't exist yet.** Joel's own §11.3 calls out that `caddy:2` starts in sub-second time. The "slow disk on first run" hypothetical is not grounded in any observed failure. We are speculatively bloating `Manager.Up` against a problem that hasn't been measured.

2. **The race window doesn't fire where Joel says it does.** `decloud caddy up` brings Caddy up. The next thing the operator does is `decloud deploy service ...`, which involves a `docker build`, then `docker run` for the service, then a multi-second readiness probe, then registry save, THEN `caddy reload`. By the time we `docker exec ... caddy reload`, Caddy has had 5–60 seconds, not 200ms. The race is **architecturally pre-empted by the deploy flow itself.**

3. **If we DO add it, we add it in the wrong place.** The right place to retry a flaky `docker exec` is the reloader, not the manager — because it's the reloader that actually fails. Putting it in `Up` is solving a symptom.

4. **`_ai/explicit-inputs-not-globals.md` discipline.** The polling loop introduces a hard-coded 5-attempt × 200ms timing constant inside the manager. If it fails in the wild we have to plumb a flag through. Cheaper to not add it and respond if the failure ever materialises.

**Options:**

- **Option A (Cut it).** Drop §11.3 entirely. `Manager.Up` returns when `docker run` succeeds. If the first `caddy reload` ever races, we revisit then. The reloader's existing error message ("container not running; run 'decloud caddy up' first") is fine for the no-container case; we can add a "container running but admin API not ready" message under `isCaddyContainerMissing` if/when needed.
- **Option B (Keep it).** Joel's 1s poll loop in `Up`. Adds ~10 lines and one test. Cost: one more thing to maintain; one more place to wonder "is this magic timing right?"
- **Option C (Move it to the reloader as exec retry).** A tiny retry on `docker exec` failure with a "container starting" stderr signature. Saves operator from typing `decloud caddy reload` twice in the rare case it does race.

**My pick: Option A.** Don't pre-pay for a problem we haven't seen. If it ever fires, Option C is the right home for the fix.

**Don's decision required:** Cut the §11.3 readiness loop OR push back with evidence (a real failure observed in any environment, including the user's host).

---

### Issue 2: `--image` flag, Viper wiring, and TOML config are out of scope.

**Problem.** Joel proposes:
- A `--image` flag on `decloud caddy up`.
- Reading `caddy.image` from `/opt/decloud/config/decloud.toml` via Viper.
- This becomes "the FIRST place Viper is wired" in M1.

`_ai/decisions/m1-scope.md:18` explicitly says: **"NOT Viper — plain Cobra + `os.Getenv("DECLOUD_ROOT")` is three lines. M2 introduces Viper when there's a real `/etc/decloud/config.toml` to read."**

This task is a bug fix. We do NOT introduce Viper here. We do not introduce a TOML config here. The `caddy:2` default is fine. If an operator wants pinning, they `docker pull caddy:2.7.6 && docker tag caddy:2.7.6 caddy:2`, or they wait for M2.

Joel even flags this himself: "do NOT preemptively introduce a global `internal/config/viper.go`. We add it when there is a second consumer." The right reading of that principle is: **don't introduce Viper at all in this task.** One consumer is still preemptive when M2's whole charter is "introduce Viper for real."

**Options:**

- **Option A (Cut).** No `--image` flag. No Viper. No TOML key. Hard-code `const DefaultImage = "caddy:2"` in `internal/caddy/manager.go`. M2 introduces both Viper and the override knob together.
- **Option B (Keep flag, drop Viper).** `--image` flag only, defaulting to `"caddy:2"`. No config file integration. Cheaper than Option A in operator-ergonomics; still a small amount of code we'll re-do in M2.
- **Option C (Joel's full proposal).** Flag + Viper + TOML key.

**My pick: Option A.** This is a bug fix. The user is blocked. We do not need an image override to unblock them; `caddy:2` works. Adding the flag costs three test cases and a documentation paragraph; M2 will revisit anyway. Cut.

If the user objects to "no override at all," **Option B** is the fallback — a single CLI flag is cheap and doesn't violate the no-Viper rule.

**Don's decision required:** Confirm scope. Cut the `--image` knob and Viper wiring entirely (Option A) OR keep just the flag (Option B). Option C is rejected.

---

### Issue 3: `caddy reload` after `caddy down` semantics are underspecified — and the manager-reloader-deployer chain has a hidden coupling.

**Problem.** Joel's spec says `decloud caddy reload` after `decloud caddy down` returns exit 60 with "container not running; run 'decloud caddy up' first." Fine. But three things are not specified:

1. **What about `decloud deploy service` when Caddy is down?** Today, deploy succeeds up to the `caddy validate` step and then fails. After this fix, the failure point moves earlier (the first `docker exec` against a missing container). But the deploy has ALREADY built an image, run a container, passed readiness, and saved to the registry. We're now in a state where the registry has a service, the container is running, but Caddy isn't routing. The operator sees exit 60 and is left with a half-deployed state.

2. **The user request mentions HTTP/3 / UDP/443.** Joel addresses this in §3.2. Good. But the install doc rewrite (Raymond's task) needs to include UDP/443 in the firewall instructions if the user runs `ufw` or equivalent. Not mentioned anywhere.

3. **IPv6 publishing on the host.** The user's diagnostic shows the host has a public IPv6 (that's literally what triggered the bug). Caddy in container with `-p 80:80 -p 443:443` will bind to **IPv4 by default on most Docker setups**. If the operator's DNS has AAAA records pointing at the host (and the host is reachable on IPv6 — which the `2a03:f480:1:12::7b` evidence proves), then HTTPS over IPv6 will silently break. We need either `-p [::]:80:80 -p [::]:443:443` on hosts with IPv6, or explicit documentation that the bug-fix deploy is IPv4-only and IPv6 will land later.

   This is not theoretical. The user's host serves IPv6 today; their existing host-Caddy was almost certainly listening on `::` and the new container will listen on `0.0.0.0` only. **We are about to ship a regression.**

4. **Existing M1 docs lie about the network model in a way the migration won't catch.** `_docs/usage.md:182` says "Decloud deliberately does not publish container ports to the host (`docker run -p ...` is never invoked)." After this fix, `decloud caddy up` DOES `docker run -p ...`. Raymond's checklist mentions §6 in §10.2 but the wording change needed there is structural, not surface — the absolute "is never invoked" becomes "is never invoked for service containers; Caddy is the documented exception." Confirm this is part of the rewrite, not a footnote.

**Options for #1 (deploy-with-Caddy-down):**

- **Option A (Pre-flight check at the start of deploy).** `serviceDeployer.Deploy` calls `Manager.IsRunning` (or equivalent) first. If false, fail fast with a clear error, don't build, don't run. Adds one dependency (Manager) to the deployer.
- **Option B (Auto-bring-up Caddy from inside deploy).** Don rejected this in §3.1; I agree with the rejection.
- **Option C (Status quo — partial deploy state on Caddy-down).** Accept that an operator who runs `caddy down` then `deploy` ends up with a built-but-unrouted service. Document loudly in `_docs/usage.md` §7. Operator's recovery is `decloud caddy up && decloud caddy reload`, which works because the registry HAS the service.

**My pick: Option C with a clear error message change.** Don't add a pre-flight to the deployer (it's another coupling for the rare misuse case), but DO ensure the error message at exit 60 says: "the service was deployed and registered, but Caddy is not running; run `decloud caddy up` to route traffic." That converts a confusing partial-success into an actionable next step.

**Options for #3 (IPv6):**

- **Option A (Bind both stacks unconditionally).** `-p [::]:80:80/tcp -p 0.0.0.0:80:80/tcp -p [::]:443:443/tcp -p 0.0.0.0:443:443/tcp -p [::]:443:443/udp -p 0.0.0.0:443:443/udp`. Six port maps. Works on any host. Failure mode: if the host kernel doesn't have IPv6, Docker errors. Fine — failure is loud.
- **Option B (Detect at install time).** Operator runs `decloud caddy up`; we check if the host has a global IPv6 address; emit IPv6 port maps only when present. Adds complexity for one knob.
- **Option C (Document and punt).** IPv4 only for this fix; IPv6 binding is a follow-up task. Operator with IPv6 traffic gets IPv4-only HTTPS until then.

**My pick: Option A.** Unconditional dual-stack. Caddy is the public ingress; binding both IPv4 and IPv6 is what every "real" Caddy install does. The cost is three extra `PortMap` entries; the benefit is no IPv6 regression. Joel, push back if you have a concrete reason this fails — but absent that, ship dual-stack.

**Don's decision required:** Approve dual-stack ports (Option A) for IPv6, AND approve the deploy-with-Caddy-down recovery message change. The "register UDP/443 in the firewall doc" line is a Raymond/Kevlin nit, no decision needed.

---

## 3. Joel's open questions — my answers

Joel asked 8. Here are decisive answers:

1. **§11.3 readiness loop:** **CUT.** See Issue 1. No.
2. **§11.1 SELinux warning:** **NIT.** One sentence in the install doc: "On SELinux-enforcing hosts, you may need to relabel `/opt/decloud/config/caddy` (`chcon -Rt container_file_t`); we don't ship SELinux support in M1." Done. Don't gate with a flag.
3. **§3.1 Driver extension shape:** **APPROVED.** Three new methods (`ImagePull`, `Exec`, `RunWithOptions`), NOT bolted-on `RunRequest` fields. Joel's reasoning about test surface is correct. The asymmetry-breeds-bugs argument I'd normally use cuts the OTHER way here: making `RunRequest` polymorphic across two callers IS the asymmetry; two narrow request types is the symmetry.
4. **`decloud-caddy` constant location:** **APPROVED in `internal/caddy/manager.go`.** Joel and Don agree; I agree. `internal/ids` is for service-derived names. Caddy isn't a service.
5. **Don's §6.4 integration test:** **APPROVED REJECTION.** Joel's §13 argument is correct. M1 test strategy stands. Backlog item per §10.5. The bug we're fixing is exactly what M1 test strategy explicitly anticipated would slip through; the response is "fix the bug, log the integration test as M2's first item," not "expand scope now."
6. **Image float vs pin:** **N/A given Issue 2 above.** If the `--image` knob is cut (Option A of Issue 2), there's only the default; default is `caddy:2`. If Don keeps the flag (Option B), default is still `caddy:2`. Either way: float.
7. **`--restart unless-stopped` on Caddy:** **APPROVED unconditional.** Joel's right; one user's edge case isn't worth a flag.
8. **Stub Caddyfile written by `caddy up`:** **APPROVED.** Without it, the container starts and exits because no config file. Confusing as Joel says. Write the stub.

---

## 4. Decomposition sanity check

Joel's three-piece split — `Driver` extensions, `caddy.Manager`, reloader rewire — is correct. I considered two simpler shapes and rejected both:

**Simpler #1: Embed the Caddy lifecycle in the existing `serviceDeployer`.** Don't add a `Manager`. Make `Deploy()` ensure Caddy is running. Reject — couples deploy to ingress lifecycle, makes `decloud caddy up` impossible without a deploy, and forces the orchestrator to know about Caddy as a container vs. as a service-of-services. Joel's separation is right.

**Simpler #2: Skip the `Driver.Exec` method; have the reloader shell out directly.** Reject — breaks the pattern that all `docker` invocations go through `Driver`. Joel's argument that `Driver` should be the single mockable seam for Docker verbs is correct, and adding `Exec` there keeps `internal/caddy/reloader_test.go` clean.

**Simpler #3: Skip the `RunWithOptions` method; reuse `Run(RunRequest)` and conditionally include port/volume fields.** Reject — pollutes `RunRequest` for one caller. Joel's defensive `assert.Empty` argument is the right one.

So three pieces it is. The phase-ordering in §9 is also correct: Phase 1 (driver primitives) blocks Phases 2 and 3, both of which block Phase 4. Phases 2 and 3 are parallelisable. Ship it that way.

---

## 5. Other things to flag (these are also revision items)

### 5.1 Manager.Up rollback (§11.4) — agreed but expand.

Joel's §11.4 says: if `RunWithOptions` succeeds but a follow-up step fails, manager attempts `Stop`+`Remove` before returning the error. Good. **But Joel ALSO removed §11.3's readiness loop in my Issue 1, so the only follow-up step is "log success."** Without §11.3, there's no failure window between `Run` and "we're done." So §11.4's rollback test becomes vacuous.

**Action:** if we cut §11.3 (per Issue 1), also cut §11.4. There is nothing to roll back from. The whole compound issue evaporates. If Don keeps §11.3, keep §11.4.

### 5.2 The reloader's `cmdFactory` test seam.

Joel wants to keep `cmdFactory` "as a fallback for path-translation isolation." I disagree — once `Driver.Exec` is the seam, the `cmdFactory` is dead code. Path translation is pure-Go and tests itself without an exec. Cut `newCLIReloaderWithFactory` entirely; tests use the `MockDriver` for exec assertions and call `translatePath` directly for path tests. Less surface.

**Action:** delete `cmdFactory` from `cliReloader`. One fewer test seam.

### 5.3 Migration: M1.0 host Caddy could leak ACME state.

Joel's §7 acknowledges that ACME state at `/var/lib/caddy/.local/share/caddy/` is NOT migrated. The advanced `cp -a` recipe is documented. Fine. **But the install-doc rewrite (Raymond's §10.1) needs to call this out at the top, not in §3.1.** Operators who run `decloud caddy up` on a host that previously had host-Caddy will hit Let's Encrypt rate limits if they have many hostnames. The warning "first deploy will take an extra second to issue a fresh cert" understates the failure mode for an operator with 30 services.

**Action:** Raymond, the migration callout in §3.1 of the new install doc must include a hard "if you have more than ~5 hostnames, copy the ACME volume" warning, not an "extra second" softball. LE rate-limit recovery is 7 days.

### 5.4 The `is the tmp file path inside the bind mount?` contract.

Joel's §4.4 introduces `translatePath` with an "outside the bind-mount" error. Good. But the contract needs to be a doc-comment on `Reloader.Validate` AND `Reload`, not just sketched in the path-translation function. Future callers reading the interface should see the constraint without spelunking the impl.

**Action:** Joel/Rob, write the contract on the `Reloader` interface methods, not just the impl.

### 5.5 The `_docs/usage.md:182` lie needs explicit correction, not patch.

Per Issue 3 #4 above. Today's `usage.md` says "`docker run -p ...` is never invoked." Tomorrow that's false. Raymond's checklist §10.2 §6 needs to rewrite that paragraph, not paper over it.

### 5.6 `NetworkName` constant deduplication.

Joel's §5 introduces `caddy.NetworkName = "decloud"` and notes the existing six literals scattered across `service.go`, `cli_driver.go`, etc. He proposes deferring the cleanup to M1.x. **I agree on the deferral, but require the new code use the constant.** Not "don't introduce a seventh literal" — that's already the rule. The new manager and the new reloader both reference `caddy.NetworkName` (or `internal/networks` if you want a package — Joel's call). Lock it in for the new code so the M1.x cleanup is mechanical.

### 5.7 The decision-doc-as-load-bearing point.

Don's §6.1 is right that `_ai/decisions/caddy-runs-in-container.md` matters. Make sure Raymond writes it BEFORE the implementation lands, not after — so reviewers (especially Kevlin's hallucination check) have it as ground truth during the implementation review, not as a retrospective. **Sequence the work so Raymond's decision-doc draft is part of Phase 6 but reviewed during Phase 4 alongside the CLI surface, not after Phase 7.**

---

## 6. Migration adequacy

Don's "stop host Caddy, run `decloud caddy up`" is sufficient for the simple case. The cases that bite:

1. **ACME state migration (covered above, §5.3).** Needs hard warning, not soft.
2. **Port 80/443 still bound after `systemctl disable --now caddy`.** If the operator forgets `apt-get remove`, the unit might come back on next boot. Joel's error text "ports already in use; run `systemctl disable --now caddy`" is necessary but **not sufficient** — the operator needs `apt-get remove caddy` OR `systemctl mask caddy` to make the disable persistent. Add to the error text or the migration doc.
3. **The install doc's "Caddy fails to start until the Caddyfile exists" footgun.** `_docs/install.md:61-62` says today: "Caddy will fail to start until the Caddyfile exists. The first `decloud deploy service` writes a stub Caddyfile, after which `systemctl start caddy` succeeds." After this fix, that paragraph is obsolete. The new flow is: `decloud caddy up` writes the stub AND brings Caddy up. **Raymond, this paragraph must be DELETED in the rewrite, not just edited.** It's the most-likely place a hasty rewrite leaves a stale instruction.

The fix is **purely additive** w.r.t. existing M1 code — no service-deploy logic regresses. Confirmed by reading `service.go` and `lifecycle.go`. The reloader's constructor signature change is the only breaking interface change, and it's contained inside `internal/cli/deploy_service.go`. Joel's §2.3 list is correct.

---

## 7. Test strategy

Rejecting the integration test for THIS task is correct. `_ai/decisions/m1-test-strategy.md` §1 explicitly anticipates this exact bug class. Joel's §13 argument is unimpeachable. Backlog item #6 in `_ai/m1x-backlog.md` is the right home.

**One additional test I want.** Joel listed `TestReloader_PathTranslationOutsideBindMount` (passes `/tmp/foo`, asserts error mentions "outside the bind-mount"). Add a positive case: `TestReloader_PathTranslationCanonicalForm` — pass `/opt/decloud/config/caddy/Caddyfile.tmp`, assert translated to `/etc/caddy/Caddyfile.tmp` exactly. The negative-only test is half the contract.

No docker-compose smoke. We have neither the infrastructure nor the budget for it, and one-off compose files rot fastest of all test harnesses. M2's integration-test introduction is the right venue.

---

## 8. Scope: should we ship a smaller fix now, full containerisation later?

I considered this. There is no smaller fix that actually works.

The candidates for "smaller":
- Ship a doc fix telling operators to run `iptables`/host DNS. Not a fix; it's pushing the bug to the operator.
- Patch the generator to emit IP literals with a `caddy reload` after every container restart. Don's Candidate A; correctly rejected.
- Publish ports. Don's Candidate B; correctly rejected.

The only thing smaller than Candidate C IS Candidate A or B. Both are wrong. Candidate C is the smallest correct fix.

**Scope IS right.** Don and Joel correctly recognised this is a bug fix that requires architectural movement. The architectural movement is small (~600 LOC including tests, by Joel's reckoning) and isolated (driver, manager, reloader, CLI, docs). It does NOT bleed into deploy logic, registry, env capture, or readiness. The blast radius is bounded.

---

## 9. Non-blocking nits

For the record. Don/Joel, you don't need to do anything with these unless you want to.

- Joel's §15 "57-hour Joel's-π adjustment" is funny and accurate. Keep the joke.
- The two-line stdout shape on `caddy up` (Joel §1.2) is fine. Don't bikeshed it; ship it.
- Joel's §1.5 error-text table is well done; harvest it as the spec for `internal/cli/exit_codes.go`'s mapping behaviour test.
- "`docker exec` against a not-yet-ready container" (§11.3) — already covered in Issue 1.
- The §5 deduplication of `"decloud"` literal — covered in §5.6.
- The Caddy admin API on `localhost:2019` not being host-published (§11.8) — yes, correct, lock in with a doc-comment as Joel says.

---

## 10. Verdict

**REVISIONS REQUIRED.**

The required changes:

1. **Cut §11.3** of Joel's tech plan (the `Up` readiness loop). Either remove the section outright (Option A of Issue 1), or push back with evidence of a real failure. My pick: cut.

2. **Cut §11.4** consequently (rollback after partial `Up` failure becomes vacuous if §11.3 is cut). Conditional on #1.

3. **Cut the `--image` flag, Viper wiring, and TOML config integration** (Issue 2 Option A). Hard-code `caddy:2` as `caddy.DefaultImage` in `internal/caddy/manager.go`. M2 introduces all overrides + Viper together. If Don wants the bare flag back as a single concession (Option B), document why. **No Viper either way for this task.**

4. **Add dual-stack IPv6 port publishing** (Issue 3 Option A): `-p [::]:80:80/tcp -p 0.0.0.0:80:80/tcp -p [::]:443:443/tcp -p 0.0.0.0:443:443/tcp -p [::]:443:443/udp -p 0.0.0.0:443:443/udp`. Six entries in the Caddy `RunOptions.Ports` slice. Test for the dual-stack shape.

5. **Update the deploy error message** for the "service deployed but Caddy down" case (Issue 3 Option C) to say "service registered; run `decloud caddy up` to route traffic." This is a one-line change in the wrap-text where `ErrCaddyReload` gets returned in `service.go`.

6. **Delete `cmdFactory` from `cliReloader`** (§5.2). Test seam is now `MockDriver.Exec` exclusively. One fewer surface.

7. **Strengthen the migration callout** in `_docs/install.md` §3.1 (§5.3): hard warning about ACME state for hosts with many hostnames, including the `cp -a` recipe inline (not just in the decision doc). Add `apt-get remove` or `systemctl mask` to the error text in §1.5 of Joel's plan.

8. **Doc-comment the Reloader contract** about the bind-mount path constraint on the interface methods, not just in the impl (§5.4).

9. **Use `caddy.NetworkName`** (or equivalent) constant from new code; do NOT introduce a seventh `"decloud"` literal (§5.6). Existing literal cleanup stays in M1.x backlog.

10. **Sequence Raymond's decision-doc draft** to land in Phase 6 reviewed alongside Phase 4, not after Phase 7 (§5.7).

When Joel revises with these in, send the revised tech plan around again. The bones are right; we're trimming and tightening, not redesigning.

—

**Non-blocking but encouraged:** Add the positive path-translation test (`TestReloader_PathTranslationCanonicalForm`) per §7. Add the IPv6 dual-stack test per #4 above (`TestCLIDriver_RunWithOptionsDualStackPorts`). And Raymond, please make sure `_docs/install.md:61-62` is **deleted** in the rewrite, not edited (§6 #3).

— Linus
