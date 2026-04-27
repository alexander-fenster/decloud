# 011 — Linus impl review (post-execution)

Author: Linus Torvalds (high-level review agent)
Date: 2026-04-26
Reviews: implementation on disk for the containerised-Caddy plan against `005-don-plan-v2.md`, `006-joel-tech-plan-v2.md`, and my own `007-linus-review-v2.md` (APPROVED-with-nits).
Scope: ONLY changes within this task. Pre-existing code untouched is not my concern here.

---

## TL;DR

The team shipped exactly what v2 promised. The bug is fixed at the level the
plan says it's fixed at — Caddy now runs as `decloud-caddy` on the `decloud`
bridge, the reloader `docker exec`s into it, and the embedded-DNS path is the
only resolution path the Caddyfile can possibly take. Dual-stack publishing
went in at the type level (`PortMap.HostBind`) and the formatter does not
auto-bracket. The seven required revisions from `004-linus-review.md` are all
present in code, not just in plan. Every one of my non-blocking nits from
`007-linus-review-v2.md` was either inherited correctly or honestly held
under the bar I set.

**Verdict: APPROVED.** Task can advance to the PLAN-loop step.

The remainder of this document walks the eight audit items the request asked
me to check, calls out two legitimate observations Don should know about
before deciding whether to act in this task or defer, and leaves nothing else
on the table.

---

## 1. The fix actually fixes the bug — code-path walk

The user's symptom was `decloud-durak-live` resolving to `[2a03:f480:1:12::7b]`
(the host's GUA) and the dial refusing because nothing on the host was bound
to port 10001. The new architecture makes that resolution path
*architecturally impossible*. Walked end-to-end:

**On `decloud caddy up`:**

- `internal/cli/caddy_up.go:16` calls `caddyManagerFactory(...)`, which is
  `buildProductionCaddyManager` at `internal/cli/deploy_service.go:155`. That
  hands `cliManager` a real `dockerdrv.Driver` and `config.Paths`.
- `cliManager.Up` (`internal/caddy/manager.go:64`) does, in order:
  `NetworkEnsure("decloud")` → `WriteStubIfMissing` → `Inspect("decloud-caddy")`
  → on `absent` → `ImagePull("caddy:2")` → `RunWithOptions(...)` from
  `runOpts()` at line 120. The `RunOptions` literal hardcodes
  `Network: "decloud"` (via `NetworkName`), so `decloud-caddy` is on the
  bridge. Verified.
- `RunWithOptions` in `internal/dockerdrv/cli_driver.go:203` emits
  `docker run -d --name decloud-caddy --network decloud --restart
  unless-stopped --label decloud.managed=caddy -p 0.0.0.0:80:80/tcp -p
  [::]:80:80/tcp -p 0.0.0.0:443:443/tcp -p [::]:443:443/tcp -p
  0.0.0.0:443:443/udp -p [::]:443:443/udp -v
  /opt/decloud/config/caddy:/etc/caddy:ro -v decloud_caddy_data:/data -v
  decloud_caddy_config:/config caddy:2`. Locked by
  `TestCLIDriver_RunWithOptionsCaddyShape` (cli_driver_test.go:325-360).

**On `decloud deploy service`:**

- `serviceDeployer.Deploy` (`internal/deploy/service.go:127`) calls
  `Driver.NetworkEnsure(ctx, "decloud")` first. Because Caddy is already a
  member, the new service container joins the same bridge. `Driver.Run` at
  line 195 runs `decloud-<svc>` with `--network decloud`.
- After registry save, `regenerateAndReload` at line 302 generates the
  Caddyfile with `reverse_proxy decloud-<svc>:PORT`, then
  `Reloader.Validate` (`internal/caddy/reloader.go:46`) runs
  `docker exec decloud-caddy caddy validate --config
  /etc/caddy/Caddyfile.tmp` via `Driver.Exec`. Exec runs *inside* the Caddy
  container, so the validator sees the same Caddyfile through the bind
  mount and resolves names through the container's DNS — i.e. embedded
  Docker DNS at `127.0.0.11`.
- Atomic rename (host-side) makes `Caddyfile` visible inside the container
  via the directory bind mount. `Reloader.Reload` runs `caddy reload
  --config /etc/caddy/Caddyfile`, which signals the live Caddy process to
  re-resolve upstreams. Caddy now resolves `decloud-<svc>` → embedded DNS
  → `172.18.0.x`. Connection succeeds. Bug fixed.

The IPv6-fallthrough resolution path that triggered the original bug is no
longer reachable. Caddy's resolver sits inside the container; `127.0.0.11`
serves only A records on an IPv4-only bridge; `2a03:f480::*` is no longer a
possible answer. Confirmed by `internal/dockerdrv/cli_driver.go:170`'s
`ContainerIP` template on `.NetworkSettings.Networks.decloud.IPAddress` — the
network is IPv4-only.

This is the right fix at the right layer.

## 2. Architecture matches the approved plan

Three-piece decomposition is clean:

- **Driver primitives** (`internal/dockerdrv/`) — three new methods
  (`ImagePull`, `RunWithOptions`, `Exec`), four new types (`PortMap`,
  `VolumeMount`, `RunOptions`, `ExecOptions`). Existing methods byte-identical;
  `RunRequest` is untouched. Ports/volumes/labels did NOT smear into
  `RunRequest`. Acceptance criterion #20 (Joel §12) honoured.
- **`caddy.Manager`** (`internal/caddy/manager.go`) — singleton-fixture
  semantics; constants `ContainerName`/`NetworkName`/`DefaultImage` live here;
  `Up`/`Down`/`IsRunning` are the only surface; no `Image` field on
  `ManagerConfig`; `runOpts()` is a pure function of `cfg.Paths.CaddyDir`.
- **Reloader** (`internal/caddy/reloader.go`) — only fields are `driver` and
  `hostCaddyDir`. Constructor signature exactly matches Joel §4.5. The
  `cmdFactory` reloader test seam is gone (acceptance criterion #21
  honoured).

No coupling leaks between the three. `Manager` does not know about the
`Reloader`; `Reloader` does not know about the `Manager`; both depend only
on `Driver`. `regenerateAndReload` in `service.go` is unchanged in shape
— only the wrap text grew. Right boundaries.

## 3. Dual-stack publishing actually works

This was the audit item I was most paranoid about. Worth nothing was hacked
in. Verified:

- `PortMap.HostBind` is a first-class struct field
  (`internal/dockerdrv/driver.go:56`).
- `formatPortMap` (`internal/dockerdrv/cli_driver.go:268`) splices `HostBind`
  literally — no auto-bracketing. The doc-comment explicitly forbids it.
- `cliManager.runOpts()` emits all six `PortMap` entries
  (`internal/caddy/manager.go:127-134`).
- IPv6 path is exercised by `TestCLIDriver_RunWithOptionsDualStackPorts`
  (cli_driver_test.go:362-377) and `TestFormatPortMap_DoesNotAutoBracketIPv6`
  (cli_driver_test.go:463-470). Manager-side coverage:
  `TestManager_UpFreshInstall` (manager_test.go:55-82) asserts the exact
  six-entry slice.
- Argv emission verified by `TestCLIDriver_RunWithOptionsCaddyShape`
  (cli_driver_test.go:325-360); the argv reads exactly like the operator
  would type it: `0.0.0.0:80:80/tcp` then `[::]:80:80/tcp` etc., declared
  order, six `-p` flags total.

Joel §9.9 ("DO NOT auto-bracket") is enforced by both the impl comment and
the test. Don's acceptance criterion #11 (manual `ss -tlnp` showing both
listeners on the actual host) remains the operator-side gate per Don v2 §7
step 5.

