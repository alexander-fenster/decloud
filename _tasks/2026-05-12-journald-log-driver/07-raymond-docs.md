# Raymond — documentation report for journald log driver

STEP 3c (EXECUTION — docs). Documentation step against Rob's green
implementation commit `b3a3620`. All work done on branch
`task/journald-log-driver`.

## Files touched

### `_docs/usage.md`

Three changes, all corresponding to the user-facing surface Don's plan
§5.1 / Joel §13 named:

1. **§2 "What the deploy actually does, in order" — step 4.** Extended
   from "Run the new container on the `decloud` network." to add the
   journald sentence: every Decloud-managed container (services AND
   `decloud-caddy`) is started with `--log-driver=journald --log-opt
   tag=decloud/<service>`; `decloud logs` keeps working unchanged
   because journald supports `docker logs` natively; cross-redeploy
   history is queryable via `journalctl CONTAINER_TAG=decloud/<service>`.
   Cross-links to §4 (logs command) and §6 (debugging).

2. **§4 "Lifecycle commands" — `decloud logs` bullet.** Annotated that
   `decloud logs <name>` shows only the **current** container instance;
   previous container generations (pre-redeploy, pre-restart) are not
   reachable through `decloud logs` and require `journalctl
   CONTAINER_TAG=decloud/<name>` instead. Cross-link to §6 for the full
   recipe.

3. **§6 "Debugging a container directly" — new subsection "Reading
   logs across redeploys".** Explains the tag scheme (`decloud/<service>`
   for services, `decloud/caddy` for Caddy), shows three `journalctl`
   invocations (all-history, `--since '1 hour ago' -f`, Caddy's own
   logs), documents that `CONTAINER_TAG=` is exact-match-only (no prefix
   form) and how to query multiple services with one command (multiple
   matches against the same field are OR'd by journalctl). Mentions
   that service names containing `/` or empty service names are
   rejected at the driver layer (so the tag is always unambiguous),
   and notes journald retention is the operator's concern.

### `_docs/install.md`

One change: added a fourth bullet to §1 "Prerequisites" stating that
the Docker daemon must run under systemd because every container
Decloud starts uses the journald log driver; the default Docker Engine
install (`systemctl enable --now docker` in §2) satisfies this. One
sentence, with a cross-link to the new `usage.md` §6 subsection.

### `_ai/decisions/journald-log-driver.md` (NEW)

New decision record. Matches the existing voice in
`_ai/decisions/caddy-runs-in-container.md` (context, decision,
alternatives, consequences, why-this-isn't-in-_docs). Records:

- The always-on policy (no flag, no env var) and why.
- Tag literal: `decloud/<service>` for services, `decloud/caddy` for
  the manager container. Hardcoded literal at
  `internal/caddy/manager.go:127`. Flag pair spliced after `--restart`
  at `internal/dockerdrv/cli_driver.go:58` (Run) and `:232`
  (RunWithOptions).
- Why journald and not syslog (three concrete reasons; `docker logs`
  continuing to work is the load-bearing one).
- Why `Service` flows explicitly via a new `RunRequest`/`RunOptions`
  field rather than being derived from `Name` by `TrimPrefix` (forward
  compat with M4 container renaming per `_ai/container-naming.md`).
- The two sentinel errors (`ErrEmptyService`, `ErrInvalidService`),
  why they're distinct, and the slash-rejection invariant (journald
  tag unambiguity under `journalctl CONTAINER_TAG=` queries).
- The `docker start` invariant (HostConfig.LogConfig is sealed at
  create time; `docker start` does NOT re-emit log flags), and the
  test that locks it.
- Acceptance evidence shape (the four `docker inspect` /
  `journalctl` invocations Don §6 named).
- Forward-looking notes: `decloud logs --history` follow-up,
  retention as operator concern, M4 alignment.

### `_ai/m1x-backlog.md`

Added item 12: `decloud logs --history` to surface the journald
cross-redeploy archive. Cites Don §6 and Joel §10.9 as originators.
Names the design surface a future fix would have to settle (flag
shape, journald-vs-docker-logs `-f` semantics, opt-in vs default).
Confirmed via grep that no prior entry for this work existed in the
file before this commit (no duplication).

## What I deliberately did NOT document

- **No `decloud logs --history`, no journalctl wrapper, no CLI flag
  changes.** Don §6 / Joel §10.9 / `06-rob-implementation.md` all flag
  this as out of scope. The user-facing docs tell operators to use
  `journalctl CONTAINER_TAG=decloud/<service>` directly. The future
  CLI wrapper is captured in `_ai/m1x-backlog.md` item 12 only.
- **No log retention / rotation tuning.** Plan §8 explicit
  out-of-scope. `usage.md` §6 names it as the operator's concern via
  `journald.conf` (`SystemMaxUse`, `MaxRetentionSec`) and stops there.
- **No migration story for pre-change service containers.** Plan §8
  notes that operators on an older Decloud build pick up journald on
  the next `decloud deploy service`, and pre-change `json-file` logs
  in the old container are lost on the next redeploy as before. Not
  worth a doc paragraph — it's the bug we're fixing for future
  redeploys, mentioning it would suggest there's a migration to do
  when there isn't.
- **No CHANGELOG or release notes.** Confirmed the repo carries no
  CHANGELOG.md or release-notes file (`ls / | grep -i -E
  "changelog|release"` returns nothing).
- **No update to `_ai/container-naming.md`.** It already names the
  rule that anything depending on container name must be touched at
  M4. The new decision record cross-references it; bidirectional
  linking would be one back-link too many for a file that's already
  short.

## Accuracy notes for Kevlin

I caught and fixed one factual error during this step before commit:
my initial draft claimed `journalctl CONTAINER_TAG=~^decloud/`
matches any tag with that prefix. Verified against the live
`journalctl` man page on a Linux server: `journalctl FIELD=`
is exact-match only — there is no `=~` regex form for fields;
`--grep` is PCRE but against MESSAGE content, not field names.
Multiple matches against the same field are OR'd. Fixed in both
`_docs/usage.md` §6 (showed the OR'd multi-tag form) and
`_ai/decisions/journald-log-driver.md` (corrected the same claim).

Other claims verified against source:

- Flag literals at `internal/dockerdrv/cli_driver.go:58` and `:232`
  (both functions, post-`--restart`, pre-env).
- Sentinel error definitions at `internal/dockerdrv/driver.go:22-34`.
- Service field at `internal/dockerdrv/driver.go:47` (`RunRequest`)
  and `:94` (`RunOptions`).
- Caddy `Service: "caddy"` at `internal/caddy/manager.go:127`.
- Caller populations at `internal/deploy/service.go:246` (fresh
  deploy), `:379` (rollback), `internal/deploy/lifecycle.go:69`
  (absent-branch re-run).
- `TrimPrefix` smell removal: grep -rn `"TrimPrefix.*decloud-"
  internal/ cmd/` returns no matches in production code.

## Verification

- `gofmt -l .` — clean.
- `go build ./...` — clean.
- Did not re-run `go test ./...` — docs-only changes outside
  `internal/` and `cmd/`; Rob's `06-rob-implementation.md` already
  reported 246/0 green at commit `b3a3620`.

## Hand-off to Kevlin and Linus

All documentation reflects the merged implementation. The new
decision record is the load-bearing artifact — future contributors
need it to understand the field-flow rule and the two sentinel
errors. Kevlin: please verify the field-name and file-line claims in
this report against the actual code; that's the highest-value
check given the doc-step's hallucination risk.
