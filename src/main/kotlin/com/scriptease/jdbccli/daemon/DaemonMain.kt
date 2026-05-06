package com.scriptease.jdbccli.daemon

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import org.eclipse.jetty.io.Content
import org.eclipse.jetty.server.Handler
import org.eclipse.jetty.server.Request
import org.eclipse.jetty.server.Response
import org.eclipse.jetty.server.Server
import org.eclipse.jetty.unixdomain.server.UnixDomainServerConnector
import org.eclipse.jetty.util.Callback
import org.eclipse.jetty.util.thread.QueuedThreadPool
import java.nio.ByteBuffer
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.attribute.PosixFilePermission

object DaemonMain {
    private val sockPath: Path = Path.of(System.getProperty("user.home"), ".jdbc-cli", "sock")

    fun run() {
        Files.createDirectories(sockPath.parent)
        Files.deleteIfExists(sockPath)

        val threadPool = QueuedThreadPool()
        val server = Server(threadPool)

        val connector = UnixDomainServerConnector(server)
        connector.unixDomainPath = sockPath
        server.addConnector(connector)

        server.handler = Router
        server.start()

        Files.setPosixFilePermissions(
            sockPath,
            setOf(PosixFilePermission.OWNER_READ, PosixFilePermission.OWNER_WRITE)
        )

        Runtime.getRuntime().addShutdownHook(Thread {
            Pools.closeAll()
            server.stop()
        })

        System.err.println("jdbc-cli daemon started on $sockPath")
        server.join()
    }
}

@Serializable
private data class OpenReq(val alias: String, val jdbcUrl: String, val user: String = "", val password: String = "")

@Serializable
private data class CloseReq(val alias: String)

object Router : Handler.Abstract() {
    private val json = Json { ignoreUnknownKeys = true }

    override fun handle(request: Request, response: Response, callback: Callback): Boolean {
        val path = request.httpURI.path
        val method = request.method
        return when {
            method == "GET" && path == "/ping" -> {
                sendText(response, callback, "ok")
                true
            }
            method == "GET" && path == "/list" -> {
                val aliases = Pools.list()
                val body = aliases.joinToString(",", "[", "]") { "\"$it\"" }
                sendJson(response, callback, body)
                true
            }
            method == "POST" && path == "/open" -> {
                val body = Content.Source.asString(request, Charsets.UTF_8)
                val req = json.decodeFromString<OpenReq>(body)
                try {
                    Pools.open(req.alias, req.jdbcUrl, req.user, req.password)
                    sendJson(response, callback, """{"ok":true}""")
                } catch (e: Exception) {
                    sendError(response, callback, 400, e.message ?: "open failed")
                }
                true
            }
            method == "POST" && path == "/close" -> {
                val body = Content.Source.asString(request, Charsets.UTF_8)
                val req = json.decodeFromString<CloseReq>(body)
                try {
                    Pools.close(req.alias)
                    sendJson(response, callback, """{"ok":true}""")
                } catch (e: Exception) {
                    sendError(response, callback, 400, e.message ?: "close failed")
                }
                true
            }
            else -> {
                sendError(response, callback, 404, "not found")
                true
            }
        }
    }

    fun sendText(response: Response, callback: Callback, text: String) {
        val bytes = text.toByteArray(Charsets.UTF_8)
        response.status = 200
        response.headers.put("Content-Type", "text/plain; charset=utf-8")
        response.headers.put("Content-Length", bytes.size.toString())
        response.write(true, ByteBuffer.wrap(bytes), callback)
    }

    fun sendJson(response: Response, callback: Callback, json: String) {
        val bytes = json.toByteArray(Charsets.UTF_8)
        response.status = 200
        response.headers.put("Content-Type", "application/json; charset=utf-8")
        response.headers.put("Content-Length", bytes.size.toString())
        response.write(true, ByteBuffer.wrap(bytes), callback)
    }

    fun sendError(response: Response, callback: Callback, status: Int, msg: String) {
        val escaped = msg.replace("\"", "\\\"")
        val bytes = """{"error":"$escaped"}""".toByteArray(Charsets.UTF_8)
        response.status = status
        response.headers.put("Content-Type", "application/json; charset=utf-8")
        response.headers.put("Content-Length", bytes.size.toString())
        response.write(true, ByteBuffer.wrap(bytes), callback)
    }
}
