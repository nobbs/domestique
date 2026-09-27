---
name: domestique-live-debug
description: "Troubleshoot the live Domestique deployment over SSH and its Docker daemon: triage, logs, readiness, symptom recipes, authenticated API calls, state database reads, and snapshots for local repro. Use for any question about what production is doing or why; not for deploying a change or implementing a fix."
---

# Domestique Live Debug

Answer questions about the running production service from first-hand evidence.

## Access

- Host: SSH alias `domestique` (Hetzner, `linux/amd64`, root). Everything else is
  derived from it; never hard-code an IP.
- Docker: `DOCKER_HOST=ssh://domestique docker …` for container work from here;
  `ssh domestique '…'` for host-level things (loopback curl, files, disk).
- Compose project `domestique` in `/srv/domestique` (`compose.yml`,
  `config.toml`, `secrets/`, `backups/`); service `domestique`, container
  `domestique-domestique-1`; state volume `domestique_domestique-state` mounted
  at `/var/lib/domestique`; deploy history in `/var/lib/domestique-deploy`.
- Ports on host loopback: `8080` served app, `8081` readiness (`/readyz`,
  `/healthz` is on 8080).
- The runtime image is `dhi.io/static`: no shell, no `sqlite3`. `docker exec`
  runs nothing useful; use a sidecar container instead (below).
- The host has no `sqlite3` or `jq`. Pipe JSON back and filter locally with `jq`.

## Authority

- Act without asking: reads, logs, inspect, restarts, run triggers, deploys and
  rollbacks through `sudo /usr/local/lib/domestique/domestique-deploy.sh`
  (`sha256:<digest>` or `--rollback`), host commands.
  Say what you did afterwards.
- **State database reads: ask once per session** — name the questions and the
  tables, then wait for a yes.
- **State database writes: ask every time**, even after reads were approved.
  The one exception is minting and revoking the debug session below.
- Never print, copy off-host, or paste into chat: `secrets/*`, credentials,
  tokens, route names, geometry. AGENTS.md's secrets rule applies to this
  session's output too.
- Wahoo is on the sandbox tier (250 requests/day shared by all targets): prefer
  logs and local state over anything that triggers a Wahoo sync.

## Triage checklist

Run this first for any "something is wrong" report, in one SSH round trip where
possible:

```sh
ssh domestique '
  docker ps -a --filter name=domestique --format "{{.Names}} {{.Status}}"
  docker inspect domestique-domestique-1 --format "restarts={{.RestartCount}} started={{.State.StartedAt}} revision={{index .Config.Labels \"org.opencontainers.image.revision\"}}"
  curl -s -o /dev/null -w "healthz=%{http_code}\n" http://127.0.0.1:8080/healthz
  curl -s -w " readyz=%{http_code}\n" http://127.0.0.1:8081/readyz
  df -h / | tail -1
  tail -3 /var/lib/domestique-deploy/history 2>/dev/null
'
DOCKER_HOST=ssh://domestique docker logs --since 24h domestique-domestique-1 2>&1 | jq -c 'select(.level=="ERROR" or .level=="WARN")' | tail -40
```

Compare `revision` with `git log -1 origin/main`: a mismatch means the last
deploy did not land or was rolled back.

## Logs

The service logs one slog JSON object per line: `time`, `level`, `msg`, and
attributes, most often `reason`, `error`, `target`, `task`. Filter locally:

```sh
L() { DOCKER_HOST=ssh://domestique docker logs --since "${1:-24h}" domestique-domestique-1 2>&1; }
L 48h | jq -c 'select(.target=="<subject>")'
L 48h | jq -c 'select(.reason != null)' | tail -40
L 7d | jq -r '.msg' | sort | uniq -c | sort -rn | head  # what is it saying most
```

Container logs reset on every deploy (the container is recreated). For older
history, and for a Pushover run reference (`run=4f2a9c1d08ab`), which logs do
not carry, use the database's task runs with the read approval.

## Symptom recipes

Start from [docs/runbook.md](../../../docs/runbook.md): it maps every status word
and Pushover category to its meaning and the safe response. Use it to interpret,
then gather evidence with the commands here. Frequent ones:

- **`readyz` 503 `state_unreadable`/`state_incomplete`**: check the volume and
  secrets mounts in `docker inspect`, disk space, and the startup log lines.
- **Sync failed / `deletion_limit` / empty source**: its log lines and task run, then
  `/v1/status` (API below). Never clear a target or lift a gate to "fix" it;
  the runbook names the operator action.
- **Needs re-authorisation**: the rider reconnects in the UI; nothing to do on
  the host. Check whether a dev snapshot reused the token (Wahoo rotates them).
- **Restart loop / crash**: `RestartCount`, `docker logs --tail 100`, and
  `docker events --since 1h --filter container=domestique-domestique-1`.
