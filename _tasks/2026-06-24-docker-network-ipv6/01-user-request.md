# User Request

implement a proper fix in the code: make sure the docker network is created with ipv6 support

## Context

Docker containers running on decloud have no working IPv6 egress, while the host system does.
Diagnosis showed the `decloud` Docker network was created IPv4-only (`EnableIPv6=false`,
subnet `172.18.0.0/16`), so containers get no IPv6 address or route at all.

The host runs Docker 29.4.1, where `ip6tables` is enabled by default, so enabling IPv6 on the
network with a private ULA subnet gives containers IPv6 egress via NAT66 (masquerade) using the
host's global address — no routed prefix required.

Root cause in code: `NetworkEnsure` in `internal/dockerdrv/cli_driver.go` runs a bare
`docker network create <name>` with no `--ipv6` flag.

## Goal

Make decloud create its Docker network with IPv6 support so containers have working IPv6 egress.
