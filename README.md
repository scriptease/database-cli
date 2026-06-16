# database-cli

`database-cli` is a macOS SQL CLI with a background daemon that keeps named database connections open across commands.

This `Go` branch keeps the outside interface compatible with the original project:

- same subcommands and flags
- same alias + transaction model
- same Unix-socket daemon/client split
- same default TSV / optional JSON / NDJSON batch outputs
- same `--jdbc-url` flag for connection strings

The implementation is now a **single self-contained Go binary**. It does **not** require Java, JDBC drivers, `mysql`, `psql`, `sqlite3`, or any other runtime/client software to be installed. SQLite support is pure Go, and MySQL/PostgreSQL use native Go drivers. The only external commands involved are macOS built-ins for optional launchd install and optional Keychain lookup.

## Supported URLs

`--jdbc-url` is kept for compatibility even though the implementation is no longer JDBC-based.

- MySQL: `jdbc:mysql://localhost:3306/app`
- PostgreSQL: `jdbc:postgresql://localhost:5432/app`
- SQLite file: `jdbc:sqlite:/tmp/app.db`
- SQLite memory: `jdbc:sqlite::memory:`

## Install

```bash
./scripts/install.sh
```

That script builds the binary with `CGO_ENABLED=0`, installs it to `~/.local/share/database-cli/database-cli`, writes a launchd agent, and drops a wrapper at `/opt/homebrew/bin/database-cli`.

You only need Go to build from source. Once built, the resulting `database-cli` binary is standalone and can be copied to another compatible macOS machine without installing Java or database client libraries.

## Commands

```text
database-cli ping
database-cli open --alias A --jdbc-url URL [--user U] [--password-stdin|--password-keychain S] [--read-only]
database-cli close --alias A
database-cli list
database-cli query --alias A [--json] 'select ...'
database-cli exec --alias A 'update ...'
database-cli schema --alias A
database-cli describe --alias A --table T
database-cli begin --alias A
database-cli commit --alias A
database-cli rollback --alias A
database-cli batch [--alias A]
```

## Examples

```bash
database-cli ping
database-cli open --alias pg --jdbc-url jdbc:postgresql://localhost:5432/app --user app --password-stdin
database-cli query --alias pg 'select now()'
database-cli query --alias pg --json 'select id, email from users order by id limit 10'
database-cli exec --alias pg "update users set active = false where last_login < now() - interval '1 year'"
database-cli schema --alias pg
database-cli describe --alias pg --table users
```

SQLite:

```bash
database-cli open --alias mem --jdbc-url jdbc:sqlite::memory:
database-cli exec --alias mem 'create table t(id integer primary key, name text)'
database-cli exec --alias mem "insert into t(name) values ('Ada')"
database-cli query --alias mem 'select * from t'
```

Transactions:

```bash
database-cli begin --alias pg
database-cli exec --alias pg "insert into audit_log(message) values ('hello')"
database-cli rollback --alias pg
```

## Batch mode

Input is NDJSON on stdin. `query` returns TSV by default or JSON when the line includes `"json":true`; `exec` returns `{"rowsAffected":...}`, and transaction commands return `{"ok":true}`.

```bash
printf '%s\n' \
  '{"op":"begin"}' \
  '{"op":"exec","sql":"create table if not exists t(id integer)"}' \
  '{"op":"exec","sql":"insert into t(id) values (1)"}' \
  '{"op":"query","sql":"select * from t","json":true}' \
  '{"op":"commit"}' \
  | database-cli batch --alias mem
```

## Passwords

`--password-stdin` reads the password from stdin.

`--password-keychain SERVICE` reads it from the macOS Keychain entry stored like this:

```bash
security add-generic-password -a database-cli -s SERVICE -w 'secret'
```

## Runtime files

- Socket: `~/.database-cli/sock`
- launchd logs: `~/Library/Logs/database-cli/daemon.log`
- launchd stderr: `~/Library/Logs/database-cli/daemon.err.log`
