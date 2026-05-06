package com.scriptease.jdbccli.client

import java.net.UnixDomainSocketAddress
import java.nio.ByteBuffer
import java.nio.channels.SocketChannel
import java.nio.file.Path

object HttpClient {
    private val sockPath: Path = Path.of(System.getProperty("user.home"), ".jdbc-cli", "sock")

    fun get(path: String): String = request("GET", path, null)

    fun post(path: String, body: String): String = request("POST", path, body)

    private fun request(method: String, path: String, body: String?): String {
        val addr = UnixDomainSocketAddress.of(sockPath)
        SocketChannel.open(addr).use { ch ->
            val bodyBytes = body?.toByteArray(Charsets.UTF_8)
            val req = buildString {
                append("$method $path HTTP/1.1\r\n")
                append("Host: localhost\r\n")
                if (bodyBytes != null) {
                    append("Content-Type: application/json\r\n")
                    append("Content-Length: ${bodyBytes.size}\r\n")
                }
                append("Connection: close\r\n")
                append("\r\n")
                if (body != null) append(body)
            }
            ch.write(ByteBuffer.wrap(req.toByteArray(Charsets.UTF_8)))

            val sb = StringBuilder()
            val buf = ByteBuffer.allocate(8192)
            while (ch.read(buf) != -1) {
                buf.flip()
                sb.append(Charsets.UTF_8.decode(buf))
                buf.clear()
            }

            return parseBody(sb.toString())
        }
    }

    private fun parseBody(raw: String): String {
        val sep = raw.indexOf("\r\n\r\n")
        if (sep == -1) error("Malformed HTTP response")
        val headerSection = raw.substring(0, sep)
        val body = raw.substring(sep + 4)

        val statusLine = headerSection.lineSequence().first()
        val statusCode = statusLine.split(" ")[1].toIntOrNull() ?: 0
        if (statusCode >= 400) {
            System.err.println(body.trim())
            System.exit(1)
        }
        return body.trim()
    }
}
