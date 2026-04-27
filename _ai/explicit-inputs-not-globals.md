# Take inputs explicitly, not from globals or env-fallbacks

When a function consumes a value that the caller has already resolved (a root dir, a config flag, a secret), make the function take it as a parameter. Do NOT have the function re-read the env var, a singleton, or Viper. Wrong call site can't compile; wrong runtime config can't sneak in past type-checking.

## The bug class this prevents

Two callers on the same machine (or one caller and one test) read different values of the "same" config because one of them lives in code that does its own env-var read. The original bug: `--config-root /tmp/X` set state under `/tmp/X` but `logging.Init()` re-read `DECLOUD_ROOT` and wrote logs under `/opt/declouding`. One config knob, two sources of truth, surprising split — see `_tasks/2026-04-26-fix-deploy-service-review-findings/01-user-request.md` Finding 2.

## Recipe

`internal/logging/logging.go:Init(root string) error`. Caller resolves the root via Cobra's flag-default-from-env mechanism (`internal/cli/root.go:26-27`) and passes the resolved value in (`root.go:23`):

```go
PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
    return logging.Init(rc.ConfigRoot)
}
```

Empty-string fallback inside `Init` mirrors `config.NewPaths` — one source of truth (`config.DefaultRoot`), two consumers, identical fallback rule. If "what no root means" ever changes, it changes in `config.DefaultRoot` and both callers follow.

## Rejected alternatives

- **Setter pattern** (`logging.SetRoot(...)` then `logging.Init()`) — creates two-call ordering bugs; the original Finding 2 was exactly this shape (Cobra set `rc.ConfigRoot` from one source, `logging.Init` read another).
- **Viper read inside `Init`** — package depends on a viper instance being initialized; invents infrastructure to fix a one-line bug.
- **Env-var fallback inside `Init`** — defeats the explicit-input contract and re-introduces the bug class. Hard pass.

## When to apply

Any time a caller has already resolved a config value through Cobra/Viper/CLI parsing. Push the resolved value down as an argument; do not re-resolve in the leaf. Tests assert by passing the value directly — no `t.Setenv` choreography around the unit under test. Locked in by `TestInit_UsesPassedRootNotEnv` (positive AND negative existence assertion on the env path).
