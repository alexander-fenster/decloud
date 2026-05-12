# State sealed at create-time: lock the absence with `assert.NotContains`

When an external system seals configuration at object-creation time and ignores it on subsequent lifecycle calls — Docker's `HostConfig.LogConfig` is sealed at `docker create`/`docker run` and persists across `docker start` — the start-path argv must NOT carry the create-time flags. The lock is a *negative* assertion: `assert.NotContains` on each forbidden token in the start-path argv test.

## Live example

`internal/dockerdrv/cli_driver.go` deliberately omits `--log-driver` and `--log-opt` from the `docker start` argv (`Start` method). The lock lives in `TestCLIDriver_StartArgs`:

```go
assert.NotContains(t, args, "--log-driver",
    "HostConfig.LogConfig is sealed at create time; docker start must not re-emit it")
assert.NotContains(t, args, "--log-opt",
    "HostConfig.LogConfig is sealed at create time; docker start must not re-emit it")
```

Two assertions, one per flag. Failure-mode message names the invariant — a future "consistency" refactor that copy-pastes log flags into `docker start` argv fails the test with the explanation of why it shouldn't.

## Why the negative test matters more than the positive

`assert.Contains(args, "--log-driver")` is what locks the `Run` path (create-time emission). `assert.NotContains` is what locks the `Start` path (post-create *omission*). Without the negative test, the start-path argv could grow log flags by silent copy-paste — every assertion in the existing `StartArgs` test would still pass, because none of them prohibits the new tokens. The omission is invisible until production behaviour diverges (e.g., a Docker version where re-emitting the flag produces a startup error instead of a silent no-op).

## When to apply

Any time the external system has a "config sealed at X, lifecycle calls ignore it" semantic:

- Docker `HostConfig.*` — sealed at `docker run`/`docker create`. `docker start` ignores most of it.
- systemd unit files — sealed at `systemctl daemon-reload`. `systemctl start` reads cached state.
- Cloud-init user-data — sealed at instance launch. `reboot` doesn't re-run it.

Any flag that belongs on the create path gets a *positive* assertion in the create-path test AND a *negative* assertion in every lifecycle-path test (start, restart, reload). The pair locks both directions: the flag IS emitted at create, the flag is NOT re-emitted elsewhere.

## Anti-pattern: silent absence

Test passes because the production code happens not to emit the flag today; nothing prevents tomorrow's copy-paste from adding it. The absence is unprotected. Add the `NotContains` row with the failure-mode message and the protection becomes structural.

## Originator

`_tasks/2026-05-12-journald-log-driver/03-tech-plan.md` §6.2.7 specified the two `NotContains` rows; Kent shipped them as part of the existing `TestCLIDriver_StartArgs` extension (`05-kent-tests.md`); Kevlin's `08-kevlin-review.md` §4 called them out as the right shape ("a future 'consistency' refactor that adds log flags to `docker start` argv would now fail two assertions named with the invariant they defend").
