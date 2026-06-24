# Plan: Docker network IPv6 support (Don)

## Scope (narrowed by user 2026-06-24)

User decision: "no need to upgrade — I upgraded my host manually, I only want
the clean installs to support ipv6." So this is now a MINIMAL change:

- ONLY the fresh-create path gains IPv6.
- NO reconciliation of pre-existing IPv4-only networks.
- NO `EnableIPv6` inspection, NO `docker network rm`, NO recreate, NO
  "operator must recreate" notes. The idempotent early-return stays exactly as
  it is today.

This deliberately drops the destructive rm+create path. On a host whose
`decloud` network already exists, `NetworkEnsure` remains a no-op — which is
correct, because the maintainer has already upgraded that host by hand. Fresh
installs (network absent) get IPv6 from the create flags.

## Problem

The `decloud` Docker network is created IPv4-only. Containers get no IPv6
address or route, so IPv6 egress fails entirely while the host's own IPv6
works. We make the fresh-create path pass `--ipv6` and a fixed ULA subnet,
giving containers IPv6 egress via NAT66/masquerade (Docker 29.x, `ip6tables`
on by default). No routed prefix needed or available (host has only an on-link
`/120`).

## Execution trace (verified, not assumed)

Single chokepoint: `(*cliDriver).NetworkEnsure` in
`internal/dockerdrv/cli_driver.go:176-184`.

```
NetworkEnsure(ctx, name):
  176  run: docker network inspect <name>
  177    err == nil  -> return nil          // EARLY RETURN: network exists, untouched (UNCHANGED)
  180  run: docker network create <name>    // BARE create — THIS is what we change
  181    err != nil  -> wrap and return
  183  return nil
```

We change ONLY line 180: add `--ipv6 --subnet <ULA>`. Lines 176-179 (inspect +
early return) stay byte-for-byte as they are.

Two callers, both fire before any container is launched:
- `internal/caddy/manager.go:66` — `NetworkEnsure(ctx, NetworkName)` where
  `NetworkName = "decloud"` (`manager.go:20`).
- `internal/deploy/service.go:159` — `NetworkEnsure(ctx, "decloud")` (string
  literal; also a second literal at `:163` in the log line).

`(*cliDriver).ContainerIP` (`cli_driver.go:186-203`) reads
`.NetworkSettings.Networks.decloud.IPAddress` — the IPv4 field. Docker keeps
that and adds `GlobalIPv6Address` separately, so adding IPv6 does NOT change it.
**Verified: the readiness probe is unaffected.**

Existing tests constraining this code (`internal/dockerdrv/cli_driver_test.go`):
- `TestCLIDriver_NetworkEnsureWhenAbsent` (line 202): asserts a `create` happens
  and that `--driver` is NOT passed (readiness probe needs the default bridge).
  Our change keeps the default bridge; this test gets EXTENDED to also assert
  `--ipv6` and `--subnet` are present.
- `TestCLIDriver_NetworkEnsureWhenPresent` (line 218): asserts `create` is NOT
  called when inspect succeeds. **UNCHANGED** — our change does not touch the
  present path, so this test passes as-is and stays as-is.

## Design decisions

### 1. Hardcode `--ipv6` with a fixed ULA subnet. NOT configurable.

CLAUDE.md makes Viper/TOML available; it does not mandate configurability. The
ULA subnet is an internal implementation detail of how this one driver builds
its private bridge — not a per-service knob.

- `Driver.NetworkEnsure` signature is `(ctx, networkName string)`. No existing
  plumbing for network-creation options; `RunSpec.Network`
  (`internal/registry/types.go:53`) controls which network a service joins, not
  how it is created. Threading a subnet through would churn the interface, both
  call sites, the caddy config, and the registry for zero benefit and real
  footgun potential (invalid subnet breaks every fresh install).
- ULA (`fd00::/8`, RFC 4193) is private-by-design, routed nowhere. One fixed
  `/64` is correct. NAT66 hides it behind the host GUA regardless of value.
- **Chosen subnet: `fd00:dec0:11d::/64`** — pronounceable mnemonic, in ULA
  range, ample. Unexported const in `cli_driver.go`, NOT in `config`.
- **Only the IPv6 `--subnet` is pinned.** Do NOT pass an IPv4 `--subnet`;
  omitting it lets Docker auto-allocate IPv4 from its default pool exactly as
  today, so `ContainerIP`'s IPv4 read is unchanged and there is no risk of
  colliding with a host-used range.

