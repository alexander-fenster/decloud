# Identity flows as an explicit field, not derived by string-munging another field

When a function needs an identity value (service name, account id, deploy id) and a related-but-different field (container name, ARN, branch path) is in scope, the temptation is `strings.TrimPrefix(req.Name, "decloud-")` or `strings.Split(arn, "/")[2]`. Resist. Add the identity as its own field on the struct and require every call site to populate it from the source-of-truth available at the call site.

## The smell that motivated this

`internal/dockerdrv/cli_driver.go` used to derive the `decloud.service` label by `strings.TrimPrefix(req.Name, "decloud-")`. Worked. Couples two fields by a string-shape contract that lives nowhere — a future M4 container rename (`decloud-<name>-<deploy-id>` per `_ai/container-naming.md`) would silently produce per-deploy label drift, breaking `journalctl CONTAINER_TAG=decloud/<service>` queries across redeploys with no compile-time warning.

The fix was `Service string` on `RunRequest` and `RunOptions` (`internal/dockerdrv/driver.go:47` and `:94`), populated by four production call sites:

- `internal/deploy/service.go:246` — fresh deploy (`req.Name`).
- `internal/deploy/service.go:379` — rollback (`prev.Config.Name`).
- `internal/deploy/lifecycle.go:69` — absent-branch re-run (function arg).
- `internal/caddy/manager.go:127` — caddy manager (hardcoded `"caddy"`).

## Driver-layer guard catches the zero-value accident

Adding the field is half the discipline; the other half is a guard at the leaf that rejects empty/invalid values BEFORE the side effect. `ErrEmptyService` (driver.go:22-34) fires when a future `RunRequest{}` literal forgets the field — the test suite fails loudly at the new site, not at runtime under journalctl. Pair with `two-sentinels-for-two-failure-modes.md`.

## When this applies

Any time you find yourself writing:

- `strings.TrimPrefix(other, "prefix-")` to recover identity.
- `strings.Split(combined, "/")[N]` to extract an embedded id.
- `regexp.MustCompile(...).FindStringSubmatch(name)[1]` to parse out a sub-field.

Stop. Add the identity as a field. The cost is N call-site touches (mechanical) plus one leaf guard (locks against zero-value accidents). The benefit is a structural contract that survives future renames of the *other* field.

## When NOT to apply

When the identity genuinely IS the other field — e.g., the deploy id IS the trailing path segment of the staging directory, by directory-layout design — `filepath.Base(stagingDir)` is fine because the path layout itself is the contract, locked elsewhere. The smell is *deriving* identity from a string that holds *something else* (container name embeds service name with a `decloud-` prefix; ARN embeds account in position 4).

## Originator

`_tasks/2026-05-12-journald-log-driver/` — Don's plan §3 named the smell, Joel's tech plan §5.2 / §11.5 specified the field placement and population rule, Kevlin §2 verified `grep -rn 'TrimPrefix.*decloud-' internal/ cmd/` returns no matches post-task. Decision record at `_ai/decisions/journald-log-driver.md` "Why `Service` flows explicitly" section has the longer narrative; this file extracts the general rule.
