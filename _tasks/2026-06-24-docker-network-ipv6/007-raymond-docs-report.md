# Raymond — Docs Report: Docker network IPv6 support

## Scope

Documented exactly the change in the code diff (`git diff main...HEAD` on
`internal/dockerdrv/cli_driver.go` + `internal/deploy/service.go`):

- `NetworkEnsure` now creates the `decloud` bridge with
  `docker network create --ipv6 --subnet fd00:dec0:11d::/64 <name>` on a FRESH
  install (network absent). NAT66/masquerade egress, hidden behind the host's
  global IPv6 address.
- The `docker network inspect` early-return is unchanged: an already-existing
  network is a strict no-op and is NOT auto-upgraded.
- IPv4 subnet still auto-allocated (no IPv4 `--subnet`, no `--driver`), so
  `ContainerIP`'s IPv4 read and the readiness probe are unaffected.
- `deploy/service.go` swapped two `"decloud"` literals for `caddy.NetworkName`
  (pure refactor; not operator-facing, so not documented in `_docs/`, recorded
  in `_ai/`).

No hallucinated commands/flags/paths: every command (`docker network inspect`,
`docker network create --ipv6 --subnet fd00:dec0:11d::/64`, `docker network rm`,
`docker network inspect decloud`, `decloud caddy down`/`up`, `decloud stop`) and
the subnet literal `fd00:dec0:11d::/64` are taken verbatim from the diff and the
plan. The "manual out-of-band upgrade" recipe describes an OPERATOR action, not
anything Decloud does — explicitly so, per the no-auto-upgrade constraint.

Note: `_docs/` in this repo is plain Markdown (`install.md`, `usage.md`), NOT a
Next.js/JSX app — there is no `next build` step. No build to run.

## Files changed

### `_docs/install.md` (operator-facing)

1. §3 "Bring up Caddy", network-creation bullet (was: "Ensures the `decloud`
   Docker network exists."): added that a fresh install creates the network
   IPv6-enabled, with a cross-link to the new §3.3.
2. **New §3.3 "The `decloud` network and IPv6"** inserted between §3.2 and §4.
   Covers: single shared bridge; fresh-install IPv6 with ULA
   `fd00:dec0:11d::/64` (RFC 4193 link); NAT66/masquerade egress behind the
   host GUA; egress-only (no inbound IPv6 to containers — inbound terminates at
   Caddy); fixed/non-configurable subnet; IPv4 still auto-allocated so the
   readiness probe is unchanged. Host prerequisites: host must have working IPv6
   egress; Docker `ip6tables` (default-on in Docker 27+). **Explicit
   no-auto-upgrade paragraph** + a manual, out-of-band
   `caddy down` → `docker network rm decloud` → `caddy up` recipe framed as an
   operator action during a maintenance window, with the `network rm` endpoint
   caveat and an `inspect` verification step.
3. New troubleshooting entry "Container IPv6 egress does not work" (§7) — a
   three-step ordered checklist: host IPv6 egress, `inspect` shows
   `EnableIPv6: true` + the ULA subnet (else upgrade out-of-band), `ip6tables`
   enabled.

### `_docs/usage.md` (operator-facing)

4. §5 deploy step 0 (network-ensure): noted fresh hosts get the network
   IPv6-enabled with a cross-link to install §3.3, and that an existing network
   is left untouched.

### `_ai/decisions/caddy-runs-in-container.md` (decision record)

5. **New "Amendment 2026-06-24"** before "Forward-looking notes": the create
   flags + ULA const `decloudIPv6Subnet`; egress-only/NAT66; no IPv4 `--subnet`
   / no `--driver` and why; the deliberate NO-reconcile decision with the
   load-bearing gotcha (**Docker has no command to toggle `EnableIPv6` on an
   existing network**) and a "do NOT add rm+recreate back without a decision"
   warning; subnet-is-not-a-config-knob rationale; the `caddy.NetworkName`
   literal cleanup; pointers to the operator docs.

### `_ai/MEMORY.md` (index)

6. Extended the `caddy-runs-in-container.md` index line with a one-line
   2026-06-24 amendment summary (create flags, fresh-only, no auto-upgrade,
   `ContainerIP` unchanged, `caddy.NetworkName` cleanup).

## Verification

- `git diff` confirms only the four files above changed; every non-trivial edit
  in the diff is listed in this report.
- Anchor `#33-the-decloud-network-and-ipv6` matches the repo's existing
  slug convention (e.g. `## 3. Bring up Caddy` → `#3-bring-up-caddy`: dots and
  periods stripped, spaces → dashes, lowercased).
- README.md left unchanged: its "Architecture in 60 seconds" is a high-level
  overview that does not claim IPv4-only; the egress detail belongs in
  install.md, not the summary.
