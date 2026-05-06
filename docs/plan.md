# jdbc-cli — Plan

## Architecture

Single Kotlin program, single fat-jar. `Main.kt` dispatches on `argv[0]`:

```
jdbc-cli daemon            → DaemonMain.run()        # blocks
jdbc-cli <anything else>   → ClientMain.run(argv)    # short-lived
```

Daemon and client share serialization types but never share process state.

```
┌──────────────────────────────────────────────────┐
│ jdbc-cli daemon  (one process, supervised by launchd)
│
│   Jetty (UnixDomainServerConnector)
│       └── /open /close /query /exec /…  (HTTP/1.1)
│              │
│              ▼
│       Pools: Map<alias, HikariDataSource>
│       TxConns: Map<alias, Connection>     (pinned during BEGIN…COMMIT)
│              │
│              ▼
│       JDBC: MySQL | PostgreSQL | SQLite (drivers bundled)
└──────────────────────────────────────────────────┘
                  ▲
                  │ HTTP over ~/.jdbc-cli/sock (0600)
                  │
┌──────────────────────────────────────────────────┐
│ jdbc-cli <subcmd>  (short-lived per call)
│   parses argv → builds JSON request → POSTs over UDS → prints response
└──────────────────────────────────────────────────┘
```

## File layout

```
~/github/jdbc-cli/
├── README.md
├── docs/
│   ├── spec.md
│   └── plan.md
├── autorun/
│   └── JDBC-CLI-01.md             # playbook
├── skill/
│   └── SKILL.md                   # agent-facing usage skill
├── settings.gradle.kts
├── build.gradle.kts               # Kotlin JVM, Shadow plugin for fat-jar
├── gradle/wrapper/…
├── gradlew
├── src/main/kotlin/com/florian/jdbccli/
│   ├── Main.kt                    # argv dispatch
│   ├── daemon/
│   │   ├── DaemonMain.kt          # Jetty bootstrap, signal handling
│   │   ├── Routes.kt              # /open /close /query /…
│   │   ├── Pools.kt               # alias → HikariDataSource + tx map
│   │   ├── ResultSets.kt          # ResultSet → typed JSON
│   │   └── Keychain.kt            # `security find-generic-password -w`
│   └── client/
│       ├── ClientMain.kt          # argv parse, http call, pretty-print
│       ├── ArgParse.kt            # `--alias`, `--json`, positional SQL
│       └── HttpClient.kt          # JDK HttpClient + Unix-socket SocketChannel
├── bin/
│   └── jdbc-cli                   # wrapper template (installed to PATH)
├── launchd/
│   └── com.scriptease.jdbc-cli.plist # template
└── scripts/
    └── install.sh                 # build, place wrapper, bootstrap launchd
```

## Build

`build.gradle.kts`:

- Kotlin 2.x JVM target 21
- Plugins: `kotlin("jvm")`, `id("com.github.johnrengelman.shadow")`
- Dependencies:
  - `org.eclipse.jetty:jetty-server:12.0.x`
  - `org.eclipse.jetty:jetty-unixdomain-server:12.0.x`
  - `com.zaxxer:HikariCP:5.1.0`
  - `org.jetbrains.kotlinx:kotlinx-serialization-json:1.7.3`
  - `com.mysql:mysql-connector-j:9.4.0`
  - `org.postgresql:postgresql:42.7.4`
  - `org.xerial:sqlite-jdbc:3.46.1.3`
  - `org.slf4j:slf4j-simple:2.0.x` (Hikari/Jetty logging)
- Shadow output: `build/libs/jdbc-cli-all.jar`
- Manifest `Main-Class: com.scriptease.jdbccli.MainKt`

## Wrapper

Mirrors the Maestro pattern. Installed to `/opt/homebrew/bin/jdbc-cli`:

```bash
#!/bin/bash
exec /opt/homebrew/opt/openjdk@21/bin/java \
     -jar /Users/florian/github/jdbc-cli/build/libs/jdbc-cli-all.jar "$@"
```

Created by `scripts/install.sh` via `tee` + `chmod +x` (no sudo on
`/opt/homebrew/bin` for Homebrew users).

## Client transport

