# Tech Plan: Docker network IPv6 support (Joel)

Expands Don's revised `002-plan.md` (scope narrowed by user 2026-06-24: clean
installs only, **no reconcile**). This is now a minimal change — one production
line in `NetworkEnsure` plus a cleanup of two magic strings. The reconcile /
rm+recreate path from the earlier draft is **gone**.

---

## 0. Source facts established by inspection

- `NetworkEnsure` today: `internal/dockerdrv/cli_driver.go:176-184`.
  ```go
  func (d *cliDriver) NetworkEnsure(ctx context.Context, name string) error {
      if err := d.cmd(ctx, "docker", "network", "inspect", name).Run(); err == nil {
          return nil
      }
      if err := d.cmd(ctx, "docker", "network", "create", name).Run(); err != nil {
          return fmt.Errorf("docker network create: %w", err)
      }
      return nil
  }
  ```
  Lines 176-179 (inspect + early return) stay **byte-for-byte**. The ONLY change
  is the create argv on line 180.
- The inspect call throws stdout away and only checks the error. **We do NOT
  touch it** — no `--format '{{.EnableIPv6}}'`, no stdout capture, no
  EnableIPv6 branching. (That machinery belonged to the dropped reconcile path.)
- Error-wrap house style: `fmt.Errorf("docker <verb>: %w; stderr=%q", ...)`
  (Run l.80, Stop l.94, Inspect l.135, pull l.215). The existing create wrap
  (l.181) is the one shell-out in the file that omits `stderr=%q`; see §7.
- `ContainerIP` (l.187) reads `.NetworkSettings.Networks.decloud.IPAddress` —
  the IPv4 field. Docker keeps it and adds `GlobalIPv6Address` separately, so
  `--ipv6` does not change it. Readiness probe unaffected (grep confirms nothing
  in-tree reads any IPv6 field).
- `Driver.NetworkEnsure(ctx, name string) error` signature UNCHANGED ⇒ gomock in
  `internal/dockerdrv/mocks/` needs NO regen.

---

## 1. The ULA subnet constant — placement confirmed

Unexported package-level const in `dockerdrv` (`cli_driver.go`), as Don
specifies and as import direction requires (`caddy`/`deploy` import `dockerdrv`,
never the reverse — a const in `caddy` would be unreachable from the driver).

```go
// decloudIPv6Subnet is the fixed ULA (/64, RFC 4193) the decloud bridge is
// created with. NAT66/masquerade hides it behind the host's global address, so
// the exact prefix is an internal driver detail, not an operator knob.
const decloudIPv6Subnet = "fd00:dec0:11d::/64"
```

Place it immediately above `NetworkEnsure` (~line 175), consistent with the
per-feature locality of the `formatArg` consts elsewhere in the file.

---

## 2. The one production change in `NetworkEnsure`

Replace lines 176-184 with the following. Lines 176-179 are identical to today;
only the create call and its wrap change.

```go
// NetworkEnsure creates the decloud bridge with IPv6 (ULA + NAT66 egress) on a
// fresh install. An already-existing network is left untouched — there is no
// docker command to toggle EnableIPv6, and recreating in the deploy path would
// be destructive, so upgrading an old IPv4-only network is done out-of-band.
func (d *cliDriver) NetworkEnsure(ctx context.Context, name string) error {
	if err := d.cmd(ctx, "docker", "network", "inspect", name).Run(); err == nil {
		return nil
	}
	var stderr bytes.Buffer
	cmd := d.cmd(ctx, "docker", "network", "create",
		"--ipv6", "--subnet", decloudIPv6Subnet, name)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker network create: %w; stderr=%q", err, stderr.String())
	}
	return nil
}
```

### Arg construction — concrete decision

The create args are a **fixed positional slice**, passed as variadic literals to
`d.cmd`, exactly mirroring how `Inspect`/`ContainerIP` append `--format` as
trailing literals. No `[]string` builder, no loop — there is nothing dynamic
except `name` (already the trailing positional today). Order:

```
docker network create --ipv6 --subnet fd00:dec0:11d::/64 <name>
```

- `--ipv6` is a boolean flag, no value.
- `--subnet` takes its value as the next token (`decloudIPv6Subnet`).
- `<name>` stays last, matching the current bare-create positional contract.
- **No `--driver`** — default bridge required by the readiness probe and the
  `WhenAbsent` test. Confirmed absent.
- **No IPv4 `--subnet`** — Docker auto-allocates IPv4 from its default pool as
  today, keeping `ContainerIP`'s IPv4 read valid and avoiding pool collisions
  (Don §1).

### Why I keep the rest byte-for-byte

The inspect early-return is the no-op-on-existing-network behavior the user
explicitly wants preserved. Touching it (e.g. to read EnableIPv6) would be
exactly the reconcile scope the user cut. So `NetworkEnsure` gains only the
create flags + the stderr capture (§7 refinement).

---

## 3. Test impact — exactly what Kent does

Two existing tests bracket this code. Re-read of `cli_driver_test.go:200-227`:

- `TestCLIDriver_NetworkEnsureWhenAbsent` (lines 202-215) drives the create path:
  scripted shell `if [ "$2" = inspect ]; then exit 1; else exit 0; fi` (inspect
  absent ⇒ create runs). It already asserts a `create` call happens and `--driver`
  is absent. **Kent EXTENDS this test.**
- `TestCLIDriver_NetworkEnsureWhenPresent` (lines 218-227): scripted `exit 0`
  (inspect succeeds), asserts no `create`. **Kent LEAVES THIS UNTOUCHED** — it
  pins the no-op-on-existing behavior we are preserving, and our change does not
  touch the present path.

The reconcile tests (T3/T4/T5 in the prior draft) are **dropped** — there is no
reconcile, no `rm`, no recreate to test.

