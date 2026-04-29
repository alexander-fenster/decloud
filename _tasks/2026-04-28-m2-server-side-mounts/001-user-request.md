# 001 — User request

The user invoked `/do implement M2 features`.

## Resolved scope

Per `_ai/decisions/m1-scope.md` line 32 (the canonical milestone roadmap, post-2026-04-28 resequence):

> M1 service deploy MVP → **M2 server-side mounts + env-file hardening (`--mount` flag, loader populates `Mounts`)** → M3 host bootstrap (introduces Viper, `caddy.image` config knob) → M4 zero-downtime blue/green via Caddy admin API → M5 jobs (systemd timers) → M6 backups + image GC → M7 operational polish (supervisor, deploy locks, secret-files-on-disk, client binary, etc.).

**M2 scope:**

1. **`--mount` flag** — server-side `decloud deploy service` accepts a repeatable `--mount` flag and the loader populates the existing `Mounts` field in the per-service TOML.
2. **Env-file hardening** — improvements to env-file handling that were originally bundled with M3a in the older roadmap.

**Explicitly NOT in M2** (per the resequence):

- **No secret-files-on-disk** — that is M7. Do not add `secrets/<name>/files/` reading or any on-disk secret-file substructure.
- **No Viper, no `/etc/decloud/config.toml`** — that is M3. Mounts are per-service via `--mount` flag and per-service TOML; do NOT introduce a global config file in M2 even for "default mount options".
- **No client binary** — M7.
- **No blue/green** — M4 (and the M2 work must not break the recreate strategy or container-naming-by-`decloud-<name>`).

## Current state to be unwound by M2

- `internal/cli/deploy_service.go` rejects `--mount` with exit 10 (the `ErrMountsNotSupported` path).
- The loader rejects non-empty `Mounts` in per-service TOML with the same error (closes the hand-edit loophole at M1).
- `_docs/usage.md` line 71 documents that `--mount` is rejected and "Persistent volumes are M2".
- `Run.Mounts` field exists in the schema and was reserved at M1 for this exact moment (per `_ai/decisions/schema-versioning.md`: shape doesn't change between milestones, only values).

## Workflow

CLAUDE.md mandates the full agent workflow (Don plan → Joel tech plan → Linus review → Kent tests → Rob impl → Raymond docs → Kevlin+Linus review → Don/Joel/Linus closeout → Ward learnings → Andy agent review → squash-merge). NO CODING AT TOP LEVEL. Branch: `feat/m2-server-side-mounts`.

## Cross-references

- `_ai/decisions/m1-scope.md` §"Milestone sequence (M1 → M7)" and §"Explicit M1 cuts" (the `--mount` and `Mounts`-rejection lines).
- `_ai/decisions/schema-versioning.md` §"M2 writes `schema_version = 1`. M2 populates `Mounts`."
- `_ai/m1x-backlog.md` item 6 — the integration smoke test belongs to M2 (first post-M1 milestone that touches real Docker).
- `_ai/cli-flag-surface-coherence.md` — `--mount`'s help text currently asserts `"M2"` substring; once M2 ships, that contract changes meaning (no longer "rejected, coming in M2" but "now supported").
- `_tasks/2026-04-28-milestone-resequence/` — the resequence rationale; M3a became M2 minus secret-files.
