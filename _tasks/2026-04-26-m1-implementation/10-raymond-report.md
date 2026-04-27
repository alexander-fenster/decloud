# Step 3c — Raymond: M1 docs

**Author:** Raymond Chen (doc writer)
**Status:** Two operator-facing docs written, every flag and behavior verified against shipped code (not against the plans).

---

## Files created

- `/Users/fenster/dev/decloud/_docs/install.md` — server-side installation. Prereqs, Docker install (link to upstream), Caddy install with the systemd unit drop-in pointing at `/opt/decloud/config/caddy/Caddyfile`, the `/opt/decloud/` tree with exact modes, `docker network create decloud`, `go install` of the binary, license-not-yet sentence, verify step. Explicitly notes: no `decloud` daemon, no `systemctl enable decloud`, the operator runs `decloud` ad hoc over SSH.
- `/Users/fenster/dev/decloud/_docs/usage.md` — operator usage. Quick start, `decloud deploy service` with the full flag table, exit codes 0/2/10/20/30/40/50/60/70, one-paragraph reference for each of the seven lifecycle commands (`unregister`, `start`, `stop`, `restart`, `status`, `logs`, `caddy reload`), the `<name> state=... container=... deploy=... deployed_at=<RFC3339>` status format with the four state values (`running`, `stopped`, `absent`, `config-only`), an end-to-end example (deploy → status → logs → rollback by re-deploying previous source → unregister), the `docker exec -it decloud-<name> sh` debug paragraph, the `caddy reload` recovery paragraph, and the warning that `decloud caddy reload` regenerates from registry and discards manual Caddyfile edits.

CLAUDE.md mentions a Next.js documentation app under `_docs/`; in this repo `_docs/` does not exist yet. Don's plan §3 (and Joel's tech plan) explicitly call for plain markdown under `_docs/`. I wrote plain markdown. The `_docs/operator/` subdirectory the plans named was simplified to flat `_docs/install.md` and `_docs/usage.md` per Don's §9 "Raymond's call on document structure; content is what matters."

## Verifications performed against shipped code, not plans

- All `decloud deploy service` flags by reading `internal/cli/deploy_service.go` and running the built binary's `--help` output.
- Exit-code constants from `internal/cli/exit_codes.go`.
- Lifecycle behavior from `internal/deploy/lifecycle.go` — confirmed `Stop` does no Caddy reload, `Start` from `absent` re-runs from `prev.Config.Build.ImageRef` on the `decloud` network, `Restart` is stop-then-start (not `docker restart`), `Status` maps Docker's `exited` to `stopped` and surfaces `ErrSecretsMissing` as `config-only`.
- Status output format from `internal/cli/status.go` line 25 — exactly `<name> state=<state> container=<container-name> deploy=<deploy-id> deployed_at=<RFC3339>`.
- `ContainerName` is `decloud-<name>` per `internal/ids/ids.go`.
- Deploy ID format is `YYYYMMDD-HHMMSS-XXXXXX` (six-hex suffix) per `internal/ids/ids.go` — fixed my example after first writing a wrong one.
- Default flag values: `--readiness-path=/healthz`, `--readiness-timeout=60s`, `--strategy=recreate`, `--dockerfile=Dockerfile`. Verified against the binary.
- Path layout from `internal/config/paths.go`: `/opt/decloud/{config/{services,jobs,caddy},secrets,state/deploys,logs}` with `Caddyfile` at `config/caddy/Caddyfile`.

## Things I deliberately did NOT document (per plan §2.2.2)

- No client binary. There is none in M1.
- No `decloud deploy job`. M1 has no job type.
- No backup, no `decloud bootstrap`, no volumes. M2/M3/M6.
- No `--strategy=blue_green`. Documented only as "rejected with exit 10, M4."
- No `DECLOUD_LOG_TO_STDERR_ONLY=1`. Test-only escape hatch per Rob's report.
- No `--json` flag on `status`. M1.5 if anyone asks (Don §9).

## Plan vs. code disagreements

None worth flagging. Rob's deviations (probe injection seam, `Config.Name` overwrite from filename, wider `ErrUnknownField` mapping, Cobra usage-error substring fallback) are all internal — they do not surface to the operator and required no doc adjustments. The shipped CLI surface matches Joel §8 verbatim.

## Things I trusted

When the plans and Rob's report agreed, I trusted them. When the code differed in detail (e.g., the deploy-ID format string), I trusted the code. Every flag, default, and exit code in the docs came from re-reading the source, not from the plans.

End of Raymond report.
