# database-cli — Plan

## Architecture

Single Go program, single self-contained binary. `main.go` dispatches on
`argv[0]`:

```
database-cli daemon            → daemon.Run()      # blocks
database-cli <anything else>   → cli.Run(argv)     # short-lived
```

Daemon and client share protocol types but never share process state.

```
┌────────────────────────────────────────────────────────────┐
│ database-cli daemon  (one process, supervised by launchd)      │
│                                                            │
│   net/http over Unix domain socket                         │
│       └── /open /close /query /exec /…                     │
│              │                                             │
│              ▼                                             │
│       Store: Map<alias, *sql.DB>                           │
│       Tx:    Map<alias, *sql.Tx>   (pinned during          │
│                                     BEGIN…COMMIT)          │
│              │                                             │
│              ▼                                             │
│       database/sql + native Go drivers                     │
│       MySQL | PostgreSQL | SQLite                          │
└────────────────────────────────────────────────────────────┘
                  ▲
                  │ HTTP over ~/.database-cli/sock (0600)
                  │
┌────────────────────────────────────────────────────────────┐
│ database-cli <subcmd>  (short-lived per call)                  │
│   parses argv → builds JSON request → POSTs over UDS       │
│   → prints response                                        │
└────────────────────────────────────────────────────────────┘
```

## Go-port note

The goal difference on this branch is internal, not external:

- keep the CLI/daemon contract effectively the same
- replace the JVM fat-jar with a self-contained Go binary
- remove the runtime requirement for Java, JDBC jars, or external DB clients

## File layout

```
~/github/database-cli/
├── README.md
├── docs/
│   ├── spec.md
│   └── plan.md
├── skill/
│   └── SKILL.md
├── go.mod
├── go.sum
├── main.go
├── internal/
│   ├── cli/
│   │   └── cli.go
│   ├── client/
│   │   └── http.go
│   ├── daemon/
│   │   ├── jdbc.go
│   │   ├── keychain.go
│   │   ├── render.go
│   │   ├── server.go
│   │   └── store.go
│   ├── jsonerror/
│   │   └── jsonerror.go
│   └── protocol/
│       └── protocol.go
├── launchd/
│   └── com.scriptease.database-cli.plist
└── scripts/
    └── install.sh
```

## Build

`go.mod`:

- Go 1.24
- Dependencies:
  - `github.com/go-sql-driver/mysql`
  - `github.com/jackc/pgx/v5/stdlib`
  - `modernc.org/sqlite`
- Build output: `build/database-cli`
- Installer build flags: `CGO_ENABLED=0`, `-trimpath`, `-ldflags="-s -w"`

## Wrapper

Mirrors the old install shape. Installed to `/opt/homebrew/bin/database-cli`:

```bash
#!/usr/bin/env bash
set -euo pipefail
exec "$HOME/.local/share/database-cli/database-cli" "$@"
```

Created by `scripts/install.sh`, which also installs the launchd plist and
boots the daemon.

## Client transport

The short-lived CLI process uses `net/http` with a custom `Transport` that
dials the Unix socket directly:

```go
transport := &http.Transport{
    DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
        return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
    },
}
```

That keeps the client tiny and dependency-free while preserving the same
daemon architecture.

## Server transport

The daemon uses:

```go
listener, _ := net.Listen("unix", socketPath)
server := &http.Server{Handler: routes(store)}
server.Serve(listener)
```

After binding, the socket file is chmodded to `0600`.

## Typed JSON for rows

In `render.go`, switch on the scanned Go value and the reported DB type:

| DB / Go shape                          | JSON                        |
| -------------------------------------- | --------------------------- |
| booleans                               | `true` / `false`            |
| integer types                          | JSON number                 |
| float types                            | JSON number                 |
| `NUMERIC`, `DECIMAL`                   | string (preserve precision) |
| text types                             | string                      |
| `time.Time`                            | ISO-8601 string             |
| binary types                           | base64 string               |
| `NULL`                                 | `null`                      |
| anything else                          | string fallback             |

## Phases (each independently shippable)

| #   | Scope                                                        | Acceptance                                          |
| --- | ------------------------------------------------------------ | --------------------------------------------------- |
| 1   | Go module skeleton, `main` dispatch, `daemon` + `ping`       | `database-cli ping` returns `ok`                        |
| 2   | `open` / `close` / `list`, native drivers, alias store       | open SQLite `:memory:`, list, close                 |
| 3   | `query` / `exec` / `schema` / `describe` + typed JSON        | query + write round-trip works                      |
| 4   | Transactions (`begin` / `commit` / `rollback`)               | rollback leaves row count unchanged                 |
| 5   | Keychain creds + `--password-stdin`                          | both auth paths work                                |
| 6   | launchd plist + install wrapper                              | install + kickstart + ping work                     |
| 7   | `batch`, compatibility cleanup, docs restore                 | batch + old CLI shape work                          |

## What is **not** in this plan (deferred to v2)

- Lazy driver loading beyond the built-in engines.
- MCP front-end on the same daemon.
- Result paging / cursor support.
- Audit log of executed statements.
- Per-query timeout / cancellation.
- Linux/Windows packaging.
- TLS-to-DB cert pinning helpers.

## Risks / open watch-items

1. **Unix socket path length on macOS** — the limit is still tight. `~/.database-cli/sock` is safe; deep temp directories are not.
2. **JDBC URL compatibility translation** — the outside interface stays `--jdbc-url`, so MySQL/PostgreSQL/SQLite parsing must remain stable.
3. **SQLite memory semantics** — `:memory:` needs a single pooled connection or state disappears across commands.
4. **Metadata differences across drivers** — `schema` and `describe` are dialect-specific, not one generic JDBC metadata path anymore.
5. **Keychain prompts** — the first macOS Keychain lookup may still trigger a GUI approval prompt.
