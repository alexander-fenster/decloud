# Installation

Server-side installation for Declouding M1. The target is a single Linux server (Ubuntu LTS or Debian assumed; other distros work where Docker and Caddy do, but the systemd snippets below assume systemd). Operator runs every step over SSH as root or via `sudo`.

There is no `decloud` daemon in M1. The `decloud` binary is a one-shot CLI that the operator invokes ad hoc; it owns no background process, no listening port, and no host service unit. Container supervision is delegated to Docker via `--restart=unless-stopped`.

For day-to-day usage, see [`usage.md`](./usage.md).

## 1. Prerequisites

- Linux host with systemd, root access, outbound HTTPS to package mirrors and Docker Hub.
- DNS records for any public hostnames you intend to deploy must already point at the host so Caddy can complete ACME challenges.
- Go toolchain on the host if you intend to `go install` the binary (otherwise build elsewhere and copy).

## 2. Install Docker

Follow the official Docker Engine instructions for your distribution: <https://docs.docker.com/engine/install/>. Do not paste the apt command list here — the upstream procedure changes faster than this doc.

Enable and start the daemon, then verify:

```sh
systemctl enable --now docker
docker run --rm hello-world
```

## 3. Install Caddy

Follow the official Caddy install instructions: <https://caddyserver.com/docs/install>. After installing the package, you must give Caddy its own systemd unit pointing at the Declouding-managed Caddyfile. Do not enable the default `caddy.service` that ships with the package.

Create `/etc/systemd/system/caddy.service`:

```ini
[Unit]
Description=Caddy
Documentation=https://caddyserver.com/docs/
After=network-online.target
Wants=network-online.target

[Service]
User=caddy
Group=caddy
ExecStart=/usr/bin/caddy run --environ --config /opt/declouding/config/caddy/Caddyfile --adapter caddyfile
ExecReload=/usr/bin/caddy reload --config /opt/declouding/config/caddy/Caddyfile --adapter caddyfile
TimeoutStopSec=5s
LimitNOFILE=1048576
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
```

Reload systemd and enable the unit, but do not start it yet:

```sh
systemctl daemon-reload
systemctl enable caddy
```

Caddy will fail to start until the Caddyfile exists. The first `decloud deploy service` writes a stub Caddyfile, after which `systemctl start caddy` succeeds.

The `caddy` binary must be on the operator's `PATH`. The deployer invokes `caddy validate` before every reload; if `which caddy` fails, deploys fail at exit code 60 (`ExitCaddyReloadFail`).

## 4. Create the `/opt/declouding/` tree

Declouding keeps all persistent state in one directory so a single backup path covers everything that matters. Create it with these exact modes:

```sh
mkdir -p /opt/declouding/config/services
mkdir -p /opt/declouding/config/jobs
mkdir -p /opt/declouding/config/caddy
mkdir -p /opt/declouding/secrets
mkdir -p /opt/declouding/state/deploys
mkdir -p /opt/declouding/logs

chmod 0755 /opt/declouding
chmod 0755 /opt/declouding/config
chmod 0755 /opt/declouding/config/services
chmod 0755 /opt/declouding/config/jobs
chmod 0755 /opt/declouding/config/caddy
chmod 0700 /opt/declouding/secrets
chmod 0755 /opt/declouding/state
chmod 0755 /opt/declouding/state/deploys
chmod 0755 /opt/declouding/logs
```

`secrets/` must be `0700`. Per-service secrets files are written `0600` inside it; the registry's loader rejects the service if the modes are wrong.

`state/deploys/` is created here but no M1 code populates it. M2 will write source bundles there for backup.

To use a different root, set `DECLOUD_ROOT=/some/other/path` in the operator's environment or pass `--config-root` on every invocation. The default is `/opt/declouding`.

## 5. Create the shared Docker network

Caddy and every service container join one shared bridge network so Caddy can reach upstreams by container name. Create it once:

```sh
docker network create decloud
```

The network name is the literal `decloud`. The default bridge driver is required — do not pass `--driver`. The readiness probe relies on host-to-container reachability over that bridge.

## 6. Install the `decloud` binary

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

The output must list `caddy`, `deploy`, `logs`, `restart`, `start`, `status`, `stop`, and `unregister` subcommands. There is no `decloud daemon`, no `decloud bootstrap`, and no `systemctl enable decloud` — M1 deliberately ships no host service for Declouding itself.

## 7. License

This repository does not yet declare a license. If you intend to redistribute the binary or use it in a context that requires explicit license grant, ask the maintainer before doing so.

## 8. Next steps

Deploy your first service. See [`usage.md`](./usage.md).
