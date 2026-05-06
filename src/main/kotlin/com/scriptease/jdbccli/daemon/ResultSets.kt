package com.scriptease.jdbccli.daemon

import java.sql.ResultSet
import java.sql.Types
import java.util.Base64

object ResultSets {

    fun toJson(rs: ResultSet): String {
        val meta = rs.metaData
        val cols = meta.columnCount
        val rows = mutableListOf<String>()
        while (rs.next()) {
            val fields = (1..cols).map { i ->
                val name = meta.getColumnLabel(i).jsonEsc()
                val type = meta.getColumnType(i)
                "\"$name\":${columnJson(rs, i, type)}"
            }
            rows += "{${fields.joinToString(",")}}"
        }
        return "[${rows.joinToString(",")}]"
    }

    fun toTsv(rs: ResultSet): String {
        val meta = rs.metaData
        val cols = meta.columnCount
        val sb = StringBuilder()
        sb.appendLine((1..cols).joinToString("\t") { meta.getColumnLabel(it) })
        while (rs.next()) {
            sb.appendLine((1..cols).joinToString("\t") { i ->
                val v = rs.getString(i)
                if (rs.wasNull()) "\\N" else v ?: "\\N"
            })
        }
        return sb.toString().trimEnd()
    }

    private fun columnJson(rs: ResultSet, i: Int, type: Int): String = when (type) {
        Types.BIT, Types.BOOLEAN -> {
            val v = rs.getBoolean(i); if (rs.wasNull()) "null" else v.toString()
        }
        Types.TINYINT, Types.SMALLINT, Types.INTEGER -> {
            val v = rs.getInt(i); if (rs.wasNull()) "null" else v.toString()
        }
        Types.BIGINT -> {
            val v = rs.getLong(i); if (rs.wasNull()) "null" else v.toString()
        }
        Types.REAL, Types.FLOAT, Types.DOUBLE -> {
            val v = rs.getDouble(i); if (rs.wasNull()) "null" else v.toString()
        }
        Types.NUMERIC, Types.DECIMAL -> {
            val v = rs.getBigDecimal(i)
            if (rs.wasNull()) "null" else "\"${v.toPlainString()}\""
        }
        Types.DATE -> {
            val v = rs.getDate(i)
            if (rs.wasNull()) "null" else "\"${v.toLocalDate()}\""
        }
        Types.TIME, Types.TIME_WITH_TIMEZONE -> {
            val v = rs.getTime(i)
            if (rs.wasNull()) "null" else "\"${v.toLocalTime()}\""
        }
        Types.TIMESTAMP, Types.TIMESTAMP_WITH_TIMEZONE -> {
            val v = rs.getTimestamp(i)
            if (rs.wasNull()) "null" else "\"${v.toLocalDateTime()}\""
        }
        Types.BINARY, Types.VARBINARY, Types.LONGVARBINARY, Types.BLOB -> {
            val v = rs.getBytes(i)
            if (rs.wasNull()) "null" else "\"${Base64.getEncoder().encodeToString(v)}\""
        }
        else -> {
            val v = rs.getString(i)
            if (rs.wasNull()) "null" else "\"${v.jsonEsc()}\""
        }
    }

    private fun String.jsonEsc() = replace("\\", "\\\\").replace("\"", "\\\"")
        .replace("\n", "\\n").replace("\r", "\\r").replace("\t", "\\t")
}
