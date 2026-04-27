# 002 — Don's Plan: Caddy can't reach `decloud-durak-live` because Caddy is on the host, not on the `decloud` Docker network

Author: Don Melton (planning agent)
Date: 2026-04-27
Status: PLAN draft, awaiting Joel's tech expansion + Linus review.

## TL;DR

We shipped a broken architecture. The Caddyfile says `reverse_proxy decloud-durak-live:10001`, which depends on Docker's embedded DNS resolving the container name on the `decloud` bridge. But our install doc puts Caddy on the host as a plain systemd unit (`/etc/systemd/system/caddy.service` with `User=caddy`), and nothing — anywhere in the codebase or install procedure — attaches Caddy to the `decloud` network. So when Caddy tries to resolve `decloud-durak-live`, it falls through to the host's resolver, which returns the host's public IPv6 (`2a03:f480:1:12::7b`) via a wildcard / search domain / catch-all `A`+`AAAA` answer. Caddy then dials `[2a03:f480:...]:10001` on the host's public address, where nothing is listening, and gets `connection refused`.

This is not a config bug. It is an architectural gap. The Caddyfile and the install doc disagree about where Caddy lives. One of them has to change.

The right fix for M1 is: **Caddy runs as a Docker container on the `decloud` network, owned by Decloud, brought up by a new `decloud caddy up` command.** That is the only way the `reverse_proxy decloud-durak-live:10001` line means what it says.

The rest of this document proves the diagnosis with code citations, lays out the fix, names the edge cases, names the tests, names the manual verification steps, and calls out the architectural smell honestly.

---

## 1. Root cause — proof, not opinion

### 1.1 The Caddyfile assumes embedded Docker DNS

Generated form (`internal/caddy/generator.go:43-46`):

```go
fmt.Fprintf(&buf, "%s {\n", host)
fmt.Fprintf(&buf, "    reverse_proxy %s:%d\n", in.ContainerName, in.Port)
fmt.Fprintln(&buf, "}")
```

`in.ContainerName` is `decloud-<service>` (`internal/caddy/generator.go:65-68`, mirroring `ids.ContainerName(req.Name)` at `internal/deploy/service.go:158`). That short name is **only** resolvable by Docker's embedded DNS server (`127.0.0.11`), which is **only** mounted into containers that are members of a user-defined Docker bridge network. A host process cannot resolve `decloud-durak-live` unless something else (host `/etc/hosts`, libvirt, mDNS, a search domain wildcard) happens to resolve it — and on this host, it is the search-domain/wildcard catch-all that returns the host's public IPv6.

### 1.2 Service container IS on `decloud` — proven

`internal/deploy/service.go:131-134`:

```go
if err := d.deps.Driver.NetworkEnsure(ctx, "decloud"); err != nil {
    ...
}
```

`internal/deploy/service.go:187-194`:

```go
runReq := dockerdrv.RunRequest{
    Name:    containerName,
    Image:   imageRef,
    Network: "decloud",
    ...
}
```

`internal/dockerdrv/cli_driver.go:46-50`:

```go
args := []string{
    "run", "-d",
    "--name", req.Name,
    "--network", req.Network,
    ...
}
```

The `docker network inspect decloud` output in the user request confirms `decloud-durak-live` at `172.18.0.2/16`. Service-side networking is correct.

### 1.3 Caddy is NOT on `decloud` — proven by absence

Search results:

- `internal/caddy/reloader.go:25-27` constructs `cliReloader{cmd: exec.CommandContext}`. It shells `caddy validate` and `caddy reload` against the local `caddy` binary on `PATH`. There is no `docker exec`, no `docker run`, no `docker network connect`, no Caddy container management anywhere.
- `internal/cli/caddy_reload.go` exposes only `decloud caddy reload`. There is no `decloud caddy up`, no `decloud caddy install`, no `decloud caddy ensure-network`.
- `cmd/decloud/main.go` and `internal/cli/root.go:40-42` confirm `caddy` is the only Caddy-related subcommand surface, and it has exactly one child, `reload`.
- `_docs/install.md:30-52` instructs the operator to write a host systemd unit with `ExecStart=/usr/bin/caddy run ...`. The unit has no `ExecStartPre=docker network connect ...`, no `BindsTo=docker.service`, and no Docker side-channel of any kind. It is a plain systemd-managed host process.
- `_docs/install.md:96-100` says "Create the shared Docker network: `docker network create decloud`" but does not connect Caddy to it. Caddy is not even mentioned in §5.
- The `_tasks/2026-04-26-m1-implementation/` history shows the team identified this exact problem for the readiness probe (tech-plan §9.4 at line 784: "the decloud process runs on the host, it can't resolve `<containerName>` over Docker DNS") and solved readiness by going through the bridge IP directly (`internal/deploy/readiness.go:48-55`, calling `Driver.ContainerIP` and dialing `http://<ip>:<port><path>`). **That same realisation was not propagated to Caddy.** Caddy's resolution path was assumed to "just work" because Caddy is "the only thing that talks to containers by name," but nobody verified Caddy was actually on the network. It is not.

