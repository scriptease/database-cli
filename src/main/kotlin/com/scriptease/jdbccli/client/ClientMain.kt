package com.scriptease.jdbccli.client

object ClientMain {
    fun run(args: Array<String>) {
        if (args.isEmpty() || args[0] == "--help" || args[0] == "-h") {
            printTopHelp(); return
        }

        when (args[0]) {
            "ping" -> {
                if (args.any { it == "--help" }) { printHelp("ping"); return }
                println(HttpClient.get("/ping"))
            }
            "list" -> {
                if (args.any { it == "--help" }) { printHelp("list"); return }
                println(HttpClient.get("/list"))
            }
            "open" -> {
                if (args.any { it == "--help" }) { printHelp("open"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                val jdbcUrl = p["jdbc-url"] ?: die("--jdbc-url required")
                val user = p["user"] ?: ""
                val keychainRef = p["password-keychain"]
                val readOnly = p.containsKey("read-only")
                val password = when {
                    keychainRef != null -> ""
                    p.containsKey("password-stdin") -> readLine() ?: ""
                    else -> ""
                }
                val body = buildJsonOpen(alias, jdbcUrl, user, password, keychainRef, readOnly)
                println(HttpClient.post("/open", body))
            }
            "close" -> {
                if (args.any { it == "--help" }) { printHelp("close"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                println(HttpClient.post("/close", """{"alias":"$alias"}"""))
            }
            "query" -> {
                if (args.any { it == "--help" }) { printHelp("query"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                val asJson = p.containsKey("json")
                val sql = parsePositional(args, 1) ?: die("SQL argument required")
                fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                val body = """{"alias":"${esc(alias)}","sql":"${esc(sql)}","json":$asJson}"""
                println(HttpClient.post("/query", body))
            }
            "exec" -> {
                if (args.any { it == "--help" }) { printHelp("exec"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                val sql = parsePositional(args, 1) ?: die("SQL argument required")
                fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                val body = """{"alias":"${esc(alias)}","sql":"${esc(sql)}"}"""
                println(HttpClient.post("/exec", body))
            }
            "schema" -> {
                if (args.any { it == "--help" }) { printHelp("schema"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                println(HttpClient.post("/schema", """{"alias":"${esc(alias)}"}"""))
            }
            "describe" -> {
                if (args.any { it == "--help" }) { printHelp("describe"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                val table = p["table"] ?: die("--table required")
                fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                println(HttpClient.post("/describe", """{"alias":"${esc(alias)}","table":"${esc(table)}"}"""))
            }
            "begin" -> {
                if (args.any { it == "--help" }) { printHelp("begin"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                println(HttpClient.post("/begin", """{"alias":"${esc(alias)}"}"""))
            }
            "commit" -> {
                if (args.any { it == "--help" }) { printHelp("commit"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                println(HttpClient.post("/commit", """{"alias":"${esc(alias)}"}"""))
            }
            "rollback" -> {
                if (args.any { it == "--help" }) { printHelp("rollback"); return }
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                println(HttpClient.post("/rollback", """{"alias":"${esc(alias)}"}"""))
            }
            "batch" -> {
                if (args.any { it == "--help" }) { printHelp("batch"); return }
                val p = parseFlags(args, 1)
                val defaultAlias = p["alias"]
                val lines = generateSequence(::readLine).toList()
                val body = if (defaultAlias == null) {
                    lines.joinToString("\n")
                } else {
                    fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
                    lines.joinToString("\n") { line ->
                        val trimmed = line.trim()
                        if (trimmed.startsWith("{") && !trimmed.contains(""""alias"""")) {
                            """{"alias":"${esc(defaultAlias)}",${ trimmed.removePrefix("{") }"""
                        } else trimmed
                    }
                }
                println(HttpClient.post("/batch", body))
            }
            else -> {
                System.err.println("""{"error":"unknown subcommand: ${args[0]}"}""")
                System.exit(1)
            }
        }
    }

    private fun printTopHelp() {
        println("""
jdbc-cli <subcommand> [flags] [args]

Subcommands:
  ping                   Check if the daemon is running
  list                   List open connection aliases
  open                   Open a new JDBC connection
  close                  Close an open connection
  query                  Run a SELECT query
  exec                   Run an INSERT/UPDATE/DELETE statement
  schema                 List tables in the database
  describe               Describe columns of a table
  begin                  Begin a transaction
  commit                 Commit a transaction
  rollback               Roll back a transaction
  batch                  Execute multiple operations from stdin (NDJSON)

Global flags:
  --help                 Show this help

Run 'jdbc-cli <subcommand> --help' for subcommand-specific usage.
        """.trimIndent())
    }

    private fun printHelp(cmd: String) {
        val text = when (cmd) {
            "ping" -> """
ping  Check if the daemon is running.

Usage:
  jdbc-cli ping
            """.trimIndent()

            "list" -> """
list  List all open connection aliases.

Usage:
  jdbc-cli list
            """.trimIndent()

            "open" -> """
open  Open a new JDBC connection and register it under an alias.

Usage:
  jdbc-cli open --alias <name> --jdbc-url <url> [auth flags]

Flags:
  --alias <name>               Alias to register this connection under (required)
  --jdbc-url <url>             JDBC URL, e.g. jdbc:mysql://localhost:3306/mydb (required)
  --user <user>                Database username
  --password-stdin             Read password from stdin
  --password-keychain <ref>    Look up password from macOS Keychain (service/account)
  --read-only                  Block write operations (exec, begin, commit, rollback) on this alias
            """.trimIndent()

            "close" -> """
close  Close an open connection.

Usage:
  jdbc-cli close --alias <name>

Flags:
  --alias <name>    Connection alias to close (required)
            """.trimIndent()

            "query" -> """
query  Run a SELECT query and print results.

Usage:
  jdbc-cli query --alias <name> [--json] '<SQL>'

Flags:
  --alias <name>    Connection alias (required)
  --json            Output results as JSON array instead of TSV

Example:
  jdbc-cli query --alias mydb 'SELECT * FROM users LIMIT 10'
  jdbc-cli query --alias mydb --json 'SELECT id, name FROM users'
            """.trimIndent()

            "exec" -> """
exec  Run a write SQL statement (INSERT, UPDATE, DELETE, DDL).

Usage:
  jdbc-cli exec --alias <name> '<SQL>'

Flags:
  --alias <name>    Connection alias (required)

Output:
  {"rowsAffected": <n>}

Note: blocked if the alias was opened with --read-only.
            """.trimIndent()

            "schema" -> """
schema  List all tables in the database.

Usage:
  jdbc-cli schema --alias <name>

Flags:
  --alias <name>    Connection alias (required)
            """.trimIndent()

            "describe" -> """
describe  Show column definitions for a table.

Usage:
  jdbc-cli describe --alias <name> --table <table>

Flags:
  --alias <name>     Connection alias (required)
  --table <table>    Table name (required)

Output:
  JSON array of {name, type, size, nullable} objects.
            """.trimIndent()

            "begin" -> """
begin  Begin a transaction on the connection.

Usage:
  jdbc-cli begin --alias <name>

Flags:
  --alias <name>    Connection alias (required)

Note: blocked if the alias was opened with --read-only.
            """.trimIndent()

            "commit" -> """
commit  Commit the current transaction.

Usage:
  jdbc-cli commit --alias <name>

Flags:
  --alias <name>    Connection alias (required)

Note: blocked if the alias was opened with --read-only.
            """.trimIndent()

            "rollback" -> """
rollback  Roll back the current transaction.

Usage:
  jdbc-cli rollback --alias <name>

Flags:
  --alias <name>    Connection alias (required)

Note: blocked if the alias was opened with --read-only.
            """.trimIndent()

            "batch" -> """
batch  Execute multiple operations from stdin in NDJSON format.

Usage:
  echo '<ndjson>' | jdbc-cli batch [--alias <default-alias>]

Flags:
  --alias <name>    Default alias injected into ops that omit it (optional)

Each stdin line is a JSON object with an "op" field:
  {"op":"query","alias":"mydb","sql":"SELECT 1","json":false}
  {"op":"exec","alias":"mydb","sql":"INSERT INTO t VALUES (1)"}
  {"op":"begin","alias":"mydb"}
  {"op":"commit","alias":"mydb"}
  {"op":"rollback","alias":"mydb"}

Output is one NDJSON result line per input line.

Note: blocked if the alias was opened with --read-only.
            """.trimIndent()

            else -> "No help available for '$cmd'."
        }
        println(text)
    }

    private data class ParsedArgs(val flags: Map<String, String>, val positionals: List<String>)

    private val BOOL_FLAGS = setOf("json", "password-stdin", "read-only", "help")

    private fun parse(args: Array<String>, start: Int): ParsedArgs {
        val flags = mutableMapOf<String, String>()
        val positionals = mutableListOf<String>()
        var i = start
        while (i < args.size) {
            val arg = args[i]
            if (arg.startsWith("--")) {
                val key = arg.removePrefix("--")
                if (key in BOOL_FLAGS || i + 1 >= args.size || args[i + 1].startsWith("--")) {
                    flags[key] = ""; i++
                } else {
                    flags[key] = args[i + 1]; i += 2
                }
            } else {
                positionals += arg; i++
            }
        }
        return ParsedArgs(flags, positionals)
    }

    private fun parseFlags(args: Array<String>, start: Int) = parse(args, start).flags
    private fun parsePositional(args: Array<String>, start: Int) = parse(args, start).positionals.firstOrNull()

    private fun buildJsonOpen(alias: String, jdbcUrl: String, user: String, password: String, keychainRef: String? = null, readOnly: Boolean = false): String {
        fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
        val kc = if (keychainRef != null) ""","passwordKeychain":"${esc(keychainRef)}"""" else ""
        val ro = if (readOnly) ""","readOnly":true""" else ""
        return """{"alias":"${esc(alias)}","jdbcUrl":"${esc(jdbcUrl)}","user":"${esc(user)}","password":"${esc(password)}"$kc$ro}"""
    }

    private fun die(msg: String): Nothing {
        System.err.println("""{"error":"$msg"}""")
        System.exit(1)
        error("unreachable")
    }
}
