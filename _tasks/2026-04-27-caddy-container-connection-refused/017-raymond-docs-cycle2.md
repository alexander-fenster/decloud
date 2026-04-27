# 017 — Raymond's Cycle-2 Docs Report

Author: Raymond Chen (doc writer)
Date: 2026-04-27
Status: Cycle-2 EXECUTION 3.3 complete. Three doc edits landed per Joel §2.1, §3.1, §5.1 — with Item 2 reworded per the user's direct guidance to surface raw `docker run` stderr (not the wrapped chain).

## Reading log

1. `_tasks/2026-04-27-caddy-container-connection-refused/013-joel-tech-plan-cycle2.md` — §2.1 (IPv6 reword), §3.1 (ports reword), §5.1 (backlog item).
2. `_tasks/2026-04-27-caddy-container-connection-refused/016-rob-implementation-cycle2.md` — verified the rendered ports-bound error string byte-for-byte.
3. `internal/caddy/manager.go:25,97` — `ErrCaddyUp.Error() = "caddy: up failed"`; literal in `fmt.Errorf` confirmed.
4. `_docs/install.md:168-200` — current state of troubleshooting section.
5. `_ai/m1x-backlog.md` — existing entry style and item count (1-5).

## File edits

### Edit 1 — `_docs/install.md` Item 3 (ports-bound, lines 170-184)

**Before:**

```markdown
### Ports 80/443 already bound

```
caddy up: ports 80/443 already in use
```

Something else is listening on the public ports. Almost always a leftover host Caddy from the M1.0 install. Make the disable persistent:

```sh
systemctl disable --now caddy && systemctl mask caddy
# OR
apt-get remove -y caddy
```

`systemctl disable --now caddy` alone is **not** durable — package upgrades re-enable the unit. Use `mask` or fully remove the package.
```

**After:**

```markdown
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
```

**Verification:** `grep -F` of the literal recovery-naming substring matches both `_docs/install.md` and `internal/caddy/manager.go::Up`. Byte-for-byte match confirmed. The `caddy: up failed:` prefix comes from `ErrCaddyUp.Error()` interpolated via `%w`; the rest is the verbatim literal from `internal/caddy/manager.go:97`.

### Edit 2 — `_docs/install.md` Item 2 (IPv6, lines 186-192)

**Before:**

```markdown
### IPv6 listener fails to bind

```
caddy up: docker run: listen tcp [::]:80: socket: address family not supported by protocol
```

The kernel has IPv6 disabled (`net.ipv6.conf.all.disable_ipv6=1`). Re-enable IPv6, or accept that this host cannot serve IPv6 clients. M1 does not have a flag to opt out of dual-stack publishing.
```

**After:**

```markdown
### IPv6 listener fails to bind

`decloud caddy up` fails with stderr containing `listen tcp [::]:80: socket: address family not supported by protocol`. The raw `docker run` stderr is surfaced as-is; it typically reads similar to:

```
docker: Error response from daemon: ...listen tcp [::]:80: socket: address family not supported by protocol...
```

The kernel has IPv6 disabled (`net.ipv6.conf.all.disable_ipv6=1`). Re-enable IPv6, or accept that this host cannot serve IPv6 clients. M1 does not have a flag to opt out of dual-stack publishing.
```

**Why this shape (deviation from Joel §2.1 prose):** The user's direct EXECUTION 3.3 instructions said "Reword to show the real raw docker stderr the operator actually sees when an IPv6 publish fails. … if you're unsure of the exact raw stderr, write text that says 'the raw `docker run` stderr is surfaced as-is; it typically reads similar to: `docker: Error response from daemon: ...`' rather than fabricate a precise one-line example." That overrides Joel's §2.1 specific prose. The new text:

1. Surfaces the IPv6-specific substring an operator can grep for (`listen tcp [::]:80: socket: address family not supported by protocol`) — present in `internal/caddy/manager.go::Up`'s fall-through wrap chain via the driver's `; stderr=%q` on the wrapped error.
2. Uses ellipses (`...`) inside the fenced block to make clear the surrounding daemon-prefix text is variable; does not fabricate the exact docker version's full stderr line.
3. Keeps the recovery prose byte-identical — the kernel-level fix is unchanged.

