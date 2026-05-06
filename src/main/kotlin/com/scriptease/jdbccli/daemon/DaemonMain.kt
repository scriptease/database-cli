package com.scriptease.jdbccli.daemon

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

object Router : Handler.Abstract() {
    override fun handle(request: Request, response: Response, callback: Callback): Boolean {
        val path = request.httpURI.path
        val method = request.method
        return when {
            method == "GET" && path == "/ping" -> {
                sendText(response, callback, "ok")
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
        val bytes = """{"error":"$msg"}""".toByteArray(Charsets.UTF_8)
        response.status = status
        response.headers.put("Content-Type", "application/json; charset=utf-8")
        response.headers.put("Content-Length", bytes.size.toString())
        response.write(true, ByteBuffer.wrap(bytes), callback)
    }
}
