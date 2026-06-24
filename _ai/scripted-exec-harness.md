# `scriptedFactory`: drive code branches by scripting the fake docker, no daemon

`internal/dockerdrv` shells out to the real `docker` CLI through a `cmdFactory` seam. The dev box has no Docker (see `MEMORY.md`), so every test wires a fake factory. Two variants exist, and they answer different questions:

- **`recordingFactory(&records)`** — returns a no-op success (`exec.Command("true")`) and records argv. Answers "what argv did we build?" and "did we spawn at all?" (the `assert.Empty(records)` negative; see `guard-fires-before-exec.md`).
- **`scriptedFactory(&records, script)`** — records argv AND runs `/bin/sh -c <script>` so the fake's **exit code and stdout are scriptable per invocation**. Answers "how does the driver behave across the branch the real docker would take?"

Both live in `internal/dockerdrv/cli_driver_test.go`; `driverWith(f)` wraps either into a `Driver`.

## Why scripting the exit code matters

`NetworkEnsure` is `inspect → early-return-if-present, else create`. To test the create path you must make the *first* docker call (inspect) fail and the *second* (create) succeed, in one run. A static `true`/`false` factory can't do that — the script can:

```go
d := driverWith(scriptedFactory(&records, `if [ "$2" = inspect ]; then exit 1; else exit 0; fi`))
```

`$2` is the docker subcommand verb (`network inspect …` → `$2 == inspect`). Now `inspect` returns exit 1 (network absent) and `create` returns 0, exercising the real branch with a real `*exec.Cmd` round-trip — no daemon, no network created.

## The positive-argv assertion pairs with the scripted branch

Once the create branch is forced, assert the argv the driver actually built on that branch — both POSITIVE (the flag is there) and a guarded value read:

```go
assert.Contains(t, createCall.Args, "--ipv6")
subnetIdx := indexOf(createCall.Args, "--subnet")
require.GreaterOrEqual(t, subnetIdx, 0)
assert.Equal(t, decloudIPv6Subnet, createCall.Args[subnetIdx+1])
```

Reading the value by index (`--subnet` then `Args[idx+1]`) rather than asserting a flat slice equals a fixed list keeps the test robust to flag-order churn while still pinning the load-bearing value. Pair this with the `NotContains` negatives where a flag must be absent (`assert.NotEqual "--driver"` here; the create-time `--log-driver`/`--log-opt` rejection in `sealed-at-create-lock-with-notcontains.md`).

## When to reach for scripted vs recording

- Branch depends only on argv you build → `recordingFactory` is enough.
- Branch depends on the **exit code / output of a prior docker call** (inspect-then-act, error-classification, idempotent early-return) → `scriptedFactory`. The shell script is the cheapest way to make call N's result steer call N+1 without a daemon.

## Originator

`_tasks/2026-06-24-docker-network-ipv6/005-kent-test-report.md` (Kent) — the `NetworkEnsureWhenAbsent` test that locks the `--ipv6 --subnet` create-path argv; Kevlin's `008-kevlin-review.md` confirmed the index-read pattern over flat-slice equality. The factory seam itself predates this task (journald work, `_tasks/2026-05-12-journald-log-driver/`).
