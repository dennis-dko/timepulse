package io.github.dennisdko.timepulse

import com.sun.net.httpserver.HttpServer
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.InetSocketAddress
import java.util.Collections

class InstallationTest {

    private val english = Language("en", emptyMap())

    /** answering serves [status], with a refusal's body, to every request on a port of its own while [block] runs. */
    private fun <T> answering(status: Int, block: (String) -> T): T {
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)

        server.createContext("/") { exchange ->
            val body = """{"error":{"code":"internal","message":"something went wrong"}}""".toByteArray()

            exchange.responseHeaders.add("Content-Type", "application/json")
            exchange.sendResponseHeaders(status, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }

        server.start()

        try {
            return block("http://127.0.0.1:${server.address.port}")
        } finally {
            server.stop(0)
        }
    }

    // A token the installation no longer knows - taken back, or run out - is told
    // apart from every other refusal, because the app answers it by disconnecting
    // rather than by waiting to try again: whether it opens the web interface's
    // session or asks about the reminders.
    @Test
    fun aTokenTakenBackIsToldApartFromOtherRefusals() {
        answering(401) { address ->
            assertThrows(TokenRefused::class.java) { runBlocking { Installation(address, english).handOver("timepulse_gone") } }
            assertThrows(TokenRefused::class.java) { runBlocking { Installation(address, english).reminders("timepulse_gone") } }
        }

        answering(500) { address ->
            val opening = assertThrows(Refused::class.java) { runBlocking { Installation(address, english).handOver("timepulse_kept") } }
            val asking = assertThrows(Refused::class.java) { runBlocking { Installation(address, english).reminders("timepulse_kept") } }

            assertFalse("a failing installation is taken for a token taken back", opening is TokenRefused || asking is TokenRefused)
        }
    }

    /** openingWith answers every request with a session opened under the cookie [name], or with none where it is null. */
    private fun <T> openingWith(name: String?, block: (String) -> T): T {
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)

        server.createContext("/") { exchange ->
            val body = """{"data":{}}""".toByteArray()

            if (name != null) exchange.responseHeaders.add("Set-Cookie", "$name=the-session; Path=/; HttpOnly")
            exchange.responseHeaders.add("Content-Type", "application/json")
            exchange.sendResponseHeaders(201, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }

        server.start()

        try {
            return block("http://127.0.0.1:${server.address.port}")
        } finally {
            server.stop(0)
        }
    }

    // A token's session comes back under the name TimePulse gives it, and an
    // answer without it is not this application's.
    @Test
    fun aSessionIsTakenUnderItsName() {
        openingWith("timepulse_session") { address ->
            val cookies = runBlocking { Installation(address, english).handOver("timepulse_abc123") }

            assertTrue("the session was not taken", cookies.any { it.startsWith("timepulse_session=") })
        }

        openingWith(null) { address ->
            assertThrows(Refused::class.java) { runBlocking { Installation(address, english).handOver("timepulse_abc123") } }
        }
    }

    /**
     * servingContracts opens a session for every sign-in and says the installation
     * serves contracts up to [newest], or says nothing of contracts where it is
     * null, as an installation from before them does. [block] is given the address
     * and every request it saw, with the contract the app said it speaks.
     */
    private fun <T> servingContracts(newest: Int?, block: (String, List<String>) -> T): T {
        val seen = Collections.synchronizedList(mutableListOf<String>())
        val server = HttpServer.create(InetSocketAddress("127.0.0.1", 0), 0)

        server.createContext("/") { exchange ->
            seen += "${exchange.requestMethod} ${exchange.requestURI.path} ${exchange.requestHeaders.getFirst("X-TimePulse-App-Contract").orEmpty()}".trim()

            val range = newest?.let { ""","appContracts":{"oldest":1,"newest":$it}""" }.orEmpty()
            val body = """{"data":{"user":{}$range}}""".toByteArray()

            exchange.responseHeaders.add("Set-Cookie", "timepulse_session=the-session; Path=/; HttpOnly")
            exchange.responseHeaders.add("Set-Cookie", "timepulse_csrf=the-token; Path=/")
            exchange.responseHeaders.add("Content-Type", "application/json")
            exchange.sendResponseHeaders(201, body.size.toLong())
            exchange.responseBody.use { it.write(body) }
        }

        server.start()

        try {
            return block("http://127.0.0.1:${server.address.port}", seen)
        } finally {
            server.stop(0)
        }
    }

    // The app says which contract it speaks when it signs in. Where the
    // installation's newest contract is older than its own, it ends the session it
    // was just given, which it could not use, and says so in the installation's own
    // words. An installation from before contracts answers no range, which is read
    // as the first contract, so today's app goes on working with it.
    @Test
    fun anInstallationTooOldForTheAppIsRefusedAndSignedOutOf() {
        servingContracts(newest = appContract - 1) { address, seen ->
            val refused = assertThrows(Refused::class.java) { runBlocking { Installation(address, english).handOver("timepulse_abc123") } }

            assertTrue("refused with \"${refused.message}\"", refused.message.orEmpty().contains("too old for this app"))
            assertTrue("the session it could not use was kept: $seen", seen.any { it.startsWith("POST /api/v1/auth/logout") })
        }

        servingContracts(newest = appContract) { address, seen ->
            runBlocking { Installation(address, english).handOver("timepulse_abc123") }

            assertTrue("the app did not say which contract it speaks: $seen", seen.contains("POST /api/v1/auth/token $appContract"))
        }

        servingContracts(newest = null) { address, _ ->
            runBlocking { Installation(address, english).handOver("timepulse_abc123") }
        }

        assertEquals(1, newestServed("""{"data":{}}"""))
        assertEquals(1, newestServed("not an answer"))
        assertEquals(4, newestServed("""{"data":{"appContracts":{"oldest":2,"newest":4}}}"""))
    }
}
