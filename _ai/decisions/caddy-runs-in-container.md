# Caddy runs in a container on the `decloud` network

Originating task: `_tasks/2026-04-27-caddy-container-connection-refused/`. Plan v2: `005-don-plan-v2.md`. Tech plan v2: `006-joel-tech-plan-v2.md`. Linus approval: `007-linus-review-v2.md`.

## Context

M1.0 installed Caddy as a host systemd unit (`/etc/systemd/system/caddy.service`) running the `caddy` package binary against `/opt/decloud/config/caddy/Caddyfile`. The generated Caddyfile emits `reverse_proxy decloud-<service>:<port>` per host. That short name only resolves under Docker's embedded DNS server (`127.0.0.11`), which is only reachable from containers attached to a user-defined Docker network. A host process resolving `decloud-<service>` falls through to the host resolver, which returns the host's own AAAA record (or any wildcard match) and dials a port nothing is bound to.

The bug was reproducible on a Hetzner host with public IPv6: Caddy logged `dial tcp [2a03:f480:1:12::7b]:10001: connect: connection refused` while the service container was happily listening on `172.18.0.2:10001` inside the `decloud` bridge. The M1 tech plan had spotted the same gap for the readiness probe and patched it with `Driver.ContainerIP`, but never asked the corresponding question for Caddy. The asymmetry was not written down as a Decision, so future-Don missed it.

## Decision

**Caddy runs as a Decloud-managed Docker container named `decloud-caddy`, attached to the `decloud` bridge network. `decloud caddy up` brings it up, `decloud caddy down` takes it down, `decloud caddy reload` does its job via `docker exec` into that container.**

Image: `caddy:2`, hardcoded as `caddy.DefaultImage`. No flag, no env var, no TOML override in M1 — that comes when M3 introduces Viper and a real config file. Operators who need a pinned tag can `docker pull caddy:2.7.6 && docker tag caddy:2.7.6 caddy:2`.

Ports publish dual-stack: six `-p` entries covering `0.0.0.0:80/tcp`, `[::]:80/tcp`, `0.0.0.0:443/tcp`, `[::]:443/tcp`, `0.0.0.0:443/udp`, `[::]:443/udp`. UDP/443 is HTTP/3 over QUIC — without it, mobile clients silently fall back and the symptom looks like "TLS works but my phone is slow."

State persists in two named volumes the operator never names directly:

- `decloud_caddy_data` → `/data` — issued certs, OCSP staples, ACME account keys.
- `decloud_caddy_config` → `/config` — runtime config Caddy writes itself.

`/opt/decloud/config/caddy` bind-mounts read-only at `/etc/caddy` so the deployer's atomic-rename trick (`Caddyfile.tmp` → validate → rename) works against a host directory the operator can inspect.

`decloud caddy down` removes the container but **not** the volumes. Wiping ACME state requires explicit `docker volume rm`.

## Rejected alternatives

Two from Don's analysis, seven from Linus's review (`004-linus-review.md` §3). Each is named so the next person who has a clever idea can find this list.

- **A. Keep host Caddy; write bridge IPs into the Caddyfile.** Bridge IPs are unstable across container restarts. Forces a Caddy reload after every recreate and introduces a race. Loses Caddy+Docker-DNS auto-failover. Papers over the architectural smell.
- **B. Keep host Caddy; publish service container ports with `-p` to the host.** Contradicts the M1 network model documented in `_docs/usage.md` §6. Forces port-collision management in the deployer.
- **`host.docker.internal` from the host Caddy.** Linux Docker doesn't ship this; it works on Mac/Windows where the bridge is in a VM. Wrong target anyway — we want the service IP, not the host.
- **`--network host` for Caddy.** Loses Docker DNS resolution entirely. We'd be back to host resolver lookups for `decloud-<service>`, i.e. the original bug.
- **`--network container:<svc>` shared-namespace trick.** Pins Caddy to a single service container's lifetime. Caddy must outlive any individual upstream.
- **Sidecar pattern (one Caddy per service).** N copies of Caddy means N TLS certs being managed in N places. Multiplies the ACME state surface area.
- **`/etc/hosts` injection on the host.** Maintaining synchrony between bridge IPs and host hosts file is exactly what Docker's embedded DNS exists to avoid.
- **`--resolvers 127.0.0.11` in the host Caddy.** That address is only routed inside a container's network namespace. Not reachable from the host.
- **Host-local `dnsmasq` proxying to `127.0.0.11`.** Adds a second resolver to operate, debug, and document. Nuclear option for a problem solved by attaching Caddy to the bridge.
- **`extra_hosts` per service in compose-style configs.** No compose file in M1; would still require a reload after every recreate.

The pattern across these: any solution that keeps Caddy on the host either reintroduces the resolver bug, requires hand-managed state Docker already manages, or trades one architectural smell for another. Containerising Caddy is the only path where the Caddyfile means what it says.

## Consequences

- **Dual-stack publishing on the host.** Hosts with the kernel IPv6 stack disabled (`net.ipv6.conf.all.disable_ipv6=1`) fail loudly at `caddy up` time with `address family not supported by protocol`. M1 has no flag to opt out — re-enable IPv6 or skip Decloud on that host.
- **ACME state migration is now an operator concern.** Operators upgrading from M1.0 must copy `/var/lib/caddy/.local/share/caddy/...` into the `decloud_caddy_data` volume before running `caddy up`, or accept fresh issuances and the Let's Encrypt rate limits that go with them (50 certs / domain / week, 7-day recovery if tripped). Recipe lives inline in `_docs/install.md` §3.2.
- **Host Caddy is no longer supported.** `decloud caddy up` fails with port-bound errors if the M1.0 host Caddy is still running. The persistent-disable command is `systemctl mask caddy` (not `disable --now`, which a package upgrade silently undoes) or `apt-get remove -y caddy`.
- **The reloader switched seam from `cmdFactory` to `Driver.Exec`.** `internal/caddy/reloader.go` now `docker exec`s into `decloud-caddy`, translating host paths inside `/opt/decloud/config/caddy` to container paths under `/etc/caddy`. The legacy `cmdFactory` test seam was deleted.
- **Deploy errors when Caddy is down give a one-command recovery.** `internal/deploy/service.go` wraps the validate-leg and reload-leg failures with `service is registered and running but Caddy is not routing traffic; run 'decloud caddy up' (and then 'decloud caddy reload' if needed)`. The deployer does NOT pre-flight Caddy on every deploy — that adds coupling for a rare misuse case.
- **`caddy:2` floats.** Operators who pin a tag retag locally as the workaround until M3's config file lands. Documented in `_docs/install.md`.
- **Concurrent deploys (theoretical, M2+).** Two simultaneous deploys writing `Caddyfile.tmp`, validating, renaming, then both reloading is `last-rename-wins`. Caddy handles back-to-back reloads correctly. M1 is single-operator; flag if multi-operator becomes plausible.

## Forward-looking notes

- **M4 admin API.** Blue/green via Caddy's admin API needs the admin endpoint published inside the `decloud` network only (or via Unix socket on a shared volume). Either way the container model already in place is what M4 will reuse — there is no "containerise Caddy at M4" task.
- **Pinning the image tag.** When M3 introduces Viper, the obvious config knob is `caddy.image = "caddy:2.7.6"`. The `DefaultImage` constant becomes the fallback.

## Why this isn't in `_docs/`

`_docs/install.md` and `_docs/usage.md` document **how operators run the system**. This decision record documents **why the architecture is shaped this way**. Operators don't need this to install or deploy; future contributors need it to avoid relitigating `--network host` for the eighth time.
