# jdbc-cli — Spec

## Goal

A CLI for ad-hoc SQL over JDBC that mirrors `officecli`'s resident-mode
ergonomics: short-lived `jdbc-cli <subcmd>` invocations backed by a
long-running daemon that keeps the JVM and a per-alias HikariCP pool warm.
An agent can fire 20 small queries in a row without paying JVM cold-start
or JDBC connect cost on each one.

The same daemon is the future backend for the dynamic MySQL MCP shim
([2026-05-05 follow-up](https://example.invalid/)) — one connection pool,
two front-ends.

## Non-goals (v1)

- Lazy/dynamic driver loading (Coursier `URLClassLoader`). v1 fat-jars three drivers.
- MCP front-end. v1 ships only the CLI front-end.
- Result paging, server-side cursors, audit log.
- Windows or Linux support. macOS only (launchd, Keychain, Homebrew paths).
- Multi-user. The daemon assumes a single Unix user; socket is `0600`.

## Drivers (v1)

Bundled in the fat-jar:

| DB         | Maven coordinates                           |
| ---------- | ------------------------------------------- |
| MySQL      | `com.mysql:mysql-connector-j:9.4.0`         |
| PostgreSQL | `org.postgresql:postgresql:42.7.4`          |
| SQLite     | `org.xerial:sqlite-jdbc:3.46.1.3`           |

Adding a driver post-v1 = bump Gradle deps + rebuild fat-jar. Lazy
Coursier loading is a v2 concern.

## CLI surface

```
jdbc-cli daemon                                          # foreground, used by launchd
jdbc-cli list                                            # active aliases (JSON array)
jdbc-cli open    --alias <a> --jdbc-url <u> \
                 --user <u> [--password-stdin | --password-keychain <service>]
jdbc-cli query   --alias <a> "SELECT ..." [--json]
jdbc-cli exec    --alias <a> "UPDATE ..."
jdbc-cli schema  --alias <a>                             # list tables
jdbc-cli describe --alias <a> --table <t>
jdbc-cli begin   --alias <a>
jdbc-cli commit  --alias <a>
jdbc-cli rollback --alias <a>
jdbc-cli close   --alias <a>
jdbc-cli batch   --alias <a> < ops.jsonl                 # NDJSON ops
jdbc-cli ping                                            # daemon liveness check
```

### Output

- Default for `query`/`schema`/`describe`: TSV with header row.
- `--json`: array of objects with **typed** values (numbers as JSON
  numbers, booleans as `true`/`false`, NULL as `null`, dates/timestamps
  as ISO-8601 strings, BLOBs as base64).
- Errors: exit non-zero; one-line JSON `{"error":"..."}` on stderr.

### Credentials

Two paths, both keep the password off `argv`:

1. **`op run`** — pipe via stdin:
   ```bash
   op run --env-file=secrets.env -- bash -c '
     printf "%s" "$DB_PASSWORD" | jdbc-cli open \
       --alias prod --jdbc-url jdbc:mysql://… --user root --password-stdin
   '
   ```
2. **Keychain** — daemon reads on `open`:
   ```bash
   security add-generic-password -s jdbc-cli/prod -a root -w 'pwd'
   jdbc-cli open --alias prod --jdbc-url … --user root \
                 --password-keychain jdbc-cli/prod
   ```
   Daemon shells out to `security find-generic-password -s <service> -a <user> -w`.

The password is stored only inside the Hikari pool object. `close`
destroys the pool and drops the reference.

## Transport

HTTP/1.1 over a Unix domain socket at `~/.jdbc-cli/sock` (mode `0600`).
Jetty 12 `UnixDomainServerConnector`. Curl-debuggable:

```bash
curl --unix-socket ~/.jdbc-cli/sock http://localhost/list
```

Routes:

| Method | Path           | Body                              | Returns          |
| ------ | -------------- | --------------------------------- | ---------------- |
| GET    | `/ping`        | —                                 | `"ok"`           |
| GET    | `/list`        | —                                 | `[alias, ...]`   |
| POST   | `/open`        | `{alias, jdbcUrl, user, pass}`    | `"ok"`           |
| POST   | `/close`       | `{alias}`                         | `"ok"`           |
| POST   | `/query`       | `{alias, sql}`                    | `[{col: val}]`   |
| POST   | `/exec`        | `{alias, sql}`                    | `{updated: N}`   |
| POST   | `/schema`      | `{alias}`                         | `[{table, …}]`   |
| POST   | `/describe`    | `{alias, table}`                  | `[{column, …}]`  |
| POST   | `/begin`       | `{alias}`                         | `"ok"`           |
| POST   | `/commit`      | `{alias}`                         | `"ok"`           |
| POST   | `/rollback`    | `{alias}`                         | `"ok"`           |
| POST   | `/batch`       | NDJSON of op objects              | NDJSON results   |

## Lifecycle

- launchd plist at `~/Library/LaunchAgents/com.scriptease.jdbc-cli.plist`,
  `KeepAlive=true`, `RunAtLoad=true`.
- Daemon clears `~/.jdbc-cli/sock` on startup, binds, sets mode `0600`.
- On `SIGTERM`: walk pools, `close()` each, exit 0. launchd revives it.
- Aliases are **not persisted** across restarts. Re-`open` after a crash.

## Acceptance tests

All run against real databases stood up in the playbook.

1. `jdbc-cli ping` returns `ok` with a fresh launchd-managed daemon.
2. `jdbc-cli open --alias t --jdbc-url jdbc:sqlite::memory: --user "" --password-stdin <<<""` then `jdbc-cli list` includes `t`.
3. Round-trip against MySQL: `open` → `exec "CREATE TABLE …"` → `exec "INSERT …"` → `query "SELECT …"` returns the inserted rows; `--json` produces typed values (int, string, null).
4. Round-trip against Postgres equivalent.
5. Transaction: `begin` → `exec UPDATE` → `query` (sees the change) → `rollback` → `query` (change gone).
6. Two aliases concurrent: opens against MySQL and Postgres in parallel; queries against both in interleaved sequence; both succeed.
7. `op run` flow: password reaches daemon via stdin; `ps auxww` during the call shows no `--password=…`.
8. Keychain flow: `--password-keychain svc/acct` opens pool; same `ps` check passes.
9. Crash-survival: `pkill -f jdbc-cli.jar` → launchd respawns within 5 s → `ping` returns `ok`; aliases are gone (expected).
10. SIGTERM: open alias, `launchctl kickstart -k …`; daemon logs show clean pool close (no Hikari "leaked connection" warnings).

## Out of scope for this spec

Driver hot-loading, MCP front-end, paging, audit log, query timeout
flags, multiple concurrent transactions per alias. All deferred.
