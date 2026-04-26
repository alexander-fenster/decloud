# TOML schema versioning policy

`schema_version` is an integer at the top of every persisted TOML (both halves of the config/secrets split must match). Forward compatibility relies on TWO mechanisms in tandem:

1. **`pelletier/go-toml/v2` strict mode (`DisallowUnknownFields`)** — old binary reading new file fails with `ErrUnknownField` naming the offending field. Catches new-field additions.
2. **Explicit `schema_version` integer** — bumped only when a field's *meaning* changes in a way that breaks an old loader (e.g. repurposing `routes` to mean something different). Catches semantic breaks.

## The rule

- M1 writes `schema_version = 1`.
- M3 writes `schema_version = 1`. M3 only populates fields that M1 reserved (`Mounts`, future secret-file declarations under `mounts`); the schema *shape* doesn't change.
- Bump to 2 ONLY when forced by a semantic break. Never preemptively reserve fields by bumping.

## What "reserve fields" looks like in practice

M1 declares the full schema shape — including fields M1 won't populate (`Mounts` always empty in M1). M1's loader rejects non-empty `Mounts` with the same `ErrMountsNotSupported` as the CLI's `--mount` flag (closes the hand-edit loophole). M3 starts populating; no file rewrite, no migration code. An M1-era TOML loads cleanly in an M3 binary because the shape is identical, only the values differ.

## Escalation rule

If during M1 implementation Kent or Rob discovers the schema fundamentally cannot work and needs a v2 — **stop and re-plan**. Do not silently introduce migration code mid-milestone. A schema bump in M1 means the design was wrong and we need to think before shipping.

## Cross-file mismatch

The two halves (config and secrets TOML) must declare the same `schema_version`. Mismatch is `ErrSchemaMismatch` → `ExitConfigError`. Catches the case where one file got upgraded and the other didn't.

See tech plan v2 §5 for exact loader code.
