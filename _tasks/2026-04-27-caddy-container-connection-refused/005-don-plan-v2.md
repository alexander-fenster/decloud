# 005 — Don's Plan v2: Containerise Caddy on the `decloud` network

Author: Don Melton (planning agent)
Date: 2026-04-27
Status: PLAN v2. Standalone document. Supersedes `002-don-plan.md`. Awaiting Joel's tech expansion v2 + Linus re-review.

## 0. How to read this document

This is a complete plan, not a diff. v1 (`002-don-plan.md`) and Joel's v1 tech plan (`003-joel-tech-plan.md`) gave Linus a fix that was substantially right — diagnosis correct, Candidate C correct, three-piece decomposition correct — but had three speculative scope additions and one shipped-regression-in-disguise. Linus's review (`004-linus-review.md`) demanded seven concrete revisions. v2 applies them all and is what Joel's v2 tech plan should expand against.

Section 11 ("Revisions applied") at the end maps each Linus item to a section in this document so the re-review is mechanical.

---

## 1. TL;DR

Caddy is generating a Caddyfile that says `reverse_proxy decloud-durak-live:10001`. That line only resolves under Docker's embedded DNS (`127.0.0.11`), which is only available to containers attached to a user-defined Docker network. But the M1 install puts Caddy on the host as a systemd unit. So Caddy resolves the upstream name through the host's resolver, which returns the host's public IPv6, and Caddy dials a port that nothing is bound to. Result: `connection refused`.

Fix: **Caddy runs as a Docker container named `decloud-caddy` on the `decloud` network, owned and managed by Decloud, brought up by a new `decloud caddy up` command.** That is the only way the existing Caddyfile means what it says. The reloader switches from shelling a host `caddy` binary to `docker exec`'ing into the `decloud-caddy` container.

Scope is bounded: driver primitives (`ImagePull`, `Exec`, `RunWithOptions`), a new `caddy.Manager`, the reloader rewire, two new CLI commands (`caddy up`, `caddy down`), and the doc rewrite. Generator output is unchanged. Service-deploy logic is unchanged. Registry, env capture, readiness — all unchanged.

---

## 2. Root cause — proof, not opinion

### 2.1 The Caddyfile assumes embedded Docker DNS

`internal/caddy/generator.go:43-46` writes `reverse_proxy <ContainerName>:<Port>` per host. `ContainerName` is `decloud-<service>` (`internal/caddy/generator.go:65-68`, mirrors `ids.ContainerName(req.Name)` at `internal/deploy/service.go:158`). That short name is **only** resolvable by Docker's embedded DNS server, which **only** runs inside containers that are members of a user-defined bridge network.

### 2.2 The service container IS on the `decloud` network — proven

`internal/deploy/service.go:131-134` calls `Driver.NetworkEnsure(ctx, "decloud")`. `internal/deploy/service.go:187-194` runs the container with `Network: "decloud"`. `internal/dockerdrv/cli_driver.go:46-50` builds `docker run --network decloud ...`. The user's `docker network inspect decloud` confirms `decloud-durak-live` at `172.18.0.2/16`.

### 2.3 Caddy is NOT on the `decloud` network — proven by absence

- `internal/caddy/reloader.go:25-27` constructs `cliReloader{cmd: exec.CommandContext}`. It shells out to a host `caddy` binary on `PATH`. No `docker exec`, no `docker run`, no `docker network connect`.
- `internal/cli/caddy_reload.go` exposes only `decloud caddy reload`. There is no `caddy up`, no `caddy install`.
- `_docs/install.md:30-52` instructs the operator to write a host systemd unit. The unit has no `docker network connect` step, no `BindsTo=docker.service`, no Docker side-channel of any kind.
- `_docs/install.md:96-100` says "create the shared Docker network so Caddy can reach upstreams by container name" — but does not connect Caddy to it. Caddy is not even mentioned in §5.

### 2.4 Why the dial address is the host's public IPv6

`docker network inspect decloud` shows `EnableIPv6: false`. The container has no IPv6. The dial target `[2a03:f480:1:12::7b]:10001` is the host's own public IPv6 — `2a03:f480::/29` is a Hetzner GUA prefix. Caddy's resolver returned the host's AAAA (via wildcard or search-domain match), Go's `net.Dial` preferred AAAA over the absent A, and the connection refused because nothing on the host is bound to `:10001`. The exact resolver path doesn't matter for the fix; what matters is that any path that is not Docker's embedded DNS returns a wrong answer here.

### 2.5 We had this exact bug coming and didn't see it

The M1 tech plan (`_tasks/2026-04-26-m1-implementation/03-tech-plan.md:784`) explicitly identifies that the decloud host process can't resolve container names via Docker DNS, and patched the readiness probe via `Driver.ContainerIP` to dial the bridge IP directly. That fix did not propagate to Caddy because nobody asked "where does the Caddy host binary's name resolution go?" The lesson, per Linus §1.2, is narrower than "we should never have put Caddy on the host" — it is: **when a tech plan corrects an assumption mid-stream, that correction is a Decision, and Decisions go in `_ai/decisions/` so future reviewers can audit "does my new code respect this?"** Ward will capture this in the knowledge pass.

---

## 3. The fix — Candidate C

### 3.1 Three candidates considered; C is the only one that works

**Candidate A (REJECTED) — keep host Caddy, write IPs into the Caddyfile.** Bridge IPs are not stable across container restarts; hardcoding them forces a Caddy reload after every recreate and introduces a race. The generator gains no useful capability; it loses the auto-failover Caddy+Docker-DNS gives us. Papers over the architectural smell.

**Candidate B (REJECTED) — keep host Caddy, publish container ports to the host with `-p`.** Explicitly contradicts `_docs/usage.md:181-192` ("the port is NOT exposed via `-p` because Caddy reaches the container over the shared network"). Forces port-collision management in the deployer. Same architectural smell, different paint.

