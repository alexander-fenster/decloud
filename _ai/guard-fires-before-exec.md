# Guard-fires-before-exec: lock with `assert.Empty(records)`, not just `require.Error`

When a function rejects bad input BEFORE spawning an external process (or doing any irreversible side effect), the rejection-test must assert that the side effect did NOT happen — not just that an error came back. The recording-fake-exec pattern in `internal/dockerdrv` makes this a one-line assertion: `assert.Empty(t, records)`.

## The bug class

A naive rejection test asserts `require.Error(t, err)` and stops there. A future refactor that puts the guard AFTER `cmd.Run` — for example, "check the exit code from docker, then validate" — still returns an error and still passes the naive test. But now a `docker run` actually fired with the bad input. On the journald path that means a container WAS created with an ambiguous tag (or with no log driver), then torn down on the guard's complaint. Side effect leaked, test went green.

## Recipe

The dockerdrv tests build a recording `cmdFactory`:

```go
records := [][]string{}
factory := func(ctx context.Context, name string, args ...string) *exec.Cmd {
    records = append(records, append([]string{name}, args...))
    return exec.Command("true")
}
```

The rejection test then asserts:

```go
_, err := driver.Run(ctx, dockerdrv.RunRequest{ /* Service empty */ })
require.Error(t, err)
assert.True(t, errors.Is(err, dockerdrv.ErrEmptyService))
assert.False(t, errors.Is(err, dockerdrv.ErrInvalidService))
assert.Empty(t, records, "no docker process must be spawned when Service is empty (guard fires before cmd.Run)")
```

The `assert.Empty(records)` is the load-bearing line. Failure mode message names the invariant: future refactors that move the guard see the failure and the invariant in the same breath.

## When to apply

Any guard that:

1. Rejects bad input with a typed sentinel.
2. Sits in front of an external process, a file write, a network call, or any other irreversible side effect.
3. Has a fake-exec / mock / recorder surface that can prove the side effect did not run.

The shape generalises beyond `dockerdrv` — same pattern works for any caller-of-exec where the test wires a recording factory.

## Why the No-Docker-on-this-Mac constraint reinforces this

The maintainer dev box has no Docker (see `MEMORY.md`). Every unit test runs against a fake `cmdFactory` rather than a real `docker run`. That forces the test design into argv-recording shape, and the same recorder trivially supports the `assert.Empty(records)` negative assertion. The constraint that looks like a limitation (no real Docker) actively enables the better test discipline.

## Originator

`_tasks/2026-05-12-journald-log-driver/03-tech-plan.md` §6.2.1–§6.2.4 specified the assertion triad; Kent shipped it in `05-kent-tests.md`; Kevlin's `08-kevlin-review.md` §4 calls it out as "the assertion that distinguishes a real test from mock-theatre."
