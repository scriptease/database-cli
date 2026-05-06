package com.scriptease.jdbccli.client

object ClientMain {
    fun run(args: Array<String>) {
        if (args.isEmpty()) {
            System.err.println("""{"error":"no subcommand given"}""")
            System.exit(1)
        }
        when (args[0]) {
            "ping" -> {
                val resp = HttpClient.get("/ping")
                println(resp)
            }
            else -> {
                System.err.println("""{"error":"unknown subcommand: ${args[0]}"}""")
                System.exit(1)
            }
        }
    }
}
