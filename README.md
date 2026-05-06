# jdbc-cli

CLI for talking to SQL databases over JDBC, backed by a resident daemon so
the JVM and connection pools stay warm between calls. Same shape as
`officecli`'s `open … close` model.

- Daemon: Kotlin + Jetty (HTTP over Unix domain socket) + HikariCP
- Drivers bundled in v1: MySQL, PostgreSQL, SQLite
- Credentials via `op run` or macOS Keychain — never via argv
- Single fat-jar; `jdbc-cli` shell wrapper invokes `java -jar …`

See [`docs/spec.md`](docs/spec.md) and [`docs/plan.md`](docs/plan.md).
Skill for AI agents: [`skill/SKILL.md`](skill/SKILL.md).
