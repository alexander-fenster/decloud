# Installation

Server-side installation for Decloud M1. The target is a single Linux server (Ubuntu LTS or Debian assumed; other distros work where Docker does, but a few command snippets below assume systemd). Operator runs every step over SSH as root or via `sudo`.

There is no `decloud` daemon in M1. The `decloud` binary is a one-shot CLI that the operator invokes ad hoc; it owns no background process, no listening port, and no host service unit. Container supervision is delegated to Docker via `--restart=unless-stopped`. Caddy itself runs as a Decloud-managed Docker container (`decloud-caddy`) on the shared `decloud` network — see [§3](#3-bring-up-caddy).

For day-to-day usage, see [`usage.md`](./usage.md).

## 1. Prerequisites

- Linux host with root access and outbound HTTPS to package mirrors and Docker Hub.
- DNS records for any public hostnames you intend to deploy must already point at the host so Caddy can complete ACME challenges. AAAA records are fine — `decloud caddy up` publishes 80/443 on both IPv4 and IPv6.
- Go toolchain on the host if you intend to `go install` the binary (otherwise build elsewhere and copy).
- The Docker daemon must run under systemd. Every container Decloud starts uses the journald log driver so logs survive container redeployment (see [`usage.md` §6](./usage.md#reading-logs-across-redeploys)); the daemon needs systemd to write to journald. The default Docker Engine install (`systemctl enable --now docker` in [§2](#2-install-docker)) satisfies this.

## 2. Install Docker

Follow the official Docker Engine instructions for your distribution: <https://docs.docker.com/engine/install/>. Do not paste the apt command list here — the upstream procedure changes faster than this doc.

Enable and start the daemon, then verify:

```sh
systemctl enable --now docker
docker run --rm hello-world
```

## 3. Bring up Caddy

Caddy runs as a Decloud-managed container named `decloud-caddy`, attached to the shared `decloud` Docker network. The container exists so Caddy can resolve service container names (`decloud-<service>`) via Docker's embedded DNS — a host-side Caddy cannot do that, and routing breaks on hosts with public IPv6 because the host resolver returns the host's own AAAA instead of the bridge IP.

There is **no host Caddy package**, no systemd unit, and no `caddy run` invocation. Install order is: install the `decloud` binary first ([§5](#5-install-the-decloud-binary)), then run:

```sh
decloud caddy up
```

`caddy up` is idempotent. It:

1. Ensures the `decloud` Docker network exists. On a fresh install (the network does not yet exist), Decloud creates it IPv6-enabled — see [§3.3](#33-the-decloud-network-and-ipv6).
2. Writes a stub `Caddyfile` if one is missing.
3. Pulls `caddy:2` and runs `decloud-caddy` with dual-stack publishing on `80/tcp`, `443/tcp`, and `443/udp` (the `443/udp` port is published but **inert** — HTTP/3 is disabled, so Caddy serves only HTTP/1.1 and HTTP/2), bind-mounting `/opt/decloud/config/caddy` read-only at `/etc/caddy`.
4. Persists ACME state and runtime config in two named volumes: `decloud_caddy_data` (`/data` — issued certs and OCSP staples) and `decloud_caddy_config` (`/config`).

Re-running `decloud caddy up` is safe: it logs `caddy already running` and exits 0.

To take Caddy down:

```sh
decloud caddy down
```

Volumes are **not** deleted by `caddy down`. To wipe ACME state, `docker volume rm decloud_caddy_data decloud_caddy_config` after `caddy down`.

### 3.1 Host firewall

Open `80/tcp` and `443/tcp` on any host firewall (`ufw`, `firewalld`, cloud security group). Decloud also publishes `443/udp`, but **HTTP/3 (QUIC) is disabled** — Caddy advertises only HTTP/1.1 and HTTP/2 (`servers { protocols h1 h2 }` in the generated Caddyfile). UDP/443 is therefore **published but inert**: nothing listens on it once HTTP/3 is off, so opening it on the firewall is optional and harmless. HTTP/3 was disabled deliberately because iPhone Safari over QUIC/UDP-443 was breaking connectivity in the field; see `_ai/decisions/caddy-runs-in-container.md`.

### 3.2 Migrating from the M1.0 host-Caddy install

Earlier M1 builds installed Caddy as a host systemd unit. If you followed those instructions, do this in order before running `decloud caddy up`:

```sh
# Persistently disable the host Caddy. `disable --now` alone is not enough —
# a package upgrade re-enables the unit. Use `mask` or fully remove the package.
systemctl disable --now caddy && systemctl mask caddy
# OR, to remove the package entirely:
apt-get remove -y caddy
```

**Recommended migration: copy ACME state into the named volume.** This preserves your already-issued Let's Encrypt certificates. Do this unless you have only one or two hostnames.

```sh
docker volume create decloud_caddy_data
docker run --rm \
  -v /var/lib/caddy/.local/share/caddy:/from \
  -v decloud_caddy_data:/to \
  alpine sh -c 'cp -a /from/. /to/'
decloud caddy up
```

If the host Caddy stored its data elsewhere (some packages put it under `/var/snap/caddy/common/.local/share/caddy` or `/etc/caddy/data`), substitute the correct source path. Verify with `find /var -name 'certificates' -type d` before copying.

**Alternative: cold restart without copying state.** Acceptable only if you have one or two hostnames. Caddy will obtain fresh Let's Encrypt certificates on first request per hostname.

```sh
decloud caddy up
```

Be aware of Let's Encrypt rate limits — they bite when you migrate many hostnames at once:

- **50 certificates per registered domain per week.** 30 services on subdomains of a single registered domain consume 30 of those 50.
- **5 duplicate certificates per identical SAN set per week.**
- The recovery window for the per-domain weekly cap is **7 days**. If you trip it, your TLS is broken until next week.

When in doubt, copy the volume.

### 3.3 The `decloud` network and IPv6

Every Decloud-managed container — `decloud-caddy` and each `decloud-<service>` — runs on a single shared Docker bridge network named `decloud`. Decloud creates this network the first time it is needed (during `decloud caddy up` or a deploy, whichever runs first on a fresh host).

On a **fresh install** — when the `decloud` network does not yet exist — Decloud creates it with IPv6 enabled, using the private ULA subnet `fd00:dec0:11d::/64` (a fixed, internal address range per [RFC 4193](https://www.rfc-editor.org/rfc/rfc4193)). Containers on the network get an IPv6 address in addition to their usual IPv4 address. Outbound IPv6 (egress) works via NAT66/masquerade: Docker hides the private ULA address behind the host's own global IPv6 address, the same way container IPv4 egress is masqueraded behind the host's IPv4 address. This lets a service container reach IPv6-only external hosts (for example, `curl -6 https://…`).

The IPv6 subnet is fixed and not configurable. It is an internal detail of how the bridge is built — because it is masqueraded, the exact prefix never appears outside the host. The IPv4 subnet is still auto-allocated by Docker from its default pool, exactly as before, so nothing about IPv4 addressing or the readiness probe (which reads the container's IPv4 bridge address) changes.

This affects **egress only**. There is no inbound IPv6 reachability to containers: external traffic still terminates at Caddy on the host (dual-stack on `80`/`443`, see [§3](#3-bring-up-caddy)), and Caddy reverse-proxies to each upstream over the internal network by container name. The ULA range is routed nowhere off-host.

**Host prerequisites for container IPv6 egress:**

- The host itself must have working IPv6 egress (a global IPv6 address with a working default route). The container's traffic is masqueraded behind it.
- Docker's `ip6tables` must be enabled so Docker installs the NAT66/masquerade rules. This is the default in Docker 27 and newer. If it is disabled, the network is still created IPv6-enabled but containers will not have working IPv6 egress.

**Already-existing networks are not changed.** If a `decloud` network already exists (for example, it was created by an older Decloud build that did not enable IPv6), Decloud leaves it exactly as it is — `decloud caddy up` and deploys are a no-op against an existing network and never recreate it. Decloud does **not** auto-upgrade an IPv4-only network to IPv6. Docker has no command to toggle IPv6 on an existing network, and recreating it during a deploy would be destructive (Caddy and every service container are attached to it).

To upgrade an existing IPv4-only `decloud` network to IPv6, the operator does it manually and out-of-band, during a maintenance window. The shape of the operation is: take Caddy down, remove the network, and let Decloud recreate it on the next `caddy up`:

```sh
decloud caddy down           # detaches decloud-caddy from the network
docker network rm decloud    # removes the IPv4-only network (fails if endpoints remain)
decloud caddy up             # recreates the network IPv6-enabled, restarts Caddy
```

Because `docker network rm` refuses to remove a network that still has attached containers, stop every running service container first (`decloud stop <name>` for each), or expect to restart them afterward. Plan this as a brief outage: while the network is gone, no service is reachable. Verify the result with `docker network inspect decloud` — `EnableIPv6` should read `true` and the `fd00:dec0:11d::/64` subnet should appear alongside the auto-allocated IPv4 subnet.

## 4. Create the `/opt/decloud/` tree

Decloud keeps all persistent state in one directory so a single backup path covers everything that matters. Create it with these exact modes:

```sh
mkdir -p /opt/decloud/config/services
mkdir -p /opt/decloud/config/jobs
mkdir -p /opt/decloud/config/caddy
mkdir -p /opt/decloud/secrets
mkdir -p /opt/decloud/state/deploys
mkdir -p /opt/decloud/logs

chmod 0755 /opt/decloud
chmod 0755 /opt/decloud/config
chmod 0755 /opt/decloud/config/services
chmod 0755 /opt/decloud/config/jobs
chmod 0755 /opt/decloud/config/caddy
chmod 0700 /opt/decloud/secrets
chmod 0755 /opt/decloud/state
chmod 0755 /opt/decloud/state/deploys
chmod 0755 /opt/decloud/logs
```

`secrets/` must be `0700`. Per-service secrets files are written `0600` inside it; the registry's loader rejects the service if the modes are wrong.

`state/deploys/` is created here but no M1 code populates it. M6 will write source bundles there for backup.

To use a different root, set `DECLOUD_ROOT=/some/other/path` in the operator's environment or pass `--config-root` on every invocation. The default is `/opt/decloud`.

On RHEL-family hosts with SELinux enforcing, the bind mount of `/opt/decloud/config/caddy` into the Caddy container needs the right context: `chcon -Rt container_file_t /opt/decloud/config/caddy`. SELinux is not a tier-1 supported configuration in M1.

## 5. Install the `decloud` binary

Build with the Go toolchain and place the binary on `PATH`:

```sh
go install github.com/alexander-fenster/decloud/cmd/decloud@latest
install -m 0755 "$(go env GOPATH)/bin/decloud" /usr/local/bin/decloud
```

If the host has no Go toolchain, build elsewhere and `scp` the binary to `/usr/local/bin/decloud`:

```sh
GOOS=linux GOARCH=amd64 go build -o decloud ./cmd/decloud
scp decloud root@host:/usr/local/bin/decloud
ssh root@host chmod 0755 /usr/local/bin/decloud
```

Verify the install:

```sh
decloud --help
```

The output must list `caddy`, `deploy`, `logs`, `restart`, `start`, `status`, `stop`, and `unregister` subcommands. There is no `decloud daemon`, no `decloud bootstrap`, and no `systemctl enable decloud` — M1 deliberately ships no host service for Decloud itself.

## 6. Bootstrap order and first deploy

In order on a fresh host:

```sh
# 1. Create the /opt/decloud tree (§4).
# 2. Install the binary (§5).
# 3. Bring Caddy up:
decloud caddy up

# 4. Deploy your first service (see usage.md):
decloud deploy service --name myservice --host myservice.example.com --port 8080 ./myservice
```

`decloud caddy up` writes the stub `Caddyfile` if one is missing, so the first deploy has something to regenerate against.

## 7. Troubleshooting

### Ports 80/443 already bound

`decloud caddy up` fails with:

```
caddy: up failed: ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent
```

Something else is listening on the public ports. Almost always a leftover host Caddy from the M1.0 install. The error already names the recovery commands; run one of:

```sh
systemctl disable --now caddy && systemctl mask caddy
# OR
apt-get remove -y caddy
```

`systemctl disable --now caddy` alone is **not** durable — package upgrades re-enable the unit. Use `mask` or fully remove the package.

### IPv6 listener fails to bind

`decloud caddy up` fails with stderr containing `listen tcp [::]:80: socket: address family not supported by protocol`. The raw `docker run` stderr is surfaced as-is; it typically reads similar to:

```
docker: Error response from daemon: ...listen tcp [::]:80: socket: address family not supported by protocol...
```

The kernel has IPv6 disabled (`net.ipv6.conf.all.disable_ipv6=1`). Re-enable IPv6, or accept that this host cannot serve IPv6 clients. M1 does not have a flag to opt out of dual-stack publishing.

### Caddy can't reach an upstream

If Caddy logs `dial tcp [<public IPv6>]:<port>: connect: connection refused` after a deploy, `decloud-caddy` is not on the `decloud` network. Verify with:

```sh
docker network inspect decloud
```

`decloud-caddy` and the service container must both appear under `Containers`. If Caddy is missing, run `decloud caddy down && decloud caddy up`.

### Caddy is not routing after a deploy

Exit 60 with text `service is registered and running but Caddy is not routing traffic; run 'decloud caddy up'` means the deploy wrote the registry and the container is healthy, but Caddy is down. Run `decloud caddy up`, then `decloud caddy reload` if the up command reports `caddy already running`.

### Container IPv6 egress does not work

If a service container cannot reach IPv6-only external hosts (for example `curl -6` fails from inside the container), check, in order:

1. **The host has working IPv6 egress.** Confirm from the host itself with `curl -6 https://…` or `ping6`. Container egress is masqueraded behind the host's global IPv6 address; if the host has none, containers cannot reach IPv6.
2. **The network is actually IPv6-enabled.** Run `docker network inspect decloud` and confirm `EnableIPv6` is `true` and the `fd00:dec0:11d::/64` subnet is present. If it reads `false`, the network predates IPv6 support and was left untouched — upgrade it out-of-band as described in [§3.3](#33-the-decloud-network-and-ipv6).
3. **Docker's `ip6tables` is enabled** so Docker installs the NAT66/masquerade rules. It is on by default in Docker 27 and newer. Without it, the container has an IPv6 address but no egress.

## 8. License

Decloud is licensed under the MIT License. See the top-level [`LICENSE`](../LICENSE) file for the full text.

## 9. Next steps

Deploy your first service. See [`usage.md`](./usage.md).
