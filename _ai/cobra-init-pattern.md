# Cobra `PersistentPreRunE` for filesystem-touching init

Any `Init()` that touches the filesystem (creates dirs, opens files, stats paths) MUST run from `PersistentPreRunE`, NOT from `main()` or `cobra.OnInitialize`. Otherwise `decloud --help` exits 70 on a fresh box because Cobra's help short-circuit fires AFTER global init but BEFORE `PersistentPreRunE`.

## Recipe

`cmd/decloud/main.go` is signal-plumbing only — five lines of body. Filesystem init lives in `internal/logging/logging.go:Init()` and is called by `internal/cli/root.go:22-24`:

```go
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
    return logging.Init()
}
```

Cobra short-circuits `--help`, `help <cmd>`, and `<cmd> --help` at all depths BEFORE PersistentPreRunE fires (verified by `TestRoot_HelpDoesNotRequireFilesystem` at `internal/cli/root_test.go`). Operators on a brand-new box can run `decloud --help` against a missing `DECLOUD_ROOT` and get exit 0.

## Mandatory: graceful EACCES/ENOENT fallback

Even from `PersistentPreRunE`, the operator's first real subcommand on an unbootstrapped box would fail without a fallback. Pattern at `internal/logging/logging.go:21-43`:

1. Env-var short-circuit FIRST (`DECLOUD_LOG_TO_STDERR_ONLY=1`) — deterministic test escape hatch, runs before any FS touch.
2. `MkdirAll` failure → one `fmt.Fprintf(os.Stderr, ...)` warning + `setStderrOnly()` + `return nil`.
3. `OpenFile` failure → same shape.
4. `Init()` returns nil on every M1 path; signature stays `error` so future I/O failures can surface without a churn diff.

The warning text is loadbearing: name what's happening, what we did about it, the underlying cause. `decloud: log dir unavailable, using stderr only: <err>` matches all three. Do not collapse to `Init() {}` — keep the error return for future use.

## What this combo prevents

- Exit-70-on-`--help` on a fresh box (the original B1 blocker, `011-kevlin-review.md`).
- A locked-out operator on day-one before bootstrap who can't even read help text.
- Test runs on macOS dev boxes that fight `/opt/declouding` permissions.

The pattern generalizes: any future Cobra command that needs an FS-touching init should prefer `PersistentPreRunE` + fallback over `OnInitialize` + bail.