### 2. Replace the `"decloud"` literals in deploy/service.go with `caddy.NetworkName`. KEEP.

`deploy/service.go` already imports `caddy` (`service.go:12`, uses
`caddy.Generator`/`caddy.Reloader`/`caddy.WriteStubIfMissing`) and `caddy` does
NOT import `deploy`. **Verified cycle-free.** Replacing the literals at
`:159` (the `NetworkEnsure` arg) and `:163` (the log field) with
`caddy.NetworkName` consolidates the network's identity onto its single source
of truth.

This is NOT scope creep even though the main change is now tiny: it removes two
magic strings that name the exact resource this task is modifying. If the
network name ever changes, three places must agree; this makes it one. It is a
<=5-minute, cycle-free maintenance fix directly on the concern in hand. KEEP IT.

(Out of scope: any broader `NetworkName` audit elsewhere. Only these two
literals, only because they sit on the path we are touching.)

## Implementation steps (TDD)

### Kent (tests) — `internal/dockerdrv/cli_driver_test.go`

Use the existing `scriptedFactory`/`recordingFactory` seam (lines 25-48).

1. EXTEND `TestCLIDriver_NetworkEnsureWhenAbsent`: keep the existing assertions
   (a `create` call happens; `--driver` is NOT present); ADD assertions that the
   create args contain `--ipv6` and `--subnet` with the ULA const value.
2. LEAVE `TestCLIDriver_NetworkEnsureWhenPresent` unchanged — it already pins the
   no-op-on-existing behavior we are preserving.

These are externally-meaningful behavior assertions (IPv6 egress on fresh
install, no spurious create on existing), not change-detector tests.

### Rob (implementation) — `internal/dockerdrv/cli_driver.go`

- Add unexported const: `decloudIPv6Subnet = "fd00:dec0:11d::/64"`.
- In `NetworkEnsure`, change line 180 to
  `docker network create --ipv6 --subnet <decloudIPv6Subnet> <name>`. Leave
  176-179 untouched. Keep the existing error-wrap style. A one-line comment on
  WHY the ULA + NAT66 (not obvious) is warranted; no obvious comments.

### Rob (cleanup) — `internal/deploy/service.go`

- Replace `"decloud"` at `:159` and `:163` with `caddy.NetworkName`.

### Raymond (docs)

- `_docs/`: state that the `decloud` network is created IPv6-enabled on FRESH
  installs (ULA `fd00:dec0:11d::/64`, NAT66 egress, no inbound IPv6 reachability
  to containers — inbound still terminates at Caddy on the host). Explicitly note
  that an ALREADY-EXISTING IPv4-only network is left untouched and is upgraded by
  the operator out-of-band if desired (we do not auto-recreate).
- `_ai/`: record the create-flags, the ULA value, and the deliberate
  no-reconcile decision (and the "no Docker command toggles EnableIPv6" gotcha,
  as the reason auto-upgrade was non-trivial and intentionally skipped).

## Acceptance criteria

- On a FRESH host, `decloud caddy up` / first `deploy` creates a network where
  `docker network inspect decloud` shows `EnableIPv6: true` and the
  `fd00:dec0:11d::/64` IPv6 subnet alongside an auto-allocated IPv4 subnet.
- A container on that network can `curl -6` an external IPv6 host.
- `ContainerIP` still returns the IPv4 address (readiness probe unbroken).
- On a host where `decloud` already exists, `NetworkEnsure` remains a no-op (no
  `create`), regardless of that network's IPv6 state.
- `go test ./...` green. `gofmt` clean.

## Edge cases / risks

- **No reconcile of existing IPv4-only network**: intentional per user. The
  maintainer already upgraded the live host by hand; auto-recreate would be a
  destructive rm+create in the deploy hot path for no benefit. Dropped.
- **IPv4 subnet collision**: avoided by NOT pinning IPv4 `--subnet`.
- **Daemon without ip6tables / older Docker**: host is 29.4.1 with ip6tables on
  by default. A future host lacking it would create the network but lack NAT66
  egress — out of scope; noted in `_ai/`.
- **Integration testing**: no Docker on the dev box (MEMORY.md); maintainer runs
  real egress verification on the Linux host. Unit tests cover arg construction
  via the scripted seam.

## Research facts for downstream agents

