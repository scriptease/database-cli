package com.scriptease.jdbccli.client

object ClientMain {
    fun run(args: Array<String>) {
        if (args.isEmpty()) {
            System.err.println("""{"error":"no subcommand given"}""")
            System.exit(1)
        }
        when (args[0]) {
            "ping" -> println(HttpClient.get("/ping"))
            "list" -> println(HttpClient.get("/list"))
            "open" -> {
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                val jdbcUrl = p["jdbc-url"] ?: die("--jdbc-url required")
                val user = p["user"] ?: ""
                val password = if (p.containsKey("password-stdin")) readLine() ?: "" else ""
                val body = buildJsonOpen(alias, jdbcUrl, user, password)
                println(HttpClient.post("/open", body))
            }
            "close" -> {
                val p = parseFlags(args, 1)
                val alias = p["alias"] ?: die("--alias required")
                println(HttpClient.post("/close", """{"alias":"$alias"}"""))
            }
            else -> {
                System.err.println("""{"error":"unknown subcommand: ${args[0]}"}""")
                System.exit(1)
            }
        }
    }

    private fun parseFlags(args: Array<String>, start: Int): Map<String, String> {
        val result = mutableMapOf<String, String>()
        var i = start
        while (i < args.size) {
            val arg = args[i]
            if (arg.startsWith("--")) {
                val key = arg.removePrefix("--")
                if (i + 1 < args.size && !args[i + 1].startsWith("--")) {
                    result[key] = args[i + 1]
                    i += 2
                } else {
                    result[key] = ""
                    i++
                }
            } else {
                i++
            }
        }
        return result
    }

    private fun buildJsonOpen(alias: String, jdbcUrl: String, user: String, password: String): String {
        fun esc(s: String) = s.replace("\\", "\\\\").replace("\"", "\\\"")
        return """{"alias":"${esc(alias)}","jdbcUrl":"${esc(jdbcUrl)}","user":"${esc(user)}","password":"${esc(password)}"}"""
    }

    private fun die(msg: String): Nothing {
        System.err.println("""{"error":"$msg"}""")
        System.exit(1)
        error("unreachable")
    }
}