**Candidate C (CHOSEN) — run Caddy as a Decloud-managed container on the `decloud` network.** Decloud owns Caddy. Caddyfile means what it says. Reverse proxy resolves via embedded DNS to the bridge IP. Connection succeeds. This is also the design the M1 team clearly assumed without writing it down.

Linus §1.1 enumerated the alternatives Don missed (`host.docker.internal`, `--network host`, `--network container:`, sidecar, `/etc/hosts` injection, `--resolvers 127.0.0.11`, host-local `dnsmasq`); each was rejected with reasoning. Raymond will move that enumeration into `_ai/decisions/caddy-runs-in-container.md` so it is the durable answer to "why is Caddy in a container?"

### 3.2 What changes, concretely

**Files added:**

- `internal/caddy/manager.go` — new `Manager` interface (`Up`, `Down`, `IsRunning`) plus production impl on top of `dockerdrv.Driver`.
- `internal/caddy/manager_test.go` — gomock-against-Driver tests.
- `internal/caddy/mocks/mock_manager.go` — generated.
- `internal/cli/caddy_up.go`, `internal/cli/caddy_down.go` — Cobra commands.
- `internal/cli/caddy_up_test.go`, `internal/cli/caddy_down_test.go` — CLI tests.
- `_ai/decisions/caddy-runs-in-container.md` — decision record (Raymond writes; reviewed alongside Phase 4 per Linus §5.7).

**Files modified:**

