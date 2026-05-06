# jdbc-cli

CLI for talking to SQL databases over JDBC, backed by a resident daemon so
the JVM and connection pools stay warm between calls. Same shape as
`officecli`'s `open … close` model.

- Daemon: Kotlin + Jetty (HTTP over Unix domain socket) + HikariCP
- Drivers bundled in v1: MySQL, PostgreSQL, SQLite
- Credentials via `op run` or macOS Keychain — never via argv
- Single fat-jar; `jdbc-cli` shell wrapper invokes `java -jar …`

## Install

```sh
# From repo root — builds fat-jar, installs wrapper + launchd plist, starts daemon
bash scripts/install.sh
```

Verify the daemon is up:

```sh
jdbc-cli ping   # → pong
```

Restart if needed:

```sh
launchctl kickstart -k gui/$(id -u)/com.scriptease.jdbc-cli
```

Logs: `~/.jdbc-cli/log`

## First run

```sh
# Add password to Keychain (one-time)
security add-generic-password -s jdbc-cli/mydb -a myuser -w mysecret

# Open a connection pool
jdbc-cli open mydb \
  --url "jdbc:mysql://localhost:3306/myschema" \
  --user myuser \
  --password-keychain jdbc-cli/mydb

# Run a query (TSV with header by default)
jdbc-cli query mydb "SELECT id, name FROM users LIMIT 5"

# Typed JSON output
jdbc-cli query mydb --json "SELECT * FROM orders WHERE id = 1"

# Explore schema
jdbc-cli schema mydb            # list tables
jdbc-cli describe mydb users    # columns for a table

# Write
jdbc-cli exec mydb "UPDATE users SET active = 1 WHERE id = 42"

# Transaction
jdbc-cli begin mydb
jdbc-cli exec  mydb "INSERT INTO events (type) VALUES ('login')"
jdbc-cli commit mydb            # or: jdbc-cli rollback mydb

# Batch (NDJSON — one op per line, results streamed back as NDJSON)
printf '{"op":"query","sql":"SELECT 1"}\n{"op":"query","sql":"SELECT 2"}\n' \
  | jdbc-cli batch mydb

# Done — release the pool
jdbc-cli close mydb
```

## Credentials

| Method | How |
|--------|-----|
| macOS Keychain | `security add-generic-password -s jdbc-cli/ALIAS -a USER -w PASSWORD` then pass `--password-keychain jdbc-cli/ALIAS` |
| 1Password | Wrap invocation: `op run --env-file=.env -- jdbc-cli open …` with `JDBC_PASSWORD` in the env file, then pass `--password-env JDBC_PASSWORD` |

Never pass the password via `--password` — it is visible in `ps`.

## Reference

- [`docs/spec.md`](docs/spec.md) — full specification
- [`docs/plan.md`](docs/plan.md) — implementation plan
- [`docs/credentials.md`](docs/credentials.md) — credential setup in depth
- [`skill/SKILL.md`](skill/SKILL.md) — AI agent skill reference