## 4. Deploy-failure recovery text points at `decloud caddy up`

`internal/deploy/service.go:316` and `:322` both contain:

```
service is registered and running but Caddy is not routing traffic;
run 'decloud caddy up' (and then 'decloud caddy reload' if needed) to
restore routing
```

Wrap chain still uses `%w: %w`; `errors.Is(err, ErrCaddyReload)` and
`errors.Is(err, innerErr)` continue to hold (locked by
`TestDeploy_CaddyValidateFailureMentionsCaddyUpRecovery` at
service_test.go:365 and `TestDeploy_CaddyReloadFailureMentionsCaddyUpRecovery`
at service_test.go:388). Reloader's own actionable error
(`internal/caddy/reloader.go:69`) — `container "decloud-caddy" is not
running; run 'decloud caddy up' first` — composes through. Operator gets
both pieces: registry-state context from the deployer, container-state
context from the reloader. No new sentinel.

## 5. No Viper / TOML config snuck in

`grep -rn 'viper\|toml' internal/caddy/ internal/cli/caddy_up.go
internal/cli/caddy_down.go` returns no hits. `caddy.DefaultImage = "caddy:2"`
in `internal/caddy/manager.go:20` is the only image source. `ManagerConfig`
has no `Image` field. `caddy_up.go:14` and `caddy_down.go:14` declare
`cobra.NoArgs` and zero flags. `TestCaddyUp_NoFlags` /
`TestCaddyDown_NoFlags` lock the contract. The hard cut held.

## 6. Migration story is operationally sound

