# Two-file split: config (0644) + secrets (0600)

The README's "Handling secrets" section requires structural separation of secrets from operational config on disk — not just logical separation inside one file. Each registered service is therefore TWO files:

- `/opt/decloud/config/services/<name>.toml` — root:root, mode **0644**, world-readable. Holds non-secret operational metadata: `schema_version`, `name`, `source`, `build`, `run` (incl. `mounts` paths), `routes`, `strategy`, `readiness`, `state`. Safe to inspect; future "git-mirror the non-secret config" idea stays cheap.
- `/opt/decloud/secrets/<name>/env.toml` — root:root, mode **0600**, in a directory at mode **0700**. Holds `schema_version` (must match config) + `env` map (the `env.sh` capture). M3 will add `secrets/<name>/files/` for secret file contents.

`Env` lives ONLY in `ServiceSecrets`. Strict-mode loader (`pelletier/go-toml/v2` `DisallowUnknownFields`) rejects any config TOML that tries to smuggle `env` back in — the structural split is enforced by the type system.

## Load/Save/Delete ordering — produces only the recoverable failure mode

Two-file writes are NOT cross-atomic. Pick the ordering so the only crash-window inconsistency is recoverable.

- **Save (create or update): config first, then secrets.** Crash window leaves "config without secrets" → loader returns `ErrSecretsMissing` → operator re-runs deploy → env re-captured from source → converges. The alternative (secrets first) would leave an orphan secrets file with no registration pointing at it, which `List` would silently skip.
- **Delete: secrets first, then config.** Same recoverable inconsistency on crash. Reverse order would orphan secrets.
- **Failed-deploy cleanup vs successful-prior-create:** if Save fails mid-write during a fresh deploy attempt, the deployer DELETES the just-written config file to avoid leaving deploy-failure orphans. But "config without secrets" from a successful prior create that later lost its secrets is preserved as a recoverable signal. Subtle but correct distinction.

Per-file atomicity uses `os.CreateTemp` in the same dir + `Chmod` (defends against umask) + `Sync` + `os.Rename`. Same-filesystem rename is POSIX-atomic.

## Load enforces permissions, never silently fixes them

Loader rejects (`ErrPermissionMode`) if secrets file isn't 0600 or its dir isn't 0700, naming the offending path and observed mode. We do NOT silently fix — silently fixing hides the audit signal of whatever process broke them.

Other loader rejection classes (all map to exit code 10 = `ExitConfigError`): `ErrNotFound`, `ErrSecretsMissing`, `ErrSchemaMismatch` (cross-file mismatch is also rejected), `ErrUnknownField` (strict mode), `ErrMountsNotSupported` (M1), `ErrInvalidStrategy` (M1).

## Rejected alternatives

- **A: mode 0600 on a single combined TOML** — breaks structural separation; non-secret metadata becomes root-only readable; future git-mirror story dies.
- **C: defer the split to M3 with a schema bump** — ships M1 with a known security regression and forces M3 to do data migration. No.

See `internal/registry/types.go`, `internal/registry/store.go` once implemented; tech plan v2 §4 for the type shapes and exact loader semantics.
