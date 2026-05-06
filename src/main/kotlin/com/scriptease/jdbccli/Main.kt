package com.scriptease.jdbccli

import com.scriptease.jdbccli.client.ClientMain
import com.scriptease.jdbccli.daemon.DaemonMain

fun main(args: Array<String>) {
    if (args.firstOrNull() == "daemon") {
        DaemonMain.run()
    } else {
        ClientMain.run(args)
    }
}
