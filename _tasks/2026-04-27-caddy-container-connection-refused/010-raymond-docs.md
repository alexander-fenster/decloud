# 010 — Raymond: documentation update report

Author: Raymond Chen (doc-writer agent)
Date: 2026-04-27
Status: EXECUTION step 3.3 complete. Docs reflect the containerised-Caddy architecture per Don v2 / Joel v2 / Rob's implementation.

## Files touched

### `_docs/install.md` (rewritten)

Full rewrite to match the new architecture:

- **§1 Prerequisites** — dropped the systemd phrasing ("Linux host with systemd") to "Linux host with root access" since M1 no longer ships a systemd unit; added an explicit note that AAAA records are fine (dual-stack publishing).
- **§3 Install Caddy → Bring up Caddy.** The host-Caddy systemd block (the M1.0 §3 from line 28 through line 63, including the full `caddy.service` unit and the "Caddy will fail to start until the Caddyfile exists" / "`caddy` binary must be on the operator's `PATH`" paragraphs) is **deleted**. Per Linus §6 #3 and Don's §10 criterion #12 — DELETED, not edited. Replaced with a description of `decloud caddy up`: idempotent, ensures network, writes stub Caddyfile, pulls `caddy:2`, runs `decloud-caddy` with dual-stack publishing on `80/tcp`, `443/tcp`, `443/udp`, named volumes for ACME state.
- **§3.1 Host firewall** — new. Names `80/tcp`, `443/tcp`, `443/udp` and explains that missing UDP/443 silently degrades HTTP/3.
- **§3.2 Migrating from M1.0.** Volume-copy of `/var/lib/caddy/.local/share/caddy` into the `decloud_caddy_data` named volume is the recommended default. Cold-restart is the alternative for ≤2 hostnames. Let's Encrypt rate limits are spelled out: 50 certs/domain/week, 5 duplicate certs per identical SAN set per week, 7-day recovery window if the per-domain weekly cap is tripped. The "extra second" softball is gone. `systemctl mask caddy` AND `apt-get remove -y caddy` both named explicitly because `disable --now` is undone by package upgrades.
- **§4 `/opt/decloud/` tree.** Unchanged except for the SELinux NIT (one line) per Joel v2 §11.2.
- **§5 Install the `decloud` binary.** Renumbered from §6.
- **§6 Bootstrap order and first deploy** — new. The operator-facing five-step order: tree → binary → `caddy up` → deploy.
- **§7 Troubleshooting** — new. Three failure modes with diagnostic shape: ports already bound (`systemctl mask` / `apt-get remove`), IPv6 listener fails (kernel disabled), Caddy can't reach upstream (verify `decloud-caddy` on `decloud` network), Caddy is not routing after a deploy (run `caddy up` and `caddy reload`).
- **§8 License** / **§9 Next steps** — renumbered.

The previous §5 ("Create the shared Docker network") is collapsed into §3 — `decloud caddy up` ensures the network, so the explicit `docker network create decloud` step is redundant. The §61-62 paragraph ("Caddy will fail to start until the Caddyfile exists" + "The `caddy` binary must be on the operator's `PATH`") is **deleted** as the plan demanded.

### `_docs/usage.md` (edits)

- **§1 Quick start** — added a four-line "Caddy must already be running before the first deploy" block pointing operators at `decloud caddy up` (cross-link to `install.md` §3).
- **§2 Step 7** — rewritten. Now reads `docker exec decloud-caddy caddy validate ...` and `docker exec decloud-caddy caddy reload`, with an explicit note that the deploy exits 60 if `decloud-caddy` is not running.
- **§3 Exit code table** — exit 40 mentions `decloud caddy up`/`down`. Exit 60 mentions the "Caddy not running" recovery hint.
- **§4 Lifecycle commands.** Added two new entries: `decloud caddy up` (zero flags, idempotent, dual-stack publishing, named volumes) and `decloud caddy down` (10s grace, volumes preserved). Updated `decloud caddy reload` description to mention `docker exec` and the dependency on `caddy up` having been run. Updated the section preamble (`All seven ship in M1` → `All M1 commands listed below`).
- **§6 Debugging.** "Decloud deliberately does not publish container ports" rewritten — `decloud-caddy` is now the documented exception (binds 80/443 on `0.0.0.0` and `[::]`). Service containers still don't publish.
- **§7 Recovering from `caddy reload` failures.** The investigation recipe `caddy validate --config <tmp-path>` rewritten to use `docker exec decloud-caddy caddy validate --config /etc/caddy/Caddyfile.tmp`. The `journalctl -u caddy` recovery step replaced with `docker logs decloud-caddy --tail 100`. New subsection "`decloud-caddy` is not running" walks the operator through `decloud caddy up` (and `caddy reload` if up reports already-running).

### `_ai/decisions/caddy-runs-in-container.md` (new)

Architecture decision record:

