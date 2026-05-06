package com.scriptease.jdbccli.daemon

import com.zaxxer.hikari.HikariConfig
import com.zaxxer.hikari.HikariDataSource
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

object Pools {
    private val lock = ReentrantLock()
    private val pools = mutableMapOf<String, HikariDataSource>()

    fun list(): List<String> = lock.withLock { pools.keys.toList() }

    fun open(alias: String, jdbcUrl: String, user: String, password: String) = lock.withLock {
        if (pools.containsKey(alias)) error("alias '$alias' already open")
        val cfg = HikariConfig().apply {
            this.jdbcUrl = jdbcUrl
            if (user.isNotEmpty()) username = user
            if (password.isNotEmpty()) this.password = password
            maximumPoolSize = 5
        }
        pools[alias] = HikariDataSource(cfg)
    }

    fun close(alias: String) = lock.withLock {
        val ds = pools.remove(alias) ?: error("alias '$alias' not found")
        ds.close()
    }

    fun closeAll() = lock.withLock {
        pools.values.forEach { it.close() }
        pools.clear()
    }
}