The old example (`caddy up: docker run: listen tcp [::]:80: ...`) was fabricated — that exact string was never emitted by `Manager.Up`; the real wrap is `caddy: up failed: docker run: docker run: exit status N; stderr="..."`. The new phrasing is honest about substring-match semantics without committing to a precise doubled-wrap chain that may shift if the wrap layers ever consolidate.

### Edit 3 — `_ai/m1x-backlog.md` (append item #6)

**Before:** File had items 1-5, ending at the `## Maintenance note` section.

**After:** New item #6 inserted between item 5 and `## Maintenance note`:

```markdown
## 6. Docker-compose-based smoke integration test for M1 deploy + Caddy ingress

**Where:** No file yet. Likely lives at `internal/integration/` (new package) or `_test/integration/`. Test invokes `decloud caddy up`, `decloud deploy service` against a real Docker daemon (CI runner with Docker-in-Docker, or a tagged opt-in test that requires `DECLOUD_INTEGRATION=1`), asserts a real HTTP request through Caddy reaches a real upstream container.

**Why deferred:** Per `_ai/decisions/m1-test-strategy.md`, M1 is unit-tests-only against the gomock'd `Driver`. The bridge-DNS resolution path is locked architecturally by the `decloud-caddy`-on-`decloud`-network design (`_ai/decisions/caddy-runs-in-container.md`); the only thing a real-Docker test catches that unit tests miss is "is our argv actually accepted by docker?", and the argv-shape tests in `internal/dockerdrv/cli_driver_test.go` lock that argv byte-for-byte. Deferred from the caddy-container-connection-refused task per `_ai/decisions/m1-test-strategy.md`.

**Fix shape:** New `integration_test.go` build-tagged with `//go:build integration`, requires `DECLOUD_INTEGRATION=1` to run, brings up Caddy, deploys a one-line nginx service, curls through Caddy, asserts 200 OK with nginx body. Tear down both containers and the network on completion. Cleanup must be idempotent (test failures must not leave dangling containers). M2 material; M2 is also the milestone where reloader stderr `%q` quoting gets revisited, so the integration test naturally covers that improvement too.

**Originator:** Joel §8.5 of `_tasks/2026-04-27-caddy-container-connection-refused/006-joel-tech-plan-v2.md`. Acknowledged by Don in `_tasks/2026-04-27-caddy-container-connection-refused/012-don-final-review.md` §5.1. Reaffirmed in `_tasks/2026-04-27-caddy-container-connection-refused/013-joel-tech-plan-cycle2.md` §5.
```

**Format match:** four bold-prefixed sections (Where / Why deferred / Fix shape / Originator) — matches items 1-5's existing entry style.

**Heading wording:** the user's EXECUTION 3.3 instructions specified the bullet wording as "Track the docker-compose-based smoke integration test for the M1 deploy + Caddy ingress path (deferred from the caddy-container-connection-refused task per `_ai/decisions/m1-test-strategy.md`)." That sentence is the integration-test bullet's intent; in the file's existing entry-style (Where/Why/Fix/Originator), I expanded it into the four-section form so it reads consistently with items 1-5 and carries the per-section detail Joel §5.1 spelled out. The user-supplied bullet text is preserved in spirit and content (the "deferred from the caddy-container-connection-refused task per `_ai/decisions/m1-test-strategy.md`" phrase appears verbatim in the **Why deferred** section).

## Verification

- **Byte-for-byte match for Item 3:** `grep -F "ports 80/443 already in use; if you ran the M1.0 install, run 'systemctl disable --now caddy && systemctl mask caddy' or 'apt-get remove -y caddy' to make the change persistent"` matches both `_docs/install.md` and `internal/caddy/manager.go`. C2-5 satisfied.
- **Item 2:** displayed string is presented as substring inside stderr, not as a fabricated wrapped chain. C2-4 satisfied.
- **Item 5:** `grep -c "^## 6\. " _ai/m1x-backlog.md` returns 1. C2-8 satisfied.

## Files touched

- `_docs/install.md` — reworded Items 2 (IPv6) and 3 (ports-bound) in the troubleshooting section.
- `_ai/m1x-backlog.md` — appended item #6 (integration test).

## Acceptance criteria coverage (Joel §9, Raymond's remit)

- C2-4 (IPv6 example string is the substring inside stderr, not a fabricated wrapped chain): satisfied.
- C2-5 (ports example matches the EXACT string emitted by the new branch): satisfied via byte-for-byte match.
- C2-8 (`_ai/m1x-backlog.md` item #6 exists): satisfied.

— Raymond
