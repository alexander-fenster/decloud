# Cobra: `StringArrayVar` for paths, `StringSliceVar` for hostnames

`pflag.StringSliceVar` (Cobra's repeatable-flag default) **splits values on commas**. `--mount /a:/x,/b:/y` produces `["/a:/x", "/b:/y"]`, not `["/a:/x,/b:/y"]`. That's wrong for any flag whose values can legitimately contain commas — which includes file paths, mount specs, and most user-typed tokens.

`pflag.StringArrayVar` is the comma-safe variant. Each repetition of the flag becomes one element verbatim. Use this for `--mount`, `--volume`, and anything else where the value is opaque to the framework.

## Rule

- **`StringArrayVar`** for `--mount`, `--volume`, `--label`, anything that takes paths or operator-typed strings.
- **`StringSliceVar`** for `--host`, `--tag`, `--alias` — comma-as-separator is a feature for these, since the values themselves can't contain commas.

## How it bit M2 (caught at plan stage, before code)

`--mount` accepts `<host-path>:<container-path>[:ro]`. Linux paths can contain commas (`/data,backup/x`). With `StringSliceVar`, `--mount /data,backup:/d` would split into `["/data", "backup:/d"]` and then fail parsing as two malformed mount strings instead of one valid one. Joel caught it in tech-plan §8.9; Linus verified in `004-linus-plan-review.md` Issue 6 ("comma-in-path... `StringArrayVar` not `StringSliceVar`. Critical and correct.").

Live site: `internal/cli/deploy_service.go` flag declaration of `--mount`.

## Why this is hard to rediscover

Both functions have nearly-identical signatures. Both work in the happy case (no commas in any value). Tests using fixtures like `--mount /a:/x` pass identically under both. The bug only surfaces when a real operator's path contains a comma — which is rare enough that it can sit latent for years. Grep-discipline:

```bash
git grep -n 'StringSliceVar\|StringArrayVar' internal/cli/
```

For each hit, verify the values cannot contain commas. If they can, it must be `StringArrayVar`.

## Originator

Joel's tech plan §8.9 of `_tasks/2026-04-28-m2-server-side-mounts/003-joel-tech-plan.md`; Linus cross-check at `004-linus-plan-review.md` Issue 6 + §"Cross-checks I performed" item 3.