### 1.4 Why the dial address is the host's public IPv6

`docker network inspect decloud` shows `EnableIPv6: false`. The container has no IPv6. The dial target `[2a03:f480:1:12::7b]:10001` is the host's own public IPv6 (the `:7b` suffix is the host's GUA-assigned address; the user's diagnostic does not include `ip -6 addr` output but the `2a03:f480:1:12::/64` prefix is unambiguously a public IPv6 prefix — likely Hetzner or similar). The `connection refused` happens because nothing is bound to `:10001` on the host's public IPv6 address — only the container is, on its private bridge IPv4. So Caddy's resolver returned the **host's** AAAA record (likely via a wildcard such as `*.<host-domain>` or via a glibc resolver search-list match), Go's `net.Dial` preferred the AAAA over the absent A, and the connection refused. The exact resolver path doesn't matter for the fix — what matters is that any path that is not Docker's embedded DNS is a path that returns the wrong answer.

### 1.5 Verdict

Caddy is generating valid Caddyfile syntax targeting an upstream name (`decloud-durak-live`) that only Docker's embedded DNS can resolve, but Caddy is running outside Docker entirely. The fix has to put Caddy somewhere that Docker's embedded DNS will be consulted for that name.

---

## 2. The fix

There are three candidates. I'll evaluate all three so we have evidence for the choice, not a knee-jerk.

### 2.1 Candidate A (REJECTED) — keep host Caddy, write IPs into the Caddyfile

Have the generator inspect each container and emit `reverse_proxy 172.18.0.2:10001` instead of `reverse_proxy decloud-durak-live:10001`.

**Why it's wrong:**
1. Bridge IPs are not stable across container restarts. The whole point of name-based reverse proxying is that the container can be replaced and Docker DNS picks up the new IP. Hard-coding IPs into the Caddyfile means every recreate forces a Caddy reload AND introduces a race where Caddy is briefly proxying to a dead IP.
2. The generator currently has no access to a Docker driver. Plumbing it in pollutes the generator's interface for a workaround.
3. We lose every nice property of Caddy + Docker-DNS integration (auto-failover on container replace, no Caddy reload needed for in-network IP changes if we ever go to swarm/compose).
4. It papers over the architectural smell instead of fixing it.

Reject.

### 2.2 Candidate B (REJECTED) — keep host Caddy, publish container ports to the host

Run containers with `-p 10001:10001` and have Caddy `reverse_proxy 127.0.0.1:10001`.

