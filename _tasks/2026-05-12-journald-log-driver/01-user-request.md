# Task: journald-log-driver

## User's original request

> implement a change to how docker containers are started: use --log-driver=journald with the log option tagging the logs by the service name to make sure the logs survive container redeployment

## Context from conversation

The user noticed that when a service is redeployed in Decloud, the new container
has fresh logs and the old container's logs are lost (because Docker's default
`json-file` log driver stores logs inside the container, which is destroyed on
redeploy).

Two options were discussed:

1. `--log-driver=journald` with a per-service tag.
2. `--log-driver=syslog` writing to per-service files on the host.

The user chose **journald** because:

- `docker logs` / `decloud logs` continue to work natively with the journald
  driver — it's one of the few Docker log drivers where the daemon can still
  read logs back from the underlying store, so the existing CLI surface keeps
  working unchanged.
- Cross-restart / cross-redeploy log history is preserved on the host via
  `journalctl` queries such as
  `journalctl CONTAINER_TAG=decloud/<service>`
  (exact tag scheme to be confirmed during planning).

## Intent

Wherever Decloud starts containers via `docker run` (server-side bootstrap of
a service's container in `/root/staging/<service>/`), add:

    --log-driver=journald
    --log-opt tag=decloud/<service>

…or the equivalent. The exact tag format (`decloud/<service>` vs.
`decloud-<service>` vs. something else) should be confirmed during the
planning step, including whether journald sanitises slashes in tags and how
that interacts with `journalctl CONTAINER_TAG=` filtering.

## Out of scope for this task (unless planning surfaces them as necessary)

- Changing the `decloud logs` client command itself.
- Log rotation / retention policy for journald on the host.
- Migrating historical logs from existing containers.