### How to match the new args in the recording

The `scriptedFactory` records each invocation as `recordedCmd{Name, Args}` where
`Args` excludes the binary (`Name == "docker"`). So for the create call,
`Args == ["network", "create", "--ipv6", "--subnet", "fd00:dec0:11d::/64", "decloud"]`.
The existing test already grabs the last recorded call:

```go
createCall := records[len(records)-1]
```

Kent adds these assertions to that block (the create call is `records[len-1]`
because inspect is recorded first, then create):

```go
assert.Contains(t, createCall.Args, "--ipv6",
    "fresh network must be created with --ipv6 for container IPv6 egress")

// --subnet must carry the ULA const value as its immediately-following token,
// not merely appear somewhere in argv.
subnetIdx := indexOf(createCall.Args, "--subnet")
require.GreaterOrEqual(t, subnetIdx, 0, "--subnet must be present on create")
require.Less(t, subnetIdx+1, len(createCall.Args), "--subnet must have a value")
assert.Equal(t, decloudIPv6Subnet, createCall.Args[subnetIdx+1],
    "--subnet value must be the decloud ULA const")
```

Notes for Kent:

- Use the package-level `decloudIPv6Subnet` const in the assertion (white-box
  same-package test — it is reachable), NOT a re-typed `"fd00:dec0:11d::/64"`
  literal, so impl and test share one source of truth and a subnet typo fails in
  exactly one place.
- `indexOf` already exists (`cli_driver_test.go:692`). No new helper needed —
  and crucially, **no `networkSubcmd` helper** (that was for the dropped
  ordering assertions).
- Keep the existing `--driver`-absent loop (lines 211-214) and the
  `Contains(create)` assertion as-is.
- The pairing comment above the test (lines 200-201) must be updated to the new
  argv per the file's house convention (lines 1-4):
  ```
  // docker network inspect decloud  -> exit 1 (absent)
  // docker network create --ipv6 --subnet fd00:dec0:11d::/64 decloud
  ```

**Why this is a real behavioral assertion, not change-detection:** it pins the
externally-meaningful contract that a freshly created network actually requests
IPv6 with the ULA subnet — the entire point of the task (container IPv6 egress).
It survives any refactor that keeps that contract and fails only on a real
regression (flag dropped, wrong subnet, IPv4-only create).

---

## 4. `deploy/service.go` literal cleanup — IN SCOPE, cycle-free

Confirmed (Don §2, verified): `internal/deploy/service.go:12` already imports
`caddy`; `caddy` does not import `deploy`. No new dependency, no cycle. Rob
replaces the two `"decloud"` literals with `caddy.NetworkName`:

- `service.go:159`: `NetworkEnsure(ctx, "decloud")` → `NetworkEnsure(ctx, caddy.NetworkName)`.
- `service.go:163`: log field `"network", "decloud"` → `"network", caddy.NetworkName`.

Value-identical (`"decloud"`), call graph identical — pure consolidation onto the
single source of truth. No deploy test should change behavior; a literal-matching
deploy test still matches.

---

## 5. Files touched (handbook)

| File | Change |
|---|---|
| `internal/dockerdrv/cli_driver.go` | Add `decloudIPv6Subnet` const (~l.175); change the create call on l.180 to add `--ipv6 --subnet <const>` + capture stderr in the wrap (§2, §7). Lines 176-179 unchanged. |
| `internal/dockerdrv/cli_driver_test.go` | EXTEND `NetworkEnsureWhenAbsent` (§3); update its pairing comment. LEAVE `NetworkEnsureWhenPresent` untouched. |
| `internal/deploy/service.go` | Lines 159 & 163: `"decloud"` → `caddy.NetworkName` (§4). |
| `_docs/`, `_ai/` | Raymond per Don's plan: fresh-install IPv6 (ULA + NAT66), existing networks left untouched / upgraded out-of-band, "no EnableIPv6 toggle" gotcha. |

No interface change ⇒ **no mock regeneration** anywhere.

---

## 6. What is explicitly OUT of scope now

- No `EnableIPv6` inspection, no `--format` on the inspect call.
- No `docker network rm`, no recreate, no "active endpoints" handling.
- No T3/T4/T5 reconcile tests; no `networkSubcmd` helper; no three-method
  dispatcher refactor. `NetworkEnsure` stays a single small function.
- No broader `NetworkName` audit beyond the two literals on the touched path.

---

## 7. Where I refine Don (called out per instructions)

One refinement, in-spirit, not a scope change:

**Add `stderr=%q` to the create error wrap.** Today's line 181 wraps as
`"docker network create: %w"` with no stderr. Every other shell-out in the file
captures stderr; an invalid-`--subnet` or daemon error on create is exactly when
the operator needs that text. Since we are already editing this exact create
statement to add the flags, completing the wrap to the file's house style
(`var stderr bytes.Buffer` + `stderr=%q`) is free and strictly better.

I do NOT add a dedicated test for this wrap — Don's two-test plan is the agreed
surface, the stderr-surfacing pattern is already exercised by
`TestCLIDriver_ImagePullPropagatesStderrOnFailure` (l.316), and adding a
create-fails test would re-introduce surface Don deliberately trimmed. If Kent
wants one cheaply it is a ~6-line scripted-failure test, but it is optional, not
required by this plan.

Everything else is a faithful, mechanical expansion of Don's narrowed plan.

---

## 8. Estimate (Joel's reality check)

One production line of flags + a stderr buffer, two literal swaps, ~4 lines of
test assertions against an existing test. Raw: ~30 min. ×π for gofmt/CI/review
round-trips ⇒ budget ~1.5h across the agent chain. The only place to get wrong
is the exact ULA literal — pinned once in the const and asserted via that same
const, so a typo cannot silently diverge.