**Why it's wrong:**
1. Explicitly contradicts the design decision in `_docs/usage.md:181-192` and tech-plan §11.2 line 1028: "the port is NOT exposed via `-p` because Caddy reaches the container over the shared network". This is not a casual choice; it is a security and namespace property the team committed to.
2. Forces port-collision management in the deployer (two services can't both want `:8080`).
3. Exposes container ports to anything else listening on the host's loopback or — worse, depending on Docker's default — to the world. We'd then need firewall rules.
4. Same architectural smell, different paint.

Reject.

### 2.3 Candidate C (CHOSEN) — run Caddy as a Decloud-managed container on the `decloud` network

Decloud owns Caddy. We ship a `decloud caddy up` command (and a complementary `decloud caddy down`) that:

1. Ensures the `decloud` network exists (via the existing `Driver.NetworkEnsure`).
2. Pulls/uses the official `caddy:2` image.
3. Runs a container named `decloud-caddy` with `--network=decloud`, `--restart=unless-stopped`, `-p 80:80 -p 443:443` (host-published, because Caddy IS public ingress and IS allowed to bind host ports — that's the one place port publishing belongs), volume-mounts for `/opt/decloud/config/caddy/Caddyfile -> /etc/caddy/Caddyfile`, plus persistent volumes for `caddy_data` and `caddy_config` so ACME state survives container replace.
4. The reloader stops shelling `caddy reload` against a host binary and instead runs `docker exec decloud-caddy caddy reload --config /etc/caddy/Caddyfile`. `caddy validate` similarly becomes `docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp` against a tmp path bind-mounted in (or copied into the container).

This makes the `reverse_proxy decloud-durak-live:10001` line **mean what it says**. Caddy is on the same Docker user-defined bridge network as the service containers; Docker's embedded DNS resolves the name to `172.18.0.2`; the connection succeeds.

This is also the design the M1 team clearly assumed without writing it down. The whole `internal/caddy/generator.go` output was written for a Caddy-in-the-network world. We'll align reality to the assumption rather than the other way around.

### 2.4 What the fix concretely changes

Files added:

- `internal/caddy/manager.go` — new `Manager` interface with `Up(ctx) error`, `Down(ctx) error`, `IsRunning(ctx) (bool, error)`. Production impl shells out via the existing `dockerdrv.Driver`. (Don't roll our own `docker run` here; use the driver we already have. If `Driver` lacks the few primitives needed — port publishing, volumes, image pulls — extend `Driver`, don't bypass it.)
- `internal/caddy/manager_test.go` — gomock against `dockerdrv.Driver` to assert exact arg sequencing on `Up`/`Down`/`IsRunning`.
- `internal/cli/caddy_up.go`, `internal/cli/caddy_down.go` — Cobra commands wired into the existing `caddy` subtree at `internal/cli/root.go:40-42`.

Files changed:

- `internal/caddy/reloader.go` — `cliReloader.runCaddy` switches from invoking host `caddy` to invoking `docker exec decloud-caddy caddy ...`. The validate/reload contracts and error wrapping stay identical (per `caddy/reloader.go:33-49`); only the command construction changes. This is the minimal-blast-radius change.
- `internal/dockerdrv/cli_driver.go` + `internal/dockerdrv/driver.go` — extend `RunRequest` (or add a new `Driver.RunWithOptions`) to support `Ports []PortMap`, `Volumes []VolumeMount`, and `ImagePull(ctx, ref)`. The Caddy container needs all three; service containers don't, so the existing `RunRequest` shape stays valid for service deploys.
- `_docs/install.md` — §3 ("Install Caddy") rewritten. Operator no longer installs Caddy as a host package or writes a systemd unit. Instead: `decloud caddy up` does it. Add an explicit warning: "If you previously ran Caddy on the host (M1.0 install instructions), `systemctl disable --now caddy && apt-get remove caddy` first, otherwise port 80/443 will already be bound and the container will fail to start."
- `_docs/usage.md` — §1 quick start gains a `decloud caddy up` step before the first deploy. §7 (recovering from `caddy reload` failures) updated to reflect that `caddy reload` is now `docker exec`'d.
- `_ai/decisions/` — new file `caddy-runs-in-container.md` documenting the architectural decision and why we rejected Candidates A and B. This is the kind of decision Future-Don will look for when someone asks "why does Caddy run in a container instead of on the host like every other Caddy install on the planet?"

Files unchanged:

- `internal/caddy/generator.go` and the generated Caddyfile format. **Don't touch the generator.** Its output is correct under the new architecture and was always intended for this architecture.
- `internal/deploy/service.go` and `internal/deploy/readiness.go`. Service deploy logic is correct. Readiness already goes through bridge IP, so it doesn't depend on Caddy being anywhere in particular.
- The `decloud` network creation step in install. It still happens; it just gets a co-resident.

---

## 3. Edge cases — name them all upfront

### 3.1 First-deploy ordering

Today: `decloud deploy service` writes a stub Caddyfile if missing (`internal/caddy/stub.go:18-28`, called from `service.go:307-309`), generates the real Caddyfile, calls `caddy validate`, then `caddy reload`. Caddy must be running for `reload`. `validate` does not strictly need a running Caddy, but `docker exec decloud-caddy caddy validate ...` does need the container running.

**New ordering:**

1. Operator runs `decloud caddy up` once, after install. This pulls `caddy:2`, runs `decloud-caddy` with the stub Caddyfile already on disk (the manager writes a stub if `/opt/decloud/config/caddy/Caddyfile` is missing, identical body to `caddy/stub.go`), publishes 80/443, joins `decloud`. Container comes up healthy serving the stub 404.
2. First `decloud deploy service` proceeds as today, but `Reloader.Validate`/`Reload` now `docker exec` into the running Caddy container.

**Edge case: operator forgets to run `decloud caddy up` first.** The `caddy validate` step in deploy will fail (`docker exec` against a non-running/non-existent container). We catch this and emit a clear error: `"caddy container 'decloud-caddy' is not running; run 'decloud caddy up' first"`. Map to existing exit code 60 (`ExitCaddyReloadFail`). Do NOT silently auto-bring-up Caddy from inside a deploy — that's a side-effect the operator didn't ask for, and it would mask the install-step omission.

**Edge case: `decloud caddy up` called when Caddy is already running.** Idempotent: detect via `docker inspect decloud-caddy`, no-op with a "Caddy already running" message. Same shape as `Lifecycle.Start`'s already-running branch.

### 3.2 Network not created yet

`decloud caddy up` calls `Driver.NetworkEnsure(ctx, "decloud")` first, same as `service.go:131`. If the network doesn't exist, it creates it. So the install doc's "Step 5: docker network create decloud" becomes optional — `decloud caddy up` will do it. Keep the explicit step in the install doc anyway, because doing-it-twice is harmless and operators like to see the network exist before running anything.

### 3.3 Port 80/443 already bound (typically: an existing host Caddy)

If the operator is upgrading from M1.0 (host Caddy), `decloud caddy up` will fail because `:80`/`:443` are already bound. We need a clear error and a documented migration path. The error message should name the most likely cause ("port 80/443 already in use; if you ran the M1.0 install, `systemctl disable --now caddy` first").

### 3.4 ACME state must survive container replace

`caddy:2` writes ACME data to `/data` and config to `/config`. We MUST mount named Docker volumes for both, otherwise every `decloud caddy down && decloud caddy up` re-issues all certs and trips Let's Encrypt rate limits in a hurry. Volumes: `decloud_caddy_data:/data`, `decloud_caddy_config:/config`. Document this in `_ai/decisions/caddy-runs-in-container.md` because it is exactly the kind of "why did this break in production" question Future-Don will get.

### 3.5 `decloud caddy down` semantics

Stop and remove the container, but **leave the volumes intact**. Operators who want a full nuke can `docker volume rm decloud_caddy_data decloud_caddy_config` themselves. We do NOT remove volumes from `caddy down` — that's the kind of footgun that gets you a 4 AM call. Document loudly.

### 3.6 Caddyfile path inside the container vs on the host

Host path is `/opt/decloud/config/caddy/Caddyfile` (`config.Paths.CaddyfilePath`). Container sees it as `/etc/caddy/Caddyfile` via bind mount. The reloader passes the **container** path to `docker exec ... caddy validate --config /etc/caddy/Caddyfile`. The atomic-rename trick in `service.go:310-320` (write `.tmp`, validate it, rename into place) needs adjusting: we now write `Caddyfile.tmp` to the same host directory (already bind-mounted), validate via `docker exec ... caddy validate --config /etc/caddy/Caddyfile.tmp`, then rename on the host. The bind mount makes the rename visible to the container. Reload then targets `/etc/caddy/Caddyfile`. Works because the bind mount is a directory bind, not a file bind.

### 3.7 Idempotency

- `caddy up` when already running: no-op + message.
- `caddy up` when stopped-but-exists (`docker ps -a` shows it): start it, no-op the create.
- `caddy up` when the image needs updating (e.g., `caddy:2` got a new digest): out of scope for M1.x. Operator does `decloud caddy down && docker pull caddy:2 && decloud caddy up`. Document.
- `caddy down` when not running: no-op, success exit.
- `caddy reload` after `caddy down`: hard error 60 with the same "container not running" message as 3.1.

### 3.8 Hostname-in-Caddyfile vs container-name resolution

Once Caddy is on `decloud`, both `decloud-durak-live` AND its DNS aliases resolve through embedded DNS. The Caddyfile already uses container names (which is what Docker DNS keys on by default). No change needed to the generator. Verified.

### 3.9 EnableIPv6 on the network

Network was created without `--ipv6`, so `EnableIPv6: false`. Once Caddy is in-network, this doesn't matter — Docker DNS only returns A records for an IPv4-only network. The IPv6-fallthrough bug literally cannot recur.

---

## 4. Tests

Per CLAUDE.md: Testify assertions, Gomock for mocks, no change-detector tests. Tests live next to the code they exercise (CLAUDE.md test location rules).

### 4.1 New: `internal/caddy/manager_test.go`

Behaviors to cover (each one is a real behavior, not a "did we type the right string into the mock"):

- `Up` when network missing and container missing: exact ordering — `NetworkEnsure` → image pull → run with `--network=decloud --name=decloud-caddy --restart=unless-stopped` plus port maps and volume mounts.
- `Up` when container already running: returns nil after a single `Inspect`, no Run.
- `Up` when container exists but exited: `Driver.Start`, no fresh Run.
- `Up` propagates and wraps NetworkEnsure errors as `ErrCaddyUp`-class with the unwrap chain intact.
- `Down` when container running: `Stop` with grace, then `Remove`. Volumes untouched (no `--volumes` flag on the rm call).
- `Down` when container absent: no error, idempotent.
- `IsRunning` returns true/false correctly off `Driver.Inspect` `running` vs other states.

Use `gomock.InOrder` for the multi-step sequences, same discipline `_ai/gomock-inorder-sequencing.md` already documents.

### 4.2 Updated: `internal/caddy/reloader_test.go`

Existing tests assert on the host `caddy` argv shape (e.g., `caddy validate --config /path/Caddyfile`). After the fix, the argv becomes `docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp`. Update the existing test seam — the `cmdFactory` already records argv, so the assertions just change shape, not structure. This is NOT a change-detector test; we are asserting the correct invocation pattern, not a snapshot.

Add: a test that asserts the reloader returns a meaningful error when the underlying `docker exec` fails because the container is not running (stderr matches `Error response from daemon: Container ... is not running` or similar). Wrap as `ErrCaddyReload` with the exec error chained.

### 4.3 New: `internal/cli/caddy_up_test.go`, `caddy_down_test.go`

Cobra-level tests using the same factory test seam pattern as `internal/cli/deploy_service_test.go` (a package-global `caddyManagerFactory`, reassigned in setup, restored in teardown — note the parallelism caveat already called out in `internal/cli/deploy_service.go:21-25`). Cover: success path, network-ensure failure → exit code mapping, container-already-running → exit 0 with informational message.

### 4.4 No integration tests in this task

Per `_ai/decisions/m1-test-strategy.md` and Don's standing M1 directive: real-Docker integration is the user's manual verification step. If we ever bring back the `-tags integration` plumbing, this is one of the obvious first integration tests (it would have caught the bug pre-ship). Note in the new `caddy-runs-in-container.md` decision doc.

### 4.5 Tests we are NOT adding

- Generator tests do not change. The output shape is unchanged.
- Service deploy tests do not change. The Caddy reload step is mocked through `caddy.Reloader`; whether the production reloader shells `caddy` or `docker exec ... caddy` is invisible to the deployer.

---

## 5. Manual verification (the user runs these on the real host)

After Rob ships and Raymond updates the docs:

```sh
# 0. Stop the host Caddy from M1.0 install, if any.
systemctl disable --now caddy || true
apt-get remove -y caddy || true   # or however it was installed

# 1. Rebuild and reinstall the binary.
GOOS=linux GOARCH=amd64 go build -o decloud ./cmd/decloud
scp decloud root@hosting:/usr/local/bin/decloud
ssh root@hosting chmod 0755 /usr/local/bin/decloud

# 2. Bring Caddy up. Should pull caddy:2, create the container, attach to `decloud`.
ssh root@hosting decloud caddy up

# 3. Verify Caddy is on the decloud network.
ssh root@hosting docker network inspect decloud
# Expect TWO containers: decloud-durak-live AND decloud-caddy.

# 4. Verify Caddy can resolve the upstream from inside its own container.
ssh root@hosting docker exec decloud-caddy nslookup decloud-durak-live
# Expect: an answer pointing at 172.18.0.2 (or whatever bridge IP the service has).

# 5. Hit it for real.
curl -v https://live.durak.click/healthz
# Expect: 200, valid TLS cert, no "connection refused" in Caddy logs.

# 6. Confirm Caddy logs are clean.
ssh root@hosting docker logs decloud-caddy --tail 50
# Expect: no `dial tcp [2a03:f480:...]` errors.
```

If step 4 returns the host's public IPv6, the fix is wrong and we restart the investigation. (It won't, because the container is on a network that has `EnableIPv6: false` and Docker's embedded DNS is the only resolver in effect inside the container.)

---

## 6. Architectural smells the user should hear honestly

I'm calling this out because I'm Don and that's my job.

### 6.1 We had this exact bug coming and didn't see it

Tech-plan §9.4 (line 784 of `_tasks/2026-04-26-m1-implementation/03-tech-plan.md`) **explicitly identifies** that the decloud host process can't resolve container names via Docker DNS. The author recognised it for the readiness probe and patched readiness via `Driver.ContainerIP` to dial the bridge IP directly. That patch fixed readiness, but **the same realisation should have prompted "wait, what about Caddy?"** It didn't, because Caddy reload was treated as a "shell out to a host binary" problem, and nobody asked where the host binary's resolution path goes when it sees a container name. This is the kind of thing the `_ai/decisions/` discipline is supposed to catch — write down "decloud is on the host, not on the network" as a Decision-with-implications, and then every later review checks: "does my new code respect this decision? does it need to?"

We will write that decision doc now. Late, but writing it now beats writing it after the next bug.

### 6.2 The whole "Caddy on the host" arc was an unforced error

A platform that runs everything else in containers, and then runs the public ingress as a host package with a hand-rolled systemd unit, is asymmetric. Asymmetry is where bugs live. The right design from day one was Caddy-in-a-container. The team chose host-Caddy presumably to dodge the volume-mount question for ACME state, but that's exactly the kind of "skip a small problem now, eat a bigger one later" trade we just paid for.

User-visible recommendation: **adopt Candidate C as the M1.x correction**. Future milestones should keep this property — ingress is a Docker concern, not a host concern.

### 6.3 The install doc lied to the operator

The install doc said "create the `decloud` network so Caddy can reach upstreams by container name" (`_docs/install.md:96-100`). Caddy could not, in fact, reach upstreams by container name, because Caddy was not on the network. The doc was telling the operator a story that was not true. Raymond owns rewriting §3 and §5 so the new doc reflects reality. Linus — please be ruthless about this in the doc review. Every sentence in the install doc has to be a true statement about what the system does.

### 6.4 We should add an integration test for the very first deploy

We deferred integration tests with explicit user agreement (`_ai/decisions/m1-test-strategy.md`). That was the right call for M1 ship, but the cost of the deferral landed today: the user's first real deploy hit the bug. **A 30-line integration test that builds a trivial container, deploys it, and `curl`s `https://<host>/healthz` end-to-end would have caught this before the user did.** I am not asking us to bring back the full `-tags integration` fleet — I am asking us to consider one happy-path end-to-end test as the M1.x companion to this fix. Joel, please weigh this in your tech expansion. If we add it, it becomes part of the manual-verification gate in §3.4 of plan-v2 instead of replacing the unit-tests-only stance.

---

## 7. Research info — facts the implementation agents will need

(Per Don's research-info standard. Read the cited files; I'm not pasting code snippets.)

- **Caddyfile generation**: `internal/caddy/generator.go`. `Generator.Generate(outPath, services)` writes the file at `outPath`. The body emits `reverse_proxy <ContainerName>:<Port>` per host. `ContainerName` comes from `svc.Config.State.ContainerName` else `"decloud-"+svc.Config.Name`. Stub at `internal/caddy/stub.go`.
- **Caddyfile reload pipeline**: `internal/caddy/reloader.go`. Two methods: `Validate(ctx, configPath)` and `Reload(ctx, configPath)`. Both shell out via `cmdFactory` (test seam at line 18). Production factory is `exec.CommandContext`.
- **Docker driver**: `internal/dockerdrv/driver.go` (interface), `internal/dockerdrv/cli_driver.go` (impl), `internal/dockerdrv/mocks/` (gomock). `RunRequest` (driver.go:31-38) currently lacks port-publishing and volume-mount fields — Rob will need to extend either the request or add a new request type for Caddy. `NetworkEnsure` exists. `ContainerIP` exists. There is no `ImagePull` — Rob adds one, calling `docker pull <ref>`.
- **Deploy orchestrator**: `internal/deploy/service.go`. Note `regenerateAndReload` at lines 302-325 — the atomic-rename + validate flow that the new reloader must keep working when validate becomes `docker exec`-based.
- **Container naming**: `internal/ids` (`ContainerName(name)` returns `"decloud-"+name`). New constant: `CaddyContainerName = "decloud-caddy"` lives in `internal/caddy` (NOT `internal/ids`, because `ids` is for service-derived names; the Caddy name is a fixed singleton).
- **Path config**: `internal/config/paths.go`. `Paths.CaddyfilePath` is `<root>/config/caddy/Caddyfile`. Bind-mount source. Volumes (`decloud_caddy_data`, `decloud_caddy_config`) are NOT under `/opt/decloud/`; they live in Docker's volume store. Document this in the install doc — backup story is "back up `/opt/decloud/` AND `docker volume export decloud_caddy_data | gzip > caddy_data.tar.gz`".
- **CLI surface**: `internal/cli/root.go:40-42` already mounts a `caddy` parent command with `reload` as its child. Adding `up` and `down` is symmetric. Use the existing `lifecycleFactory` pattern (`internal/cli/deploy_service.go:26-29`) but introduce a new `caddyManagerFactory` for the manager so the parallelism caveat at lines 21-25 stays accurate.
- **Exit codes**: `internal/cli/exit_codes.go`. `ExitCaddyReloadFail = 60`. Reuse for `caddy up` / `down` failures that originate in the reloader/manager surface. Add a new exit code only if a class of failure genuinely doesn't fit (e.g., image pull failure could plausibly be `ExitRunFail = 40` since it's a docker-driver call; lean on existing codes unless Joel argues otherwise).
- **Test seam discipline**: `_ai/gomock-inorder-sequencing.md`, `_ai/error-wrap-discipline.md`, `_ai/explicit-inputs-not-globals.md`, `_ai/cobra-init-pattern.md`. New code follows all four. Don't reinvent.

---

## 8. Acceptance criteria — when is this DONE

1. `decloud caddy up` exists, brings up `decloud-caddy` on the `decloud` network, publishes 80/443, mounts the Caddyfile and the two named volumes. Idempotent.
2. `decloud caddy down` exists, stops and removes `decloud-caddy`, leaves volumes intact. Idempotent.
3. `decloud caddy reload` continues to work; under the hood it now `docker exec`s into `decloud-caddy`.
4. `decloud deploy service ...` succeeds end-to-end on the user's actual host: TLS cert provisions, `curl https://live.durak.click/healthz` returns 200.
5. Caddy logs show no `dial tcp [2a03:...]` errors.
6. `docker network inspect decloud` shows BOTH `decloud-caddy` and the service container as members.
7. `_docs/install.md` §3 and §5 rewritten to match reality. Kevlin reviews docs for hallucinations per CLAUDE.md WORKFLOW STEP 3.4.
8. `_docs/usage.md` §1 quick start mentions `decloud caddy up`. §7 (caddy reload recovery) updated.
9. `_ai/decisions/caddy-runs-in-container.md` exists, lays out the decision and the rejected alternatives, and references this task.
10. `go test ./...` passes. `go vet ./...` clean. `gofmt -l .` empty. `go generate ./...` idempotent.
11. The user runs §5 manual verification on the actual host and reports green.

If any of 1-11 is missing at sign-off, the task is not done. No exceptions, no "we'll do it next iteration." Don't ship shit.

---

## 9. Hand-off

Joel: expand this into a full tech plan. Specifically resolve:

- Concrete shape of the `Driver` extension for image pull / port maps / volumes. One new method or extended `RunRequest`?
- Exact bind-mount path for the host Caddyfile dir into the container. Whole-dir bind, not single-file, so atomic rename works — confirm and document.
- Where the `decloud-caddy` container name constant lives. My suggestion is a new exported `caddy.ContainerName = "decloud-caddy"` constant in `internal/caddy`; if you want it in `ids`, justify.
- Whether to add the "first happy-path integration test" I called out in §6.4. Your call, but argue it explicitly so Linus and I can review the trade.
- Exit code mapping for `caddy up` failures. My default suggestion: image-pull failure → 40 (`ExitRunFail`), container-already-running → 0, network-ensure failure → 40, file-permission/mount setup → 70 (`ExitInternal`). Disagree freely.

Linus: when Joel hands you the tech plan, please be especially harsh about the "operator forgets `decloud caddy up` first" failure mode and the migration path from M1.0 host-Caddy installs. Those are the two places this fix can still bite users.

— Don
