# Label-gated orphan recovery, not name-gated

When a fresh orchestrator run encounters a pre-existing artefact with the name it wants to claim (a container, a directory, a registry entry), recovery must be gated on a label or fingerprint that proves THIS orchestrator created it — NOT on the name alone. Name-gated recovery silently destroys hand-rolled artefacts and is a footgun.

## The bug class this prevents

A fresh `decloud deploy service --name foo` finds an existing `decloud-foo` container. Two distinct origins:

1. An orphan from a prior interrupted deploy that `decloud` itself created. Removing it is correct.
2. A container the user ran by hand (`docker run --name decloud-foo ...`), or one a different tool created with that name. Removing it is destruction of unrelated state.

A name-only check (`if container "decloud-foo" exists, stop+remove`) cannot tell these apart. Linus flagged this in `_tasks/2026-04-28-deploy-cleanup-on-interrupt/04-linus-review.md` Issue 1.

## The pattern

`decloud` attaches `--label decloud.service=<name>` to every container it creates (`internal/dockerdrv/cli_driver.go:61`). The orphan check inspects that label and branches on three outcomes:

- **Absent** — `Inspect` returns `state == "absent"`. No-op (common case).
- **Present, label matches** — log warn, stop, remove, proceed.
- **Present, label missing or mismatched** — refuse with a clear `ErrRun` and a manual recovery hint.

Site: `internal/deploy/service.go:198-227` (gated on `!hasPrev`, i.e. only when no registry entry exists; redeploy of a known service uses the separate `hasPrev` branch at `service.go:185-197` and the two branches stay disjoint).

Refusal error string (verbatim from `service.go:209`):

```text
container decloud-<name> exists but was not created by decloud
(label decloud.service="..." does not match "..."); refusing to remove.
Run 'docker rm -f decloud-<name>' manually if you want to claim this name,
or pick a different service name
```

The hint names the EXACT command the user should run if they accept the destruction. Don't bury recovery in prose; quote the literal command.

## Driver-level cost: one optional field

To inspect labels safely, `dockerdrv.InspectResult` got a new field (`internal/dockerdrv/driver.go:43`):

```go
Labels map[string]string // container labels; nil when State == "absent"
```

`cliDriver.Inspect` migrated `--format` from whitespace-separated (`{{.Id}} {{.State.Status}}`) to JSON:

```go
const formatArg = `{"id":{{json .Id}},"state":{{json .State.Status}},"labels":{{json .Config.Labels}}}`
```

JSON is mandatory for labels — `strings.Fields` can't survive a label value containing spaces, equals signs, or quotes. The same migration locks the parser against future label content that wasn't anticipated.

Adding a field to a struct is non-breaking: existing `InspectResult{State: "absent"}` literals zero-value Labels to nil and continue to compile. No new mock methods needed; `go generate ./...` re-emits the existing mock.

## Why the gate covers more than ctrl+c orphans

The defensive branch fires for ANY pre-existing `decloud-<name>` container on a no-registry-entry deploy:

- Ctrl+c orphans (the headline case the user reported).
- SIGKILL of `decloud` between `docker run` and registry save.
- Host power loss between `docker run` and registry save.
- Old buggy `decloud` binaries that leaked containers on a different code path.

All four leave a labelled container with no registry entry. The label gate handles them uniformly without adding code per-cause.

## Anti-pattern: registry-only gate

A weaker check ("if the registry has no entry for `decloud-foo`, just remove the container") was the v1 plan. Linus rejected it — the registry directory can be hand-deleted, the user can be experimenting, the registry can be on a different host than the docker daemon thinks. The label is the ONLY proof of origin you actually trust. The registry-entry check stays as the BRANCH selector (don't even look at orphans on a known redeploy) but is NOT sufficient as the destruction gate.

## When to apply

Any orchestrator that:

1. Owns a namespace of named artefacts (containers, files, DNS records, processes).
2. Cannot atomically guarantee creation + registry write — i.e. has a window where a crash leaks an artefact.
3. Wants to recover automatically rather than make the user run cleanup commands.

Pair the gate with: a label/fingerprint that's set at creation time, an orchestrator-level check on next run, and a refusal path with a literal recovery command for the foreign-artefact case.

## Locked in by

- `TestDeploy_DefensiveOrphanCleanupOnFreshDeployWhenContainerExists` — happy path: labelled orphan, stop+remove+proceed.
- `TestDeploy_DefensiveOrphanCleanupSkippedWhenContainerAbsent` — no-op when state is absent; explicit `Times(0)` on Stop/Remove.
- `TestDeploy_DefensiveOrphanCleanupFailureWrapsErrRun` — stop fails, error mentions `docker rm -f decloud-foo`.
- `TestDeploy_DefensiveOrphanRefusesContainerWithoutDecloudLabel` — no decloud.service label → ErrRun + recovery hint, no Stop/Remove called.
- `TestDeploy_DefensiveOrphanRefusesContainerWithMismatchedLabel` — wrong service value → ErrRun, error surfaces the offending value via `%q`.
- `TestDeploy_DefensiveOrphanInspectCancelledReturnsErrInterrupted` — three subtests for cancellation symmetry at Inspect/Stop/Remove sites; see `cleanup-context-discipline.md` for the cancellation re-wrap rule.
- `TestCLIDriver_InspectParsesDecloudServiceLabel` and `TestCLIDriver_InspectReturnsNilLabelsWhenContainerHasNone` (`internal/dockerdrv/cli_driver_test.go:267-288`) — driver-level JSON parse coverage including the nil-labels case.