- **Context.** M1.0 chose host-systemd Caddy; the bug from `001-user-request.md` (Caddy dialing the host's public IPv6 instead of the bridge IP) demonstrated that the choice was wrong because Caddy must be on the `decloud` network for embedded DNS to resolve `decloud-<service>` names. Notes that the M1 tech plan caught the same gap for the readiness probe but never asked the corresponding question for Caddy — and that the asymmetry was missed because nobody promoted it to a Decision.
- **Decision.** Caddy runs as `decloud-caddy` on the `decloud` bridge network. `decloud caddy up`/`down` manage it. Reloader uses `Driver.Exec`. Image `caddy:2` hardcoded as `caddy.DefaultImage` (no Viper in M1 per `m1-scope.md`). Dual-stack publishing on 80/tcp, 443/tcp, 443/udp. Named volumes `decloud_caddy_data` and `decloud_caddy_config` for ACME state. Bind mount of `/opt/decloud/config/caddy` read-only at `/etc/caddy`.
- **Rejected alternatives.** Don's A and B (write IPs into Caddyfile; publish service ports with `-p`) plus Linus's seven (host.docker.internal, --network host, --network container:, sidecar, /etc/hosts injection, --resolvers 127.0.0.11, host-local dnsmasq, extra_hosts), each with one-sentence reasoning.
- **Consequences.** Dual-stack publishing on the host (loud failure if kernel IPv6 is disabled). ACME state migration is an operator concern. Host Caddy unsupported. Reloader seam switch (`cmdFactory` → `Driver.Exec`). Deploy errors give one-command recovery. `caddy:2` floats. Concurrent deploys flagged for M2+.
- **Forward-looking notes.** M4 admin API for blue/green will reuse this container model. M2 introduces a `caddy.image` Viper key; `DefaultImage` becomes the fallback.
- **Why not in `_docs/`.** Operators don't need the decision rationale to install or deploy; future contributors need it to avoid relitigating `--network host`.

### `_ai/MEMORY.md` (one-line addition)

Added a bullet under "Architecture decisions" pointing at `decisions/caddy-runs-in-container.md` with the standard one-line summary (Caddy is `decloud-caddy` on the `decloud` Docker network, not a host systemd unit; rejected variants enumerated).

## Files NOT touched

- `_ai/decisions/m1-scope.md` — the "No Viper" entry stays per Don's brief. The line that references `apt install ... caddy` in the rejected "host bootstrap first" alternative is M2-historical and isn't a Decision about Caddy itself; left alone.
- `_ai/m1x-backlog.md` — the integration-test backlog entry the plan mentioned (Don §10 criterion #15) appears to be Ward's deliverable in step 4. I did not add it.
- `_ai/cli-flag-surface-coherence.md` — no new flags introduced (Joel v2 §2.4).
- `_ai/container-naming.md` — already correctly says `decloud-<name>` for M1 services; the `decloud-caddy` container is a separate naming concern (it's a fixed name, not a service name) and doesn't fit that document's M1-vs-M4 rename narrative.
- No `_ai/architecture.md` exists; no `_docs/cli.md` exists. The plan said "find it" — the CLI surface lives in `_docs/usage.md` §4.

## Code-vs-doc inconsistencies flagged

None blocking. Minor observations:

1. **`_docs/usage.md` §2 step 0** still reads "Ensure the `decloud` Docker network exists. Missing networks are created on the fly". With containerised Caddy, by the time a deploy runs, `decloud caddy up` has already ensured the network — the deploy's NetworkEnsure is now redundant on the happy path. Not wrong, just informationally stale on the "missing networks are created on the fly" angle, which is still technically true. Left as-is; the deploy code does still call `NetworkEnsure` (`internal/deploy/service.go:131-134`).

2. **`internal/cli/exit_codes.go`** maps `caddy.ErrCaddyUp`/`caddy.ErrCaddyDown` to `ExitRunFail` (40) per Joel v2 / Rob's report. The exit-code table in `_docs/usage.md` §3 now mentions both commands explicitly under exit 40. Verified consistent.

3. **`internal/caddy/manager.go::runOpts()`** uses `m.cfg.Paths.CaddyDir` as the bind-mount source (verified line 136). The install doc references `/opt/decloud/config/caddy` and the decision record says the same; consistent with `internal/config/paths.go:33`.

4. **`internal/dockerdrv/cli_driver.go::formatPortMap`** splices `HostBind` literally — `[::]` flows through unchanged (verified lines 268-277). The decision record describes the six PortMap entries verbatim; matches `internal/caddy/manager.go:127-134`.

5. **Help text** for `decloud caddy up` and `caddy down` (Joel v2 §1.6) is shorter than the spec — `Short` only, no detailed `Long` block. Verified in `internal/cli/caddy_up.go:13` and `caddy_down.go:13`. Functional and matches Cobra's pattern; not a doc-fix concern, just noting that operators who run `decloud caddy up --help` see a one-line description, not the four-paragraph spec from Joel v2 §1.6. If Kevlin or Linus want the longer help, that's a code-side fix, not a doc fix.

## Style adherence

- No emojis added (none existed in the doc).
- Concise; no obvious-explanation paragraphs.
- "Coupons" / "discount codes" not relevant here (those are Bubblehouse-flavored; Decloud has neither).
- Container, network, and command names verified against `internal/caddy/manager.go`, `internal/cli/caddy_up.go`, `internal/cli/caddy_down.go`, `internal/cli/root.go`, `internal/dockerdrv/cli_driver.go`, `internal/caddy/reloader.go`, and `internal/config/paths.go`. No hallucinated flags — `decloud caddy up` and `decloud caddy down` take no flags (verified `cobra.NoArgs` in both files).

— Raymond
