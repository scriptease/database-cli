# database-cli spec

## Goal

Keep the original CLI contract while replacing the Kotlin/JVM implementation with Go.

## Hard requirements

1. Keep the command surface compatible:
   - `ping`, `open`, `close`, `list`, `query`, `exec`, `schema`, `describe`, `begin`, `commit`, `rollback`, `batch`
   - same flag names, especially `--jdbc-url`
   - same old-style positional SQL usage for `query` and `exec`
2. Keep the daemon/client split over a Unix domain socket at `~/.database-cli/sock`.
3. Keep named aliases with one optional active transaction per alias.
4. Keep output contracts:
   - `query` default: TSV with header row
   - `query --json`: typed JSON rows
   - `exec`: `{"rowsAffected":N}`
   - mutation commands: `{"ok":true}`
   - `batch`: continue after per-line errors and honor per-op `"json":true` on query ops
5. Resulting binary must be self-contained:
   - no JVM
   - no JDBC jars
   - no external DB client binaries
   - no CGO dependency for SQLite

## Implementation

### Binary layout

One Go binary dispatches between:

- client mode: normal CLI usage
- daemon mode: internal background service started by launchd

### Transport

- HTTP over a Unix domain socket
- plain JSON request bodies
- plain JSON or TSV response bodies

### Database support

`--jdbc-url` is translated to native Go drivers:

- MySQL -> `github.com/go-sql-driver/mysql`
- PostgreSQL -> `github.com/jackc/pgx/v5/stdlib`
- SQLite -> `modernc.org/sqlite`

SQLite is pure Go so the binary stays self-contained.

### Compatibility notes

- `ping` stays as the daemon liveness check.
- `query` / `exec` accept the original positional SQL form, while `--sql` is still accepted as a compatibility fallback.
- `jdbc:sqlite::memory:` is pinned to a single SQLite connection so memory databases behave the same across commands.
- `schema` and `describe` use dialect-specific metadata queries.
- `read-only` is enforced in the daemon for `exec`.

### Packaging

- build with `CGO_ENABLED=0`
- install a launchd agent that runs `database-cli daemon`
- install a thin shell wrapper at `/opt/homebrew/bin/database-cli`

## Acceptance checks

1. Open MySQL, PostgreSQL, and SQLite aliases using the existing flag contract.
2. Run `ping`, then run queries in TSV and JSON mode.
3. Run `begin` / `commit` / `rollback` against an alias.
4. Reject `exec` on a read-only alias.
5. Confirm batch query ops honor their JSON flag.
6. Confirm the built binary runs without Java or external database client software.