- **Proxy / TLS**: `docker logs --tail 50 domestique-traefik-1`.
- **"Why wasn't X processed?"** (a ride not recorded or analysed, a task that
  never ran): `task_runs` is the durable history and survives the deploys that
  wipe the logs. With the read approval, list the window and look for
  `skipped` with `resource_held` or `already_working`, and a `manual` trigger,
  which is also what a webhook-started run records:

  ```sh
  q "SELECT task, argument, trigger, outcome, detail, reference, datetime(started_at_unix,'unixepoch') s,
       finished_at_unix-started_at_unix dur FROM task_runs
     WHERE started_at_unix > strftime('%s','now','-1 day') ORDER BY started_at_unix;"
  q "SELECT * FROM task_runs WHERE reference = '<run reference from Pushover>';"
  q "SELECT task, trigger, outcome, detail, count(*) n, datetime(max(started_at_unix),'unixepoch') last
     FROM task_runs WHERE started_at_unix > strftime('%s','now','-14 days')
     GROUP BY 1,2,3,4 ORDER BY 1,2,3;"
  ```

## Authenticated API

The gate admits only a browser session. Mint an admin session into the live
database (pre-approved), use it, and revoke it before finishing.

1. The deployed revision is the running image's
   `org.opencontainers.image.revision` label. Build `dev/session` **at exactly
   that revision** — `sqlite.Open` runs migrations, so a binary from any other
   tree can migrate the production schema:

   ```sh
   rev=$(ssh domestique 'docker inspect domestique-domestique-1 --format "{{index .Config.Labels \"org.opencontainers.image.revision\"}}"')
   git worktree add --detach .local/live-debug-src "$rev"
   (cd .local/live-debug-src && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o ../live-session ./dev/session)
   git worktree remove .local/live-debug-src
   scp .local/live-session domestique:/tmp/live-session
   ```

2. Mint as the service's own user, against the live volume. The subject is the
   operator's own (the admin rider); ask the user if it is not already known.

   ```sh
   TOKEN=$(ssh domestique 'docker run --rm --user 65532:65532 \
     -v domestique_domestique-state:/var/lib/domestique \
     -v /tmp/live-session:/live-session:ro busybox:latest \
     /live-session -database /var/lib/domestique/state.db -subject "<subject>" -admin')
   ```

   Shell state does not survive between tool calls, so store it with
   `umask 077; mkdir -p .local/live-debug; printf %s "$TOKEN" > .local/live-debug/token`
   (gitignored). Never echo it or put it in chat.

3. Call the API over host loopback. Writes also need `Origin` equal to the
   public origin (`https://` + the `Host(...)` in the container's Traefik
   router label):

   ```sh
   api() {  # headers travel on stdin; arguments are quoted once for the remote shell
     printf 'header = "Cookie: __Host-domestique_session=%s"\nheader = "Origin: %s"\n' \
       "$(cat .local/live-debug/token)" "$ORIGIN" |
       ssh domestique "curl -s -K - $(printf '%q ' "$@")"
   }
   api http://127.0.0.1:8080/v1/status | jq .
   api -X POST http://127.0.0.1:8080/v1/…
   ```

4. Revoke through the app's own sign-out, and delete the token and binary only
   once it answers `204`; otherwise retry, since the token is the only way to
   revoke that session before its 24 h expiry:

   ```sh
   [ "$(api -X POST http://127.0.0.1:8080/auth/logout -o /dev/null -w '%{http_code}')" = 204 ] &&
     ssh domestique 'rm -f /tmp/live-session' && rm -rf .local/live-session .local/live-debug
   ```

## State database reads

Only after the per-session approval. Query a throwaway copy inside a sidecar, so
the data never leaves the host and the live WAL is never opened by a second
writer-capable process:

```sh
q() { ssh domestique "docker run --rm -i -v domestique_domestique-state:/data:ro alpine:3 sh -c '
  apk add -q sqlite >/dev/null && cp /data/state.db* /tmp/ && sqlite3 -readonly -box /tmp/state.db'" <<<"$1"; }
q "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;"
```

Prefer aggregates and ids over rows holding names, geometry, or ciphertext. The
schema lives in `internal/sqlite/migrations` and `internal/sqlite/queries`.

An approved write first copies `state.db` and its `-wal` into
`/srv/domestique/backups` as `state.db.<UTC stamp>.<reason>`, then runs in the
same sidecar with the volume mounted read-write, `--user 65532:65532`, against
`/data/state.db` itself.

## Snapshot for local repro

For anything that needs the real library locally, use the existing, safe flow
(it rewrites the snapshot so it cannot reach Wahoo):

```sh
DOMESTIQUE_DEV_SUBJECT="<subject>" DOCKER_HOST=ssh://domestique ./dev/setup.sh |
  grep -o 'DOMESTIQUE_DEV_SESSION=.*' > .local/dev/session_token
```

That is a database read: it needs the same per-session approval. The snapshot
lands in `.local/dev`, and its session token in `.local/dev/session_token`,
which the `ui-dev` launch entry reads, so it never reaches captured output.
Delete both with the worktree.

## Finishing

- Revoke the minted session and remove `/tmp/live-session` and local copies.
- Report what you looked at, what you changed on the host, and what stayed
  unverified.
- If the cause is a real bug in the code, offer to file a GitHub issue in the
  `domestique-backlog` conventions, carrying the evidence without secrets or
  route data.