JDK 17+ has `UnixDomainSocketAddress` and `SocketChannel.open(UNIX)`,
but `java.net.http.HttpClient` does **not** speak HTTP over Unix
sockets. Two options for the client:

1. Hand-roll a minimal HTTP/1.1 request writer + response parser on top
   of the `SocketChannel`. ~60 LOC. Zero deps. **Pick this** — it's a
   short-lived process; we don't need streaming or chunked.
2. Add Apache HttpClient 5 with a custom `ConnectionSocketFactory` for
   Unix sockets. Heavier, more dependency surface.

## Server transport

Jetty 12 supports Unix sockets via:

```kotlin
val server = Server()
val connector = UnixDomainServerConnector(server).apply {
    unixDomainPath = Path.of(System.getenv("HOME"), ".jdbc-cli/sock")
}
server.addConnector(connector)
```

After `server.start()`, `chmod 0600` the socket file.

## Typed JSON for ResultSet

In `ResultSets.kt`, switch on `ResultSetMetaData.getColumnType(i)`:

| `java.sql.Types`                    | JSON                       |
| ----------------------------------- | -------------------------- |
| `BIT`, `BOOLEAN`                    | `true`/`false`             |
| `TINYINT`…`BIGINT`                  | JSON number                |
| `REAL`, `FLOAT`, `DOUBLE`           | JSON number                |
| `NUMERIC`, `DECIMAL`                | string (preserve precision)|
| `CHAR`, `VARCHAR`, `LONGVARCHAR`    | string                     |
| `DATE`, `TIME`, `TIMESTAMP`, `…_WITH_TIMEZONE` | ISO-8601 string |
| `BINARY`, `VARBINARY`, `BLOB`       | base64 string              |
| `NULL` / `wasNull()`                | `null`                     |
| Anything else                       | `getString()` fallback     |

## Phases (each independently shippable)

| #   | Scope                                                        | Acceptance                                          |
| --- | ------------------------------------------------------------ | --------------------------------------------------- |
| 1   | Gradle skeleton, fat-jar, `Main` dispatch, `daemon`+`ping`   | `jdbc-cli ping` returns `ok`                        |
| 2   | `open`/`close`/`list`, Hikari pool, MySQL+Postgres+SQLite    | open SQLite `:memory:`, list, close (acceptance #2) |
| 3   | `query`/`exec`/`schema`/`describe` + typed JSON              | acceptance #3 (MySQL round-trip)                    |
| 4   | Transactions (`begin`/`commit`/`rollback` with pinned conn)  | acceptance #5                                       |
| 5   | Keychain creds + `op run` doc + `--password-stdin`           | acceptances #7, #8                                  |
| 6   | launchd plist + `install.sh` wrapper                         | acceptances #1, #9, #10                             |
| 7   | `batch` (NDJSON), concurrency hardening (virtual threads)    | acceptance #6                                       |
| 8   | `SKILL.md` polish + README finalize                          | manual review                                       |

Phases 1–7 are mergeable to `main` independently. Phase 8 is doc-only.

## What is **not** in this plan (deferred to v2)

- Coursier-driven lazy driver loading.
- MCP front-end on the same daemon.
- Result paging / cursor support.
- Audit log of executed statements.
- Per-query timeout / cancellation.
- Linux/Windows support.
- TLS-to-DB cert pinning helpers.

## Risks / open watch-items

1. **Jetty 12 fat-jar size** — Jetty + 3 JDBC drivers ≈ 12–15 MB. Acceptable; flag if it balloons.
2. **Shadow plugin + Kotlin 2.x compatibility** — occasionally lags. Fallback: plain `Jar` task with `from(configurations.runtimeClasspath.map { … })`.
3. **`UnixDomainServerConnector` on macOS** — known to work, but socket path length limit (~104 chars). `~/.jdbc-cli/sock` is well under it.
4. **JDBC driver auto-registration in fat-jar** — `META-INF/services/java.sql.Driver` files from each driver must be merged by Shadow (`mergeServiceFiles()`). Easy to forget; if forgotten, `DriverManager.getConnection` fails with "no suitable driver".
5. **`security` CLI prompts for Keychain access** — first call may show a GUI prompt. Document in SKILL.md.