- Test seam: `cmdFactory` (`cli_driver.go:19`), `recordingFactory`/
  `scriptedFactory` (`cli_driver_test.go:25-48`), `recordedCmd{Name, Args}`
  (`cli_driver_test.go:20`). `scriptedFactory` runs `/bin/sh -c <script>`.
- `Driver` interface: `internal/dockerdrv/driver.go:113-134`. `NetworkEnsure`
  signature is UNCHANGED — no interface change, gomock needs NO regen.
- Network name const: `caddy.NetworkName = "decloud"` (`caddy/manager.go:20`).
- Callers: `caddy/manager.go:66`; `deploy/service.go:159` (+log literal `:163`).
- `deploy` imports `caddy` (`service.go:12`); `caddy` does NOT import `deploy`
  — `caddy.NetworkName` usage in deploy is cycle-free.
- Config/TOML lives in `internal/registry`; `internal/config/paths.go` is
  filesystem paths only. Do NOT add the subnet to either.

## ADDENDUM — adjudication of Kevlin vs Linus (Don, post-execution PLAN)

Reviewers split:
- Kevlin (`008-kevlin-review.md`): CHANGES REQUESTED — three more `Network:
  "decloud"` literals remain in the SAME file (`service.go:254`, `:324`, `:387`)
  naming the SAME network; either swap all three or soften the "single source of
  truth" claim.
- Linus (`008-linus-impl-review.md`): APPROVED — claims those three are
  "`RunSpec.Network` struct fields populated from registry config, a different
  concern, correctly left alone."

**I read the three lines myself. Ground truth (verified, not relayed):**
- `service.go:254` — `dockerdrv.RunRequest{… Network: "decloud" …}` (deploy path).
  Raw literal. The network the container JOINS — the same one `NetworkEnsure`
  just created.
- `service.go:324` — `registry.RunSpec{… Network: "decloud" …}`. Raw literal.
  This is the value PERSISTED into the registry config on disk.
- `service.go:387` — `dockerdrv.RunRequest{… Network: "decloud" …}` (redeploy
  path). Raw literal. Note: the rest of this struct reads `prev.Config.*`
  (persisted), but `Network` is hardcoded, NOT read from `prev.Config.Run.Network`.

**Verdict: Kevlin is RIGHT. Linus approved on a false premise.** None of the three
is "populated from registry config" at the point of assignment — all three are
hardcoded string literals. Line 324 is itself the literal that WRITES the
registry value. Linus's factual basis is wrong; I do not defer to a verdict whose
ground truth doesn't hold.

**Decision: swap all three (254, 324, 387) to `caddy.NetworkName`.** Reasons:
1. The task's stated correctness argument IS "one source of truth for the network
   name." With four literals across one file, that claim is FALSE. I finish the
   argument; I do not soften the docs to fit an incomplete change. Choosing the
   prose to match a shortcut is backwards.
2. Verified cycle-free: `deploy` imports `caddy` (`service.go:12`), `caddy` does
   NOT import `deploy`. Both reviewers agree on this; I confirmed it.
3. Behavior-neutral (all values are the byte-string `"decloud"` today). No test
   impact beyond build + `gofmt` green. Zero risk. Do it while the file is open.

**Scope boundary I am drawing (for the record, NOT this task):** `service.go:387`
hardcodes `"decloud"` instead of reading `prev.Config.Run.Network` like it reads
the other `prev.Config.*` fields. That means the persisted network field (set at
324) is currently IGNORED on redeploy — a latent inconsistency, behavior-neutral
only because both are `"decloud"`. For THIS task, swapping 387's literal to
`caddy.NetworkName` is correct and sufficient (matches the canonical name).
Whether redeploy should instead honor `prev.Config.Run.Network` is a SEPARATE
question — explicitly OUT OF SCOPE here, flagged so it is not lost. Ward: note in
`_ai/`. Do NOT expand this task to chase it.

**Docs follow-up:** the `_ai` amendment's "pure consolidation onto the single
source of truth" wording is now TRUE once all five literals (159, 163, 254, 324,
387) are `caddy.NetworkName`. No softening needed — the code now matches the
claim. Raymond: no doc change required beyond confirming the claim holds; Ward
records the redeploy-387 latent-bug note above.

**Routing:** Rob makes the three-literal swap (254, 324, 387 → `caddy.NetworkName`),
runs `gofmt` and `go build ./...` + `go test ./...`. Then back through PLAN
(Don/Joel/Linus) per workflow. Linus: re-review with the corrected ground truth
— your APPROVED rested on "from registry config," which is false; reassess.