- `internal/dockerdrv/driver.go` — extend the interface with `ImagePull`, `Exec`, `RunWithOptions`. Keep existing `RunRequest`/`Run` for service deploys.
- `internal/dockerdrv/cli_driver.go` — implement the three new methods.
- `internal/dockerdrv/cli_driver_test.go` — argv-shape tests for the three new methods.
- `internal/dockerdrv/mocks/mock_driver.go` — regenerated.
- `internal/caddy/reloader.go` — `cliReloader` now invokes `docker exec decloud-caddy caddy ...` via `Driver.Exec`. Translates host paths to container paths. Constructor takes `Driver` and the host Caddyfile dir. **Delete the legacy `cmdFactory` test seam** (per Linus §5.2 / revision #6).
- `internal/caddy/reloader_test.go` — drive a `MockDriver`. Drop the host-`caddy` argv tests; add the new `docker exec` argv tests, the path-translation positive case, the path-translation negative case, and the "container not running" actionable-error test.
- `internal/cli/deploy_service.go` — `buildProductionLifecycle` and `buildProductionDeployer` pass the `Driver` and host Caddy dir into `caddy.NewCLIReloader`. Add `caddyManagerFactory` test seam.
- `internal/cli/root.go` — mount `caddy up` and `caddy down` under the existing `caddy` parent command.
- `internal/cli/exit_codes.go` — map `caddy.ErrCaddyUp` and `caddy.ErrCaddyDown` to `ExitRunFail` (40). No new constants.
- `internal/deploy/service.go` — update the `ErrCaddyReload` wrap text in `regenerateAndReload` so the operator's recovery path is one command, not investigation (per Linus revision #5; see §6.5).
- `_docs/install.md` — §3 rewritten; §5 cross-reference updated; **§61-62 paragraph DELETED, not edited** (per Linus §6 #3).
- `_docs/usage.md` — §1 quick start gains `decloud caddy up`; §6 has its "`-p` is never invoked" sentence rewritten to make Caddy the documented exception; §7 reload-recovery updated for `docker exec`.

**Files unchanged:**

- `internal/caddy/generator.go` and `internal/caddy/stub.go` — output is correct under the new architecture.
- `internal/deploy/service.go` outside the wrap-text update — orchestration logic, transaction shape, registry interaction unchanged.
- `internal/deploy/lifecycle.go`, `internal/deploy/readiness.go` — readiness already goes through bridge IP and doesn't depend on Caddy.
- `internal/registry/*`, `internal/ids/*`, `internal/envcap/*` — unchanged.

---

## 4. Architectural decisions in v2 (NEW or HARDENED from v1)

### 4.1 No `--image` flag, no Viper, no TOML config (Linus revision #3)

`_ai/decisions/m1-scope.md:18` is explicit: **"No Viper — plain Cobra + `os.Getenv("DECLOUD_ROOT")` is three lines. M2 introduces Viper when there's a real `/etc/decloud/config.toml` to read."** This task is a bug fix. The user is blocked. The `caddy:2` default works. We do not need an image override to unblock the user.

Concretely:

- `const DefaultImage = "caddy:2"` lives in `internal/caddy/manager.go` and is the only image source. No flag, no env var, no TOML key.
- `internal/cli/caddy_up.go` does **not** pull in `viper`. It does not read any config file. The Cobra command takes zero flags.
- The decision record and the install doc note that an operator who wants a pinned tag can `docker pull caddy:2.7.6 && docker tag caddy:2.7.6 caddy:2` as a workaround. M2 introduces Viper-driven overrides for this and every other config knob in one cohesive pass.

This was Linus's Issue 2, Option A. Joel's v1 §1.2 / §11.7 / §12.6 all collapse to "no override exists in this task." Joel's v2 tech plan should remove every reference to `--image`, `caddy.image`, and `resolveCaddyImage`.

**Tradeoff named honestly:** an operator who needs a pinned tag *today* (e.g., `caddy:2.7.6` is broken in a way `caddy:2` isn't) has to retag locally instead of passing `--image`. That cost is one shell command per host. The benefit is not violating the M1-scope decision and not paying for Viper integration twice (now and again in M2). Trade is right.

### 4.2 No readiness-poll loop in `Manager.Up` (Linus revisions #1 and #2)

Joel's v1 §11.3 proposed a 1-second poll loop after `docker run` to wait for Caddy's admin API. **Cut.** Reasons (Linus Issue 1):

1. It is a fix for a problem nobody has measured. `caddy:2` starts in sub-second time.
2. The race window doesn't fire in practice. `decloud caddy up` is followed by `decloud deploy service`, which spends 5–60 seconds on `docker build`, `docker run` for the service, and a multi-second readiness probe before it ever calls `caddy reload`. Caddy's admin API has had ample warm-up.
3. Even if the race did fire, the right place to retry a flaky `docker exec` is the reloader, not the manager. Putting it in `Up` solves a symptom in the wrong layer.
4. Adding a 5-attempt × 200ms timing constant in `Up` introduces a magic-number knob with no flag plumbing — the kind of "is this magic timing right?" thing we then have to remove or parameterise later.

`Manager.Up` returns when `Driver.RunWithOptions` returns. No probe. If, in the wild, a real failure ever surfaces, the right fix is a tiny retry on the **reloader's** `docker exec` path with a "container starting" stderr signature (Linus's Option C). Not in this task.

**Consequence: rollback in `Up` is also cut (revision #2).** Joel's v1 §11.4 proposed a `Stop`+`Remove` rollback if a follow-up step inside `Up` failed. With §11.3 gone, there is no follow-up step between `Run` and "we're done." The rollback becomes vacuous and the test (`TestManager_UpRollsBackOnPostRunFailure`) is dropped before it's written. `Manager.Up`'s state machine is:

```
1. NetworkEnsure
2. WriteStubIfMissing
3. Inspect:
     running  -> log "caddy already running"; return nil
     exited   -> Driver.Start; log "caddy started"; return nil
     absent   -> ImagePull; RunWithOptions; log "caddy up"; return nil
```

If any step fails, return the wrapped error. No cleanup. Operator re-runs `decloud caddy up` after fixing the underlying problem — and on re-run, an inconsistent partial state (e.g., a leftover `decloud-caddy` container in some weird state) is detected by the existing `Inspect` switch and either reused (`exited` -> `Start`) or short-circuited (`running` -> no-op).

**Tradeoff named honestly:** in the unlikely scenario where `Run` succeeded but the operator's next action depends on Caddy being not just `running` but admin-API-ready, the operator may need to wait or retry once. Acceptable — and as argued in #2 above, the deploy flow itself absorbs that wait.

### 4.3 Dual-stack IPv6 port publishing (Linus revision #4)

The user's host has a public IPv6 address (`2a03:f480:1:12::7b` — that is literally what triggered the bug). Their existing host-Caddy was almost certainly listening on `::` for both v4 and v6. If we ship Caddy-in-container with `-p 80:80 -p 443:443`, Docker binds to `0.0.0.0` only on most defaults, and HTTPS over IPv6 silently breaks. **We are not shipping an IPv6 regression on a host whose IPv6 just exposed our last bug.**

`Driver.RunWithOptions` for `decloud-caddy` publishes six port maps:

```
-p 0.0.0.0:80:80/tcp
-p [::]:80:80/tcp
-p 0.0.0.0:443:443/tcp
-p [::]:443:443/tcp
-p 0.0.0.0:443:443/udp
-p [::]:443:443/udp
```

`/udp` for HTTP/3 over QUIC; if we don't open UDP/443, HTTP/3 silently degrades and operators see "TLS works but mobile is slow" reports we can't diagnose.

**Tradeoffs named honestly:**

1. **Cost on hosts without IPv6.** If the kernel has IPv6 disabled (`net.ipv6.conf.all.disable_ipv6=1`), `docker run -p [::]:80:80` errors at startup. Failure is loud and the message ("listen tcp [::]:80: socket: address family not supported by protocol") is recognisable. This is acceptable: it's a one-time install error, the operator removes IPv6 disablement or files an issue, and the install doc names the failure mode. We do NOT add a host-IPv6-detection step in `caddy up`; complexity-for-one-knob is exactly the kind of speculative wiring Linus revision #1 cuts elsewhere.

2. **Six PortMap entries vs. three.** The argv gets longer; the test asserting the exact shape gets six lines instead of three. Negligible.

3. **The install doc must mention that UDP/443 needs to be open in any host-level firewall (`ufw`, `firewalld`).** Raymond's checklist (§7.2) names this explicitly.

This is Linus Issue 3 Option A. Joel's v2 tech plan should bake the dual-stack `RunOptions.Ports` shape into the manager's `Up` and into `TestCLIDriver_RunWithOptionsCaddyShape`. Add a dedicated `TestCLIDriver_RunWithOptionsDualStackPorts` test (Linus §10 non-blocking-but-encouraged) to lock the shape independently of the Caddy-specific test.

### 4.4 Deploy-failure error text gives a one-command recovery (Linus revision #5)

**The failure case:** Operator runs `decloud caddy down`, then `decloud deploy service`. The deployer builds the image, runs the service container, passes readiness, saves to the registry — and then `Caddyfile` regeneration fails because `docker exec decloud-caddy caddy validate` errors with "no such container." Today, the operator sees a generic exit-60 error wrapped as `ErrCaddyReload`, doesn't know the service IS registered, doesn't know how to recover, and may try to `decloud deploy service` again or — worse — manually clean up the container.

The fix: when `regenerateAndReload` returns an error and the underlying cause is "Caddy is not running," the wrap text in `internal/deploy/service.go:314-322` must say:

```
service registered and container running, but Caddy is not routing traffic;
run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to restore routing
```

Detection: the reloader returns the actionable error ("container 'decloud-caddy' is not running; run 'decloud caddy up' first") via `Driver.Exec` returning `ErrContainerNotFound` from a container-not-running condition. The deployer doesn't need to add a new sentinel — the reloader's own error text is preserved through `%w` wrapping. The change is a one-liner in the reloader's `execCaddy` (it already knows when the container is missing) plus a doc-comment on `Reloader.Validate`/`Reload` clarifying that "container missing" is a recognised, operator-actionable failure mode.

**No pre-flight check at the start of `Deploy`** (Linus Issue 3 Option C, not Option A). Adding a `Manager.IsRunning` call to the start of every deploy adds a coupling for a rare misuse case and slows the common path. Status quo + clear error message is the right tradeoff.

**Where the new error text lives:**

- `internal/caddy/reloader.go::execCaddy` already produces "container ... is not running; run 'decloud caddy up' first" when `Driver.Exec` returns `ErrContainerNotFound` (Joel v1 §4.4).
- `internal/deploy/service.go:314-322` already wraps with `ErrCaddyReload`. Update the wrap text on the validate/reload error legs to read:
  - On the validate leg: `fmt.Errorf("%w: caddy validate failed: %w; service is registered and running but Caddy is not routing", ErrCaddyReload, err)`.
  - On the reload leg: `fmt.Errorf("%w: caddy reload failed: %w; service is registered and running but Caddy is not routing", ErrCaddyReload, err)`.
- Joel's v2 plan should specify the exact final wording. The principle: any error path that returns from `regenerateAndReload` with the registry already saved must tell the operator (a) the service IS registered, and (b) the one command to run to fix it.

**Tradeoff named honestly:** the error text gets longer. CLI users who scrape exit codes are unaffected (still 60). Users who read text get useful guidance instead of a stack trace. Net positive.

### 4.5 Delete the `cmdFactory` test seam from `cliReloader` (Linus revision #6)

Joel v1 §4.4 kept `cmdFactory` as a fallback for "path-translation isolation." Linus rejected that: once `Driver.Exec` is the seam, `cmdFactory` is dead code. Path translation is pure Go and tests itself without an `exec`.

Concretely:

- `cliReloader` has one struct field for IO: `driver dockerdrv.Driver`. No `cmd cmdFactory`.
- `NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader` is the only constructor.
- `newCLIReloaderWithFactory` is deleted.
- Tests that exercised `cmdFactory` to capture argv are rewritten against `MockDriver.Exec`.
- The path-translation function is testable directly: `TestReloader_PathTranslationCanonicalForm` (positive case, Linus §7) and `TestReloader_PathTranslationOutsideBindMount` (negative case, Joel v1 §8.2).

One fewer test seam. One fewer place future-Don has to wonder "why are there two ways to mock this?"

### 4.6 Strengthened ACME-state migration warning (Linus revision #7)

Joel v1 §7 said "first deploy will take an extra second to issue a fresh cert." That softball understates the risk for an operator with many hostnames. Let's Encrypt's rate limits include:

- **50 certificates per registered domain per week.** An operator with 30 services on subdomains of a single registered domain has 30 first-issuances queued; well under 50, so they're fine.
- **5 duplicate certificates per week per identical SAN set.** Mass migration of services that previously held identical SANs (rare for Decloud's per-service-hostname model, but possible) hits this.
- **300 new orders per account per 3 hours.** Recovery window is 3 hours, not 7 days.
- **Failed-validation rate limits and account-level rate limits** can compound.

The headline number that bites operators most is **the 7-day recovery window for the per-domain weekly cap** if they trip it. Telling someone "extra second" when the worst case is "your TLS is broken until next Tuesday" is wrong.

**v2 stance:** the **default migration path** in the install doc is the volume-copy recipe, not the cold restart. The install doc presents it that way:

```
RECOMMENDED MIGRATION (preserves ACME state — DO THIS unless you have only 1-2 hostnames):

  docker volume create decloud_caddy_data
  docker run --rm \
    -v /var/lib/caddy/.local/share/caddy:/from \
    -v decloud_caddy_data:/to \
    alpine sh -c 'cp -a /from/. /to/'
  decloud caddy up

ALTERNATIVE (cold restart — only if you have 1-2 hostnames or no production traffic):

  decloud caddy up
  # First request per hostname will pause for ~1-3 seconds while Caddy
  # obtains a fresh Let's Encrypt cert. With many hostnames you risk
  # tripping LE rate limits, which can take up to 7 days to recover from.
```

Inline in the install doc, not buried in the decision record. The decision record (`_ai/decisions/caddy-runs-in-container.md`) gets the full rationale; the install doc gets the recipes operators actually run.

Also: the "ports already bound" error text from Joel v1 §1.5 must include the persistent-disable step, because `systemctl disable --now caddy` does NOT prevent the unit from being re-enabled by a package upgrade (Linus §6 #2). New error text:

```
caddy up: ports 80/443 already in use; if you ran the M1.0 install, run
  'systemctl disable --now caddy && systemctl mask caddy'
or
  'apt-get remove -y caddy'
to make the change persistent.
```

**Tradeoff named honestly:** the install-doc migration section gets longer (~20 lines instead of ~5). That is the correct length for instructions whose failure mode is a multi-day TLS outage.

---

## 5. Edge cases — name them all upfront

Most of v1's §3 stands. Updated entries:

### 5.1 First-deploy ordering (UNCHANGED from v1)

Operator runs `decloud caddy up` once after install. `Manager.Up` ensures the network, writes the stub Caddyfile if missing, runs the container. First `decloud deploy service` proceeds; `Reloader.Validate`/`Reload` `docker exec` into the running Caddy container.

**Edge case: operator forgets `decloud caddy up`.** `caddy validate` step in deploy fails. Error text per §4.4. Exit 60. Operator runs `decloud caddy up`, then `decloud caddy reload`. Recovered.

**Edge case: `caddy up` called when Caddy is already running.** Idempotent: `Inspect` shows running, log "caddy already running", return 0.

### 5.2 Network not created yet (UNCHANGED)

`caddy up` calls `Driver.NetworkEnsure(ctx, "decloud")` first. Creates the network if absent. The install doc keeps the explicit "create the network" step because doing it twice is harmless and operators like to verify state.

### 5.3 Port 80/443 already bound (HARDENED per §4.6)

If migrating from M1.0 host-Caddy, `caddy up` fails with the new error text including `systemctl mask` or `apt-get remove` — not just `disable --now`. Persistent-disable is the actual fix; the soft `disable` lasts until the next package upgrade.

### 5.4 ACME state must survive container replace (HARDENED per §4.6)

Named volumes `decloud_caddy_data:/data` and `decloud_caddy_config:/config`. **`decloud caddy down` does NOT remove the volumes.** Operators who want a full nuke `docker volume rm` themselves. Documented loudly in `caddy down`'s help text and in `_docs/usage.md`.

### 5.5 `decloud caddy down` semantics (UNCHANGED)

Stop with 10s grace, remove the container, leave volumes. Idempotent on absent container.

### 5.6 Caddyfile path translation across the bind mount (UNCHANGED)

Host path `/opt/decloud/config/caddy/Caddyfile`. Container path `/etc/caddy/Caddyfile`. Bind is the directory, not the file, so the deployer's atomic-rename trick (write `.tmp`, validate, rename — `internal/deploy/service.go:310-318`) works. Reloader translates host paths to container paths via `translatePath` and rejects paths outside `hostCaddyDir` with a clear error. Contract documented on `Reloader.Validate` and `Reload` interface methods (Linus §5.4).

### 5.7 Idempotency (UNCHANGED)

- `caddy up` already running → no-op.
- `caddy up` exited → `Start`.
- `caddy up` image needs updating → out of scope; operator runs `caddy down && docker pull caddy:2 && caddy up`.
- `caddy down` already absent → no-op success.
- `caddy reload` after `caddy down` → exit 60 with the actionable error per §4.4.

### 5.8 EnableIPv6 on the `decloud` Docker network (UNCHANGED)

Network was created without `--ipv6`. Docker's embedded DNS only returns A records on an IPv4-only network. The IPv6-fallthrough bug literally cannot recur for upstream resolution. **Distinct from §4.3:** dual-stack publishing in §4.3 is about the host listener for *inbound* traffic, not Docker's internal DNS for *upstream* lookup. Two different IPv6 stories; they don't conflict.

### 5.9 SELinux (NIT, per Linus §3.2)

On RHEL-family hosts with SELinux enforcing, bind-mounting `/opt/decloud/config/caddy` requires `chcon -Rt container_file_t /opt/decloud/config/caddy` (or a `:Z` mount option, which we don't use). One-line warning in the install doc; not a flag, not a M1 supported configuration.

### 5.10 Concurrent deploys racing the reloader (UNCHANGED, M2+ concern)

Two simultaneous deploys both write `Caddyfile.tmp`, both validate, both rename — last-rename-wins, then both reloads. Caddy handles back-to-back reloads correctly. No issue in single-operator M1; flag for M2+ when multi-operator becomes plausible. Note in the decision record.

---

## 6. Tests

Per CLAUDE.md: Testify, Gomock, no change-detector tests. No integration tests in this task (Linus §7 confirms M1 test strategy stands).

### 6.1 `internal/caddy/manager_test.go` (new)

Use `gomock.InOrder` per `_ai/gomock-inorder-sequencing.md`. Mock `dockerdrv.Driver`. Drive a real `cliManager`; assert through the recorder.

| Test | Asserts |
|---|---|
| `TestManager_UpFreshInstall` | `NetworkEnsure` → `Inspect`(absent) → `ImagePull` → `RunWithOptions` with **exact** dual-stack `RunOptions` shape (name, network, restart, six PortMap entries per §4.3, three volume mounts, label). |
| `TestManager_UpAlreadyRunning` | `NetworkEnsure` → `Inspect`(running). No `ImagePull`, no `Run`, no `Start`. Stdout contains `caddy already running`. |
| `TestManager_UpAfterPriorStop` | `NetworkEnsure` → `Inspect`(exited) → `Start`. No `ImagePull`, no `Run`. |
| `TestManager_UpNetworkEnsureFails` | Mock returns sentinel; manager wraps as `ErrCaddyUp` AND chains the inner error (`%w: %w` discipline; `errors.Is` on both). |
| `TestManager_UpImagePullFails` | Same wrap shape on the pull leg. |
| `TestManager_UpStubWriteFailsWrappedAsCaddyUp` | Pre-create CaddyfilePath as a directory or with bad mode; assert `Up` returns `ErrCaddyUp` wrapping the FS error. |
| `TestManager_DownHappyPath` | `Stop` → `Remove`. Order locked. |
| `TestManager_DownContainerAbsent` | `Stop` returns `ErrContainerNotFound` → `Remove` returns `ErrContainerNotFound` → `Down` returns nil. |
| `TestManager_DownStopFailsHard` | Non-`ErrContainerNotFound` from `Stop` → wrapped `ErrCaddyDown`, `Remove` not called. |
| `TestManager_IsRunningTrueFalseAbsent` | Sub-cases for `running` → true; `exited` → false; `absent` → false. |

**Tests NOT included** (per §4.2): no `TestManager_UpRollsBackOnPostRunFailure`, no `TestManager_UpReadinessProbe*`. These existed in Joel v1 §11.3-11.4 and are vacuous after the cuts.

### 6.2 `internal/caddy/reloader_test.go` (rewritten)

Drop the existing host-`caddy` argv tests (`TestReloader_InvokesCaddyValidate`, `TestReloader_InvokesCaddyReload`). They assert against an obsolete contract and are exactly the change-detector tests CLAUDE.md prohibits.

Add:

| Test | Asserts |
|---|---|
| `TestReloader_ValidateCallsDockerExec` | `Driver.Exec` called once with `Container=decloud-caddy`, `Cmd=["caddy","validate","--config","/etc/caddy/Caddyfile.tmp"]`. |
| `TestReloader_ReloadCallsDockerExec` | Same shape for `caddy reload --config /etc/caddy/Caddyfile`. |
| `TestReloader_PathTranslationCanonicalForm` | Pass `/opt/decloud/config/caddy/Caddyfile.tmp`; assert translated to `/etc/caddy/Caddyfile.tmp`. (Linus §7 positive case.) |
| `TestReloader_PathTranslationOutsideBindMount` | Pass `/tmp/foo`; assert error mentions "outside the bind-mount". No exec call. |
| `TestReloader_ContainerNotRunningSurfacesActionableError` | Mock `Driver.Exec` returns `ErrContainerNotFound`; assert err string matches `container "decloud-caddy" is not running; run 'decloud caddy up' first`. |
| `TestReloader_ValidateExitNonzeroPreservesStderr` | Inner exec error survives the wrap (locks `%w: %w` discipline). |

`cmdFactory` is gone; no `newCLIReloaderWithFactory` test path.

### 6.3 `internal/dockerdrv/cli_driver_test.go` (additions)

| Test | Argv asserted |
|---|---|
| `TestCLIDriver_ImagePullArgs` | `pull caddy:2` |
| `TestCLIDriver_ExecArgsBasic` | `exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp` |
| `TestCLIDriver_ExecPropagatesContainerNotFound` | `Exec` against absent container → `ErrContainerNotFound`. |
| `TestCLIDriver_RunWithOptionsCaddyShape` | Full Caddy `docker run` argv per §3.2 below, including all six dual-stack port maps. |
| `TestCLIDriver_RunWithOptionsDualStackPorts` | Independent of Caddy specifics: a `RunOptions` with both `0.0.0.0` and `[::]` host bindings produces both `-p 0.0.0.0:X:Y` and `-p [::]:X:Y` in argv. (Linus §10 non-blocking-but-encouraged; we do it.) |
| `TestCLIDriver_RunWithOptionsBindReadOnly` | Bind volume with `ReadOnly=true` produces `:ro` suffix. |
| `TestCLIDriver_RunWithOptionsNamedVolume` | Named volume produces `name:target` syntax. |

### 6.4 `internal/cli/caddy_up_test.go`, `caddy_down_test.go` (new)

`caddyManagerFactory` test seam, identical pattern to `installMockDeployer` in `deploy_service_test.go`.

| Test | Asserts |
|---|---|
| `TestCaddyUp_DelegatesToManager` | `Manager.Up` returns nil → exit 0. |
| `TestCaddyUp_ManagerErrorReturnsExitRunFail` | `ErrCaddyUp`-wrapped → exit 40. |
| `TestCaddyDown_DelegatesToManager` | `Manager.Down` returns nil → exit 0. |
| `TestCaddyDown_ManagerErrorReturnsExitRunFail` | `ErrCaddyDown`-wrapped → exit 40. |

**No `TestCaddyUp_PassesImageOverrideFromFlag`** (Joel v1 §8.4) — there is no `--image` flag.

### 6.5 `internal/deploy/service_test.go`

Reloader is mocked. The constructor signature change at production wiring layer doesn't reach the deploy tests. Run `go test ./internal/deploy/...` and confirm zero diffs needed. If a test asserts on `caddy.NewCLIReloader`, it's over-specified and we cut it.

### 6.6 Tests we are NOT adding

- Generator tests (output unchanged).
- Real-Docker integration tests (deferred per `_ai/decisions/m1-test-strategy.md`).
- Caddy's own behaviour (cert provisioning, HTTP/3, ALPN).
- Tests for `_docs/*.md` content (Raymond owns; Kevlin reviews).

---

## 7. Manual verification

Operator runs these on the actual host after Rob ships and Raymond updates docs:

```sh
# 0. Persistently stop the host Caddy from M1.0 (note the "mask" or "remove" — disable alone is not enough).
systemctl disable --now caddy && systemctl mask caddy
# OR
apt-get remove -y caddy

# 0a. RECOMMENDED: migrate ACME state to preserve issued certs.
docker volume create decloud_caddy_data
docker run --rm \
  -v /var/lib/caddy/.local/share/caddy:/from \
  -v decloud_caddy_data:/to \
  alpine sh -c 'cp -a /from/. /to/'

# 1. Rebuild and reinstall the binary.
GOOS=linux GOARCH=amd64 go build -o decloud ./cmd/decloud
scp decloud root@hosting:/usr/local/bin/decloud
ssh root@hosting chmod 0755 /usr/local/bin/decloud

# 2. Bring Caddy up. Pulls caddy:2, runs decloud-caddy on `decloud`, dual-stack ports.
ssh root@hosting decloud caddy up

# 3. Verify Caddy is on the decloud network.
ssh root@hosting docker network inspect decloud
# Expect: TWO containers — decloud-durak-live AND decloud-caddy.

# 4. Verify Caddy can resolve the upstream from inside its own container.
ssh root@hosting docker exec decloud-caddy nslookup decloud-durak-live
# Expect: A record pointing at 172.18.0.2 (or the bridge IP). NOT the host's public IPv6.

# 5. Verify dual-stack listener (the v1 plan didn't have this step).
ssh root@hosting ss -tlnp | grep -E ':(80|443) '
# Expect: listeners on both 0.0.0.0:80, 0.0.0.0:443 AND :::80, :::443.

# 6. Hit it for real over both stacks.
curl -v https://live.durak.click/healthz                 # default DNS path
curl -v -4 https://live.durak.click/healthz              # force IPv4
curl -v -6 https://live.durak.click/healthz              # force IPv6
# Expect all three: 200, valid TLS cert, no "connection refused" in Caddy logs.

# 7. Confirm Caddy logs are clean.
ssh root@hosting docker logs decloud-caddy --tail 50
# Expect: no `dial tcp [2a03:f480:...]` errors.
```

Step 5 and the dual-stack curl in step 6 are new in v2 and exist specifically because of revision #4. If step 5 shows only `0.0.0.0` listeners, the dual-stack publishing didn't take effect and the IPv6 regression is shipping; halt and investigate.

---

## 8. Architectural smells the user should hear honestly

### 8.1 We had this exact bug coming and didn't see it

Tech-plan §9.4 of the M1 implementation explicitly identified the embedded-DNS gap for the readiness probe. The author patched readiness via `Driver.ContainerIP`. The same realisation should have prompted "what about Caddy?" — it didn't, because the correction lived inline in a tech plan rather than promoted to a Decision in `_ai/decisions/`. v2 closes that loop: `_ai/decisions/caddy-runs-in-container.md` documents the architectural decision, the rejected alternatives (Don's A/B plus Linus's seven additional ones), the volume strategy, the migration recipes, and the M4 forward-looking note about the admin API.

### 8.2 The whole "Caddy on the host" arc — re-evaluated

v1 called this an unforced error. Linus pushed back (§1.2): containerising Caddy at M1 design time would have introduced the volume-mount question, the bind-mount-source question, and the cross-namespace `caddy reload` question — exactly what this task is spending effort on. Punting was defensible. **The unforced error was specifically not writing down the asymmetry as a Decision-with-implications.** v2 adopts that more accurate framing. The lesson is about decision-doc discipline, not about the original architectural choice being wrong.

### 8.3 The install doc lied to the operator

The install doc said "create the `decloud` network so Caddy can reach upstreams by container name" — but Caddy was not on the network. Raymond rewrites §3 (Caddy install — replace systemd block with `decloud caddy up`), §5 (network create — note that `caddy up` will do it; explicit step is for operators who want to inspect first), and **deletes** the §61-62 paragraph entirely (Linus §6 #3). Kevlin's hallucination check on the rewrite is mandatory.

### 8.4 The integration test we don't have

Linus §7 confirmed Joel's rejection of adding a `-tags integration` test in this task. M1 test strategy stands; backlog item lands in `_ai/m1x-backlog.md` so M2's first tech plan can revisit. The bug we're fixing is exactly the kind M1 test strategy explicitly anticipated would slip through, and the response is "fix the bug, log the gap" — not "expand scope now."

---

## 9. Research info — facts the implementation agents will need

(Per Don's research-info standard. Read the cited files; I'm not pasting code snippets.)

- **Caddyfile generation:** `internal/caddy/generator.go`. `Generator.Generate(outPath, services)` writes the file at `outPath`. Body emits `reverse_proxy <ContainerName>:<Port>` per host. `ContainerName` from `svc.Config.State.ContainerName` else `"decloud-"+svc.Config.Name`. Stub at `internal/caddy/stub.go`.
- **Reload pipeline today:** `internal/caddy/reloader.go`. `Validate(ctx, configPath)` and `Reload(ctx, configPath)` shell out via `cmdFactory` (line 18). Production factory is `exec.CommandContext`. v2 deletes `cmdFactory`; the new constructor is `NewCLIReloader(driver dockerdrv.Driver, hostCaddyDir string) Reloader`.
- **Docker driver:** `internal/dockerdrv/driver.go` (interface), `internal/dockerdrv/cli_driver.go` (impl), `internal/dockerdrv/mocks/` (gomock). `RunRequest` (driver.go:31-38) is kept for service deploys. New methods on `Driver`: `ImagePull(ctx, ref) error`, `RunWithOptions(ctx, opts RunOptions) (string, error)`, `Exec(ctx, opts ExecOptions) error`. `RunOptions` carries `Ports []PortMap` (with `HostBind string` to support `0.0.0.0` vs `[::]`), `Volumes []VolumeMount`, `Labels`, `Env`, `Restart`, `Network`, `Name`, `Image`. `PortMap` has `HostBind`, `HostPort`, `ContainerPort`, `Proto`. `VolumeMount` has `Source`, `Target`, `ReadOnly`, `IsNamed`.
- **Deploy orchestrator:** `internal/deploy/service.go`. `regenerateAndReload` at lines 302-325 — atomic-rename + validate flow stays. Wrap text on the validate-failed and reload-failed legs gets the §4.4 update.
- **Container naming:** `internal/ids` (`ContainerName(name)` returns `"decloud-"+name`). New constants live in `internal/caddy/manager.go`: `ContainerName = "decloud-caddy"`, `NetworkName = "decloud"`, `DefaultImage = "caddy:2"`. New code references `caddy.NetworkName`; existing scattered `"decloud"` literals (`internal/deploy/service.go:131`, `:190`, `:289`; `internal/dockerdrv/cli_driver.go:170`) stay for now and become an M1.x cleanup item.
- **Path config:** `internal/config/paths.go`. `Paths.CaddyfilePath = <root>/config/caddy/Caddyfile`. `Paths.CaddyDir = <root>/config/caddy`. Bind-mount source.
- **CLI surface:** `internal/cli/root.go:40-42` already mounts `caddy` parent with `reload` child. Add `up` and `down` siblings. `caddyManagerFactory` test seam in `internal/cli/deploy_service.go` mirrors `deployerFactory` (parallelism caveat at lines 21-25).
- **Exit codes:** `internal/cli/exit_codes.go`. `ExitRunFail = 40`, `ExitCaddyReloadFail = 60`. `caddy.ErrCaddyUp` and `caddy.ErrCaddyDown` map to 40. No new constants. `caddy reload` and the deploy-failed-on-reload paths continue to map to 60.
- **Test seam discipline:** `_ai/gomock-inorder-sequencing.md`, `_ai/error-wrap-discipline.md`, `_ai/explicit-inputs-not-globals.md`, `_ai/cobra-init-pattern.md`. New code follows all four.
- **Scope discipline:** `_ai/decisions/m1-scope.md:18` (no Viper), `_ai/decisions/m1-test-strategy.md` (no integration tests).

---

## 10. Acceptance criteria — when is this DONE

1. `decloud caddy up` exists and brings up `decloud-caddy` on the `decloud` network with **dual-stack** publishing on 80/tcp, 443/tcp, 443/udp (six PortMap entries per §4.3), the directory bind-mount for `/opt/decloud/config/caddy`, and the two named volumes. Idempotent.
2. `decloud caddy down` exists, stops and removes `decloud-caddy`, leaves both volumes intact. Idempotent.
3. `decloud caddy reload` continues to work; under the hood it `docker exec`s into `decloud-caddy` via `Driver.Exec`.
4. `decloud caddy up` takes **no flags**; `caddy:2` is hardcoded as `caddy.DefaultImage`. No `--image`, no Viper, no TOML key (per §4.1).
5. `Manager.Up` does NOT poll Caddy's admin API and does NOT roll back on partial failure (per §4.2).
6. The reloader's `cmdFactory` test seam is **deleted**; `Driver.Exec` is the only seam (per §4.5).
7. The deploy-failure error message on `ErrCaddyReload` paths includes the actionable recovery one-liner (per §4.4).
8. `decloud deploy service ...` succeeds end-to-end on the user's host: TLS cert provisions (or migrates per §4.6), `curl` over both v4 and v6 returns 200.
9. Caddy logs show no `dial tcp [2a03:...]` errors.
10. `docker network inspect decloud` shows BOTH `decloud-caddy` and the service container as members.
11. Host-side `ss -tlnp` shows BOTH `0.0.0.0` and `::` listeners on 80/443 (per §7 step 5).
12. `_docs/install.md` rewritten: §3 replaced (no systemd block, `decloud caddy up` instead), §5 cross-reference updated, §61-62 paragraph **deleted**, migration section uses the volume-copy recipe as the recommended path (per §4.6), `systemctl mask` / `apt-get remove` named in the ports-bound error path. Kevlin reviews for hallucinations.
13. `_docs/usage.md` §1 quick start mentions `decloud caddy up`. The `-p ... is never invoked` paragraph in §6 is rewritten — Caddy is now the documented exception. §7 reload-recovery updated.
14. `_ai/decisions/caddy-runs-in-container.md` exists; reviewed during Phase 4 (alongside CLI surface), not after Phase 7 (per Linus §5.7). Documents Don's Candidates A/B/C plus Linus's seven additional rejected alternatives, volume strategy, migration recipes, M4 admin-API forward note.
15. `_ai/m1x-backlog.md` includes the deferred integration-test item.
16. `internal/dockerdrv/driver.go` interface gains exactly three methods (`ImagePull`, `Exec`, `RunWithOptions`); existing methods byte-identical.
17. `internal/cli/exit_codes.go` gains zero constants. `ErrCaddyUp`/`ErrCaddyDown` map to `ExitRunFail` (40).
18. `go test ./...` passes. `go vet ./...` clean. `gofmt -l .` empty. `go generate ./...` then `git status --porcelain` empty.
19. The user runs §7 manual verification on the actual host and reports green.

If any of 1-19 is missing at sign-off, the task is not done. No exceptions.

---

## 11. Revisions applied — Linus map

| # | Linus required revision | Where applied in v2 |
|---|---|---|
| 1 | Cut §11.3 readiness loop / admin-API polling from `Up` | §4.2; §6.1 (no test); §10 criterion #5 |
| 2 | Cut §11.4 rollback (vacuous after #1) | §4.2; §6.1 (no rollback test) |
| 3 | Cut `--image` flag, Viper wiring, TOML `caddy.image` | §4.1; §6.4 (no flag test); §10 criterion #4 |
| 4 | Add dual-stack IPv6 port publishing for 80/tcp, 443/tcp, 443/udp | §4.3; §6.3 dual-stack test; §7 manual step 5; §10 criteria #1 and #11 |
| 5 | Update deploy-failure error text to give a one-command recovery | §4.4; §10 criterion #7 |
| 6 | Delete `cmdFactory` from `cliReloader` once `Driver.Exec` is the seam | §4.5; §6.2 (no `cmdFactory` tests); §10 criterion #6 |
| 7 | Strengthen ACME migration warning; recommend volume-copy as default | §4.6; §7 step 0a; §10 criterion #12 |

Linus also confirmed:
- §6.4 integration test rejected (M1 test strategy stands) — §6.6, §8.4.
- `_docs/install.md:61-62` paragraph DELETED, not edited — §10 criterion #12.

---

## 12. Hand-off

Joel: revise your tech plan to v2 against this document. Specifically:
- Replace your §1.2 with a flag-less spec; remove `--image`, Viper, and TOML config language entirely.
- Replace your §11.3 and §11.4 with a single paragraph explaining why neither exists (§4.2 above is the source).
- Replace your §3.2 `docker run` form with the dual-stack version (six `-p` entries, both `0.0.0.0` and `[::]`).
- Update your §4.4 reloader struct: drop `cmdFactory`; only `driver` and `hostCaddyDir` fields remain.
- Update your §4.4 algorithm sketch and the `service.go` wrap text per §4.4 above.
- Update your `_docs/install.md` checklist (Joel §10.1 / Raymond's task list) so the migration block is the volume-copy recipe by default with the cold-restart as the alternative.
- Confirm the §61-62 deletion as a separate checklist item.

Linus: when Joel's v2 lands, please re-review focusing on:
1. The dual-stack RunOptions shape — is it clean, or did we leak `[::]`/`0.0.0.0` ad-hoc strings into multiple call sites?
2. The new wrap text in `service.go` — does the operator actually know what to do, or did we just make the error longer?
3. The migration recipe — did Raymond / Joel land it inline in the install doc, or did it sneak back into the decision record?

— Don
