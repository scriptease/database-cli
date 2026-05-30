---
name: jdbc-cli
description: "Query and modify SQL databases with persistent sessions backed by a resident daemon. Use for ad-hoc queries, schema inspection, transactions across multiple agent steps, and repeat-shape query loops where connect cold-start would dominate. Supported engines: MySQL, PostgreSQL, SQLite."
---

# jdbc-cli

A CLI for talking to SQL databases through a long-lived local daemon.
The skill stays functionally the same as before; the only goal difference
on this branch is internal: the implementation is now a self-contained Go
binary instead of a JVM/JDBC fat-jar.

The daemon stays running under launchd; each `jdbc-cli <subcmd>` is a
short-lived call that hits the daemon over a Unix socket. Connection
pools are kept warm **per alias** between calls.

## Help

```bash
jdbc-cli --help                  # list all subcommands and global flags
jdbc-cli <subcommand> --help     # flags and usage for a specific subcommand
```

## When to use

- You need to run more than one SQL query in a row against the same DB.
- You want a transaction that spans multiple agent steps.
- You want typed JSON output for downstream tool consumption.

## When **not** to use

- One-off `SELECT` against a DB you'll never touch again — `mysql`/`psql`/`sqlite3` are simpler.
- Any DB outside MySQL / PostgreSQL / SQLite (until broader driver support lands).
- You need server-side cursors, paging, or audit logs (deferred to v2).

## Launch

The daemon is supervised by launchd; on a working install it's already
running. To verify:

```bash
jdbc-cli ping        # → ok
```

If `ping` errors with "daemon not running":

```bash
launchctl kickstart -k gui/$(id -u)/com.scriptease.jdbc-cli
sleep 1
jdbc-cli ping
```

If the wrapper itself is missing, re-run the installer from the repo root:

```bash
bash scripts/install.sh
```

## Lifecycle

```bash
# Open (always pair with close)
jdbc-cli open  --alias prod-shop \
               --jdbc-url jdbc:mysql://localhost:3306/shop \
               --user root \
               --password-stdin                              # OR --password-keychain

# Open read-only — exec/begin/commit/rollback are rejected by the daemon for this alias
jdbc-cli open  --alias prod-shop-ro \
               --jdbc-url jdbc:mysql://localhost:3306/shop \
               --user root \
               --password-keychain jdbc-cli/prod-shop \
               --read-only

# Read
jdbc-cli query --alias prod-shop "SELECT id, name FROM users LIMIT 10"
jdbc-cli query --alias prod-shop --json "SELECT id, name FROM users LIMIT 10"

# Write
jdbc-cli exec  --alias prod-shop "UPDATE users SET active=1 WHERE id=42"

# Inspect
jdbc-cli schema   --alias prod-shop                          # all tables
jdbc-cli describe --alias prod-shop --table users            # columns

# Close (releases the pool)
jdbc-cli close --alias prod-shop

# State
jdbc-cli list                                                # active aliases
```

## Transactions

One alias = one transaction state. Run all steps under the same alias.

```bash
jdbc-cli begin    --alias prod-shop
jdbc-cli exec     --alias prod-shop "UPDATE users SET active=0 WHERE id=42"
jdbc-cli query    --alias prod-shop "SELECT active FROM users WHERE id=42"
# … decide …
jdbc-cli commit   --alias prod-shop      # or: rollback
```

The pinned connection is dedicated to this alias from `begin` until
`commit` / `rollback`. Other aliases are unaffected.

## Credentials — never put passwords on argv

### `op run` (1Password)

```bash
DB_PASSWORD='op://Caperwhite/prod-shop/password' \
op run -- bash -c '
  printf "%s" "$DB_PASSWORD" | jdbc-cli open \
    --alias prod-shop \
    --jdbc-url jdbc:mysql://localhost:3306/shop \
    --user root \
    --password-stdin
'
```

### macOS Keychain

One-time:

```bash
security add-generic-password -s jdbc-cli/prod-shop -a root -w 'thepassword'
```

Each session:

```bash
jdbc-cli open --alias prod-shop \
              --jdbc-url jdbc:mysql://localhost:3306/shop \
              --user root \
              --password-keychain jdbc-cli/prod-shop
```

The daemon shells out to `security find-generic-password -w`. The first
call after a reboot may show a Keychain GUI prompt; allow once.

## Output

- Default: TSV with a header row (`column<TAB>column<TAB>…`, then rows).
- `--json`: array of objects with **typed** values:
  - integers → JSON numbers
  - booleans → `true` / `false`
  - NULL → `null`
  - DATE / TIME / TIMESTAMP → ISO-8601 strings
  - BLOB → base64 string
  - NUMERIC / DECIMAL → string (precision preserved)

## Batch mode

```bash
cat <<'EOF' | jdbc-cli batch --alias prod-shop
{"op":"query","sql":"SELECT count(*) FROM users"}
{"op":"query","sql":"SELECT count(*) FROM orders","json":true}
{"op":"exec","sql":"UPDATE users SET active=1 WHERE id=42"}
EOF
```

Returns NDJSON — one result line per input op, in order. A failed op emits
`{"error":"…"}` at its position; the rest of the batch continues.

Supported ops in batch: `query`, `exec`, `begin`, `commit`, `rollback`.
(`schema` and `describe` are single-op commands only.)

`--alias` on the `batch` command injects the alias into every line that lacks
one, so per-line `"alias"` fields are optional when all ops share the same
connection.

## Common pitfalls

| Pitfall                                  | Correct approach                                           |
| ---------------------------------------- | ---------------------------------------------------------- |
| `--password 'secret'` on argv            | Visible in `ps`. Use `--password-stdin` or `--password-keychain`. |
| `exec` on a read-only alias              | Daemon returns `{"error":"alias is read-only: …"}`. Open with `--read-only` intentionally to enforce safety; don't use it if you need writes. |
| Forgetting `close`                       | Keeps the alias and pool open. Always pair `open` / `close`. |
| Transaction across two terminals         | One alias = one tx state. Use the same alias for all steps.|
| Wrapper missing after install            | Re-run `bash scripts/install.sh` to rebuild and reinstall. |
| Aliases gone after a reboot              | Expected — aliases are not persisted. Re-`open`.           |
| `query` returns string `"42"` not number | Add `--json` for typed values; default TSV is text.        |

## Daemon-restart symptoms

If launchd respawns the daemon mid-session:

- All aliases are gone (`list` returns `[]`).
- Any open transaction is aborted server-side.
- `jdbc-cli ping` returns `ok` again immediately.

Recover by re-`open`ing the aliases you need.

## Logs

```bash
~/Library/Logs/jdbc-cli/daemon.log
~/Library/Logs/jdbc-cli/daemon.err.log
```

Tail when debugging:

```bash
tail -f ~/Library/Logs/jdbc-cli/daemon.log ~/Library/Logs/jdbc-cli/daemon.err.log
```
