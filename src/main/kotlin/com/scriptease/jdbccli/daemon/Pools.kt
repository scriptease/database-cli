package com.scriptease.jdbccli.daemon

import com.zaxxer.hikari.HikariDataSource
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

object Pools {
    private val lock = ReentrantLock()
    private val pools = mutableMapOf<String, HikariDataSource>()

    fun list(): List<String> = lock.withLock { pools.keys.toList() }

    fun closeAll() = lock.withLock {
        pools.values.forEach { it.close() }
        pools.clear()
    }
}