`_docs/install.md` §3.2 (lines 57-94) leads with the volume-copy recipe
(line 71-78), names the cold-restart as the alternative gated on "one or
two hostnames" (line 82-86), explicitly enumerates LE rate limits with the
**7-day recovery window** (line 92), and the persistent-disable step uses
`systemctl mask` AND `apt-get remove` (line 64-66). Per-hostname maths
spelled out (line 90: "30 services on subdomains of a single registered
domain consume 30 of those 50"). The "find /var -name 'certificates'" hint
for non-default storage paths (line 80) is a nice operator touch I didn't
ask for and didn't need to ask for.

The `disable --now` + `mask` / `apt-get remove` justification is at line
183-184, repeating the warning that `disable --now` alone is undone by
package upgrades. Good — that's the operationally correct framing.

## 7. Decision record is honest

`_ai/decisions/caddy-runs-in-container.md`:

- Don's Candidates A and B are present (lines 30-32).
- All seven of my additional rejected alternatives from §1.1 of
  `004-linus-review.md` are present with one-sentence reasoning each
  (lines 33-41).
- The volume strategy (`decloud_caddy_data` + `decloud_caddy_config`,
  never auto-removed by `caddy down`) is captured.
- The dual-stack rationale and the IPv6-disabled-host failure mode are
  named.
- The M4 admin-API forward-look is captured.
- The "why not in `_docs/`" section (line 62) makes the right operator-vs-
  contributor distinction.

Tradeoffs honestly listed. Nothing soft-pedalled.

## 8. Were my non-blocking nits handled?

From `007-linus-review-v2.md`:

| Nit | Status |
|---|---|
| `isNotRunningStderr` substring fragility | Honestly held; locked by `TestReloader_ContainerExitedSurfacesActionableError` (reloader_test.go:113-121). No regression. |
| `ports already bound` substring detection | Honestly held; same shape. |
| Wrap-text duplication in service.go:314-322 | Held; rule-of-three threshold not met. Two sites, fine. |
| Stdout shape inconsistency cold vs warm | Held; usage doc mentions the prefix. Acceptable. |
| `docker exec` no `-i`/`-t` | Held; non-interactive. Correct. |
| Empty-`HostBind` is a contract-clean fallback | Held; locked by `TestCLIDriver_RunWithOptionsEmptyHostBind`. |
| `usage.md` quick-start placement | Raymond placed it at §1 (line 13). |

All seven inherited or held under the explicit bar I set in v2 review.
Nothing snuck through.

## 9. Anything to kill, simplify, or call bullshit on?

Two observations. Neither blocks. Don decides if either gets fixed in
this task or deferred.

### Issue 9.1 — `caddy up` / `caddy down` ship `Short`-only help; Joel §1.6 specified `Long`

**Problem**: Joel's tech plan §1.6 spec'd a four-paragraph `Long`
description for `decloud caddy up --help` (containing the dual-stack
publishing detail, image and volume names) and a two-line `Long` for
`decloud caddy down --help` (volume-retention warning). Rob shipped
`Short:` only — `internal/cli/caddy_up.go:13` and `caddy_down.go:13`
have a one-line summary and no `Long`. Raymond flagged this honestly in
his report §5 nit #5.

**Impact**: Operators running `decloud caddy up --help` see only a
one-liner where the plan promised the dual-stack note and the volume
names. The volume-retention warning on `down` is actually important
operationally — losing ACME state by mistake is the kind of nasty
surprise that wakes someone up at 3 AM. The semantics are correct and
the docs cover it; the help text is a nudge for the operator who only
reads `--help` and doesn't go to the install doc.

**Options**:

- **Option A (Minimal)**: Add `Long` strings now, paste from Joel §1.6
  verbatim. ~10 lines in two files. No tests need to change because
  Cobra renders `Long` independently of `Short`. Pros: matches plan,
  catches the "I'll just `--help`" operator. Cons: trivial doc churn.
- **Option B (Defer)**: Open an M1.x backlog item ("`caddy up`/`caddy
  down` ship `Short`-only help; spec is in Joel v2 §1.6") and ship as-is.
  Pros: ships now, no scope expansion. Cons: technical debt; the
  volume-retention warning is the kind of thing that bites operators if
  they don't see it.

**My recommendation**: Option A. It is genuinely a 10-line edit and the
volume-retention warning has real operator value. The plan was specific
about it; not implementing it is a tiny "almost-done" that I'd rather
not see leak past the review boundary. But it's not a correctness bug,
so if Don wants to ship and backlog, I won't object loudly.

**DON**: Decide whether to ship the `Long` text now or backlog it.

### Issue 9.2 — `cliReloader.execCaddy` always supplies internal `Stderr`, but never propagates caller stderr to the operator

**Problem**: In `internal/caddy/reloader.go:59`, `execCaddy` allocates a
local `bytes.Buffer{}` and passes it as `ExecOptions.Stderr`. It does
NOT forward stderr to the operator's terminal. So when `caddy validate`
fails with "bad caddyfile syntax: line 12: ...", the operator sees
nothing on stderr; the message is captured into the buffer and embedded
into the wrapped error string via `fmt.Errorf("caddy %s: %w; stderr=%q",
...)` at line 72.

That's actually the v1 behaviour and I didn't ask for it to change, but
having walked through the reload pipeline against the actual error
text, I want to flag: a typical Caddyfile validation error is multi-line
and quoting it through `%q` mangles it (newlines become `\n`, quotes
become `\"`). The operator gets a single jumbled line in the error
output instead of a readable validation report.

**Impact**: Diagnosability of `caddy validate` failures is worse than
it could be. Today's bug — "Caddyfile is malformed because the
generator regressed" — is exactly when the operator wants to *read*
the full validator output. Quoting it through `%q` makes that harder.

This is not a regression — pre-task code shelled to host `caddy validate`
and presumably had the same property since `cliReloader` allocated its
own buffer. But the rewrite was a chance to fix it and didn't.

**Options**:

- **Option A (Minimal)**: Leave as-is. Operator reads the inner error
  string and mentally unescapes. Acceptable.
- **Option B (Better)**: In `execCaddy`, pass `os.Stderr` (or a writer
  the manager threads through) as `ExecOptions.Stderr`, in addition to
  the local buffer. The driver's `io.MultiWriter` path
  (`cli_driver.go:251-255`) already supports both targets. The error
  message keeps the `stderr=%q` for log/test friendliness; the
  operator-facing terminal also gets a clean copy. ~5 line change in
  the reloader; no test change because the existing tests pass a
  scoped writer or the local buffer either way.
- **Option C (Defer)**: Backlog as "improve diagnosability of caddy
  validate failures."

**My recommendation**: Option C — defer. The current behaviour is no
worse than what shipped in M1.0; the bug we set out to fix is already
fixed; my job here is "did the team do the *right* thing within the
plan's scope," not "is every nearby thing now optimal." This is a
diagnosability nit, not a correctness issue, and the plan didn't ask
for it. Ward should capture it as a backlog item; M2 can address it
alongside whatever logging-experience pass that milestone wants to do.

**DON**: Decide whether to backlog or fix in this task.

---

## What I'm NOT flagging

Clean shapes that were on my pre-execution worry-list and turned out
fine on inspection:

- `translatePath` runs its result through `filepath.ToSlash`
  (`reloader.go:83`). Rob flagged this in his report; it's a Linux/macOS
  no-op and a Windows correctness fix. Tests pass either way. Cheap
  insurance, no objection.
- `Driver.Exec` uses `io.MultiWriter` to both fan stderr to the caller
  and capture for `isNotFound` detection. Same shape as `Logs`. The
  reloader doesn't currently exercise the fan-out branch but the contract
  is right.
- The two-stage Caddyfile rename works under the directory bind mount
  because the bind is the directory, not the file (Joel §9.2). Verified
  in `runOpts()`: `Source: m.cfg.Paths.CaddyDir, Target: "/etc/caddy"` —
  directory, not the Caddyfile path.
- `cliManager.Down` uses `errors.Is(err, ErrContainerNotFound)`
  (`manager.go:102, :105`), not direct equality, future-proofing against
  a wrap. Right.
- `caddy.NetworkName` is referenced in `manager.go` and `reloader.go`'s
  caller layer; I checked that no new `"decloud"` literal was introduced
  by this task. The four pre-existing literals
  (`internal/deploy/service.go:131,190,289,238` and
  `internal/dockerdrv/cli_driver.go:170`) remain — Joel §5 explicitly
  scoped that cleanup as M1.x backlog. Existing-code, not in scope. Fine.

## Why APPROVED rather than CHANGES REQUIRED

The two issues above are scope/diagnosability nits, not correctness
defects. Issue 9.1 is a 10-line doc gap inside Cobra `Long` strings
where the plan specified text that didn't get pasted; Issue 9.2 is a
diagnosability question that was equally true before this task started.
Neither would cause the deploy to fail. Neither contradicts the user's
original symptom or the seven required revisions.

The plan v2 was right and the implementation matches the plan. Bug
fixed, architecture clean, dual-stack at the right layer, no Viper, no
rollback theatre, decision record honest, migration recipe operationally
sound. Kent and Rob and Raymond did the work straight; nobody hacked
around anything. That's a green review.

---

## Verdict

**APPROVED** — task can advance to PLAN-loop step.

Don should decide whether Issue 9.1 (Cobra `Long` help text) gets fixed
in this task or backlogged, and whether Issue 9.2 (`execCaddy` stderr
diagnosability) gets backlogged for M2. Neither blocks shipping.

— Linus
