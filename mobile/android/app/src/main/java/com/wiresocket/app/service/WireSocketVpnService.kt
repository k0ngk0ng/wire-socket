package com.wiresocket.app.service

import android.app.PendingIntent
import android.content.Intent
import android.net.VpnService
import android.os.Build
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import com.wiresocket.app.R
import com.wiresocket.app.WireSocketApp
import com.wiresocket.app.data.ConnectionState
import com.wiresocket.app.data.VpnStateHolder
import com.wiresocket.app.data.VpnStatus
import com.wiresocket.app.ui.MainActivity
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.OutputStreamWriter
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.atomic.AtomicBoolean

class WireSocketVpnService : VpnService() {

    companion object {
        private const val TAG = "WireSocketVpnService"

        const val ACTION_CONNECT = "com.wiresocket.CONNECT"
        const val ACTION_DISCONNECT = "com.wiresocket.DISCONNECT"

        const val EXTRA_SERVER = "server"
        const val EXTRA_USERNAME = "username"
        const val EXTRA_PASSWORD = "password"

        private const val MTU = 1420
    }

    private var vpnInterface: ParcelFileDescriptor? = null
    private var tunnel: Mobile.Tunnel? = null
    private val isRunning = AtomicBoolean(false)
    private var currentServer: String = ""

    override fun onCreate() {
        super.onCreate()
        Log.d(TAG, "VPN Service created")
        VpnStateHolder.setServiceRunning(true)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        when (intent?.action) {
            ACTION_CONNECT -> {
                val server = intent.getStringExtra(EXTRA_SERVER) ?: return START_NOT_STICKY
                val username = intent.getStringExtra(EXTRA_USERNAME) ?: return START_NOT_STICKY
                val password = intent.getStringExtra(EXTRA_PASSWORD) ?: return START_NOT_STICKY
                connect(server, username, password)
            }
            ACTION_DISCONNECT -> {
                disconnect()
            }
        }
        return START_STICKY
    }

    override fun onDestroy() {
        super.onDestroy()
        Log.d(TAG, "VPN Service destroyed")
        disconnect()
        VpnStateHolder.setServiceRunning(false)
    }

    private fun connect(server: String, username: String, password: String) {
        if (isRunning.getAndSet(true)) {
            Log.w(TAG, "Already connecting/connected")
            return
        }

        currentServer = server
        VpnStateHolder.updateState(ConnectionState.CONNECTING)
        startForegroundNotification("Connecting...")

        Thread {
            try {
                // Step 1: Authenticate and get WireGuard config
                Log.d(TAG, "Authenticating with server: $server")
                val config = authenticate(server, username, password)
                Log.d(TAG, "Got WireGuard config, address: ${config.address}")

                // Step 2: Create VPN interface using VpnService.Builder
                val vpnFd = createVpnInterface(config)
                if (vpnFd == null) {
                    throw Exception("Failed to create VPN interface")
                }
                vpnInterface = vpnFd
                Log.d(TAG, "VPN interface created, fd: ${vpnFd.fd}")

                // Step 3: Start WireGuard tunnel with the file descriptor
                val tunnelConfig = JSONObject().apply {
                    put("privateKey", config.privateKey)
                    put("address", config.address)
                    put("dns", config.dns)
                    put("peerPublicKey", config.peerPublicKey)
                    put("peerEndpoint", config.peerEndpoint)
                    put("allowedIPs", JSONArray(config.allowedIPs))
                    put("mtu", MTU)
                }

                tunnel = Mobile.newTunnel()
                tunnel?.startWithFD(vpnFd.fd.toLong(), tunnelConfig.toString())
                Log.d(TAG, "WireGuard tunnel started")

                // Step 4: Update UI state
                android.os.Handler(android.os.Looper.getMainLooper()).post {
                    VpnStateHolder.updateStatus(VpnStatus(
                        state = ConnectionState.CONNECTED,
                        server = server,
                        assignedIp = config.address,
                        connectedAtUnix = System.currentTimeMillis() / 1000
                    ))
                    updateNotification("Connected: ${config.address}")
                }

            } catch (e: Exception) {
                Log.e(TAG, "Connection failed", e)
                android.os.Handler(android.os.Looper.getMainLooper()).post {
                    VpnStateHolder.setError(e.message ?: "Connection failed")
                }
                cleanup()
            }
        }.start()
    }

    private fun disconnect() {
        Log.d(TAG, "Disconnecting...")
        VpnStateHolder.updateState(ConnectionState.DISCONNECTING)
        cleanup()
        VpnStateHolder.updateState(ConnectionState.DISCONNECTED)
    }

    private fun cleanup() {
        try {
            tunnel?.stop()
            tunnel = null
        } catch (e: Exception) {
            Log.e(TAG, "Error stopping tunnel", e)
        }

        try {
            vpnInterface?.close()
            vpnInterface = null
        } catch (e: Exception) {
            Log.e(TAG, "Error closing VPN interface", e)
        }

        isRunning.set(false)
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun createVpnInterface(config: WireGuardConfig): ParcelFileDescriptor? {
        val builder = Builder()
            .setSession("WireSocket")
            .setMtu(MTU)

        // Set the VPN address
        val addressParts = config.address.split("/")
        val address = addressParts[0]
        val prefixLength = if (addressParts.size > 1) addressParts[1].toInt() else 32
        builder.addAddress(address, prefixLength)

        // Add DNS servers
        if (config.dns.isNotEmpty()) {
            config.dns.split(",").forEach { dns ->
                val dnsIp = dns.trim()
                if (dnsIp.isNotEmpty()) {
                    try {
                        builder.addDnsServer(dnsIp)
                    } catch (e: Exception) {
                        Log.w(TAG, "Failed to add DNS server: $dnsIp", e)
                    }
                }
            }
        }

        // Add routes from allowedIPs
        config.allowedIPs.forEach { route ->
            try {
                val routeParts = route.split("/")
                val routeAddress = routeParts[0]
                val routePrefix = if (routeParts.size > 1) routeParts[1].toInt() else 32
                builder.addRoute(routeAddress, routePrefix)
                Log.d(TAG, "Added route: $route")
            } catch (e: Exception) {
                Log.w(TAG, "Failed to add route: $route", e)
            }
        }

        // Exclude the VPN server from the tunnel to prevent routing loops
        try {
            val serverHost = URL(normalizeServerUrl(currentServer)).host
            builder.addDisallowedApplication(packageName) // Don't route our own traffic
        } catch (e: Exception) {
            Log.w(TAG, "Failed to set server bypass", e)
        }

        return builder.establish()
    }

    private fun authenticate(server: String, username: String, password: String): WireGuardConfig {
        val baseUrl = normalizeServerUrl(server)

        // Login
        val loginUrl = URL("$baseUrl/api/auth/login")
        val loginData = JSONObject().apply {
            put("username", username)
            put("password", password)
        }

        val token = httpPost(loginUrl, loginData.toString())
            .let { JSONObject(it).getString("token") }

        // Get config
        val configUrl = URL("$baseUrl/api/config")
        val configResponse = httpGet(configUrl, token)
        val configJson = JSONObject(configResponse)

        val wgConfig = configJson.getJSONObject("config")
        val peer = wgConfig.getJSONObject("peer")

        val allowedIPs = peer.getString("allowed_ips")
            .split(",")
            .map { it.trim() }
            .filter { it.isNotEmpty() }

        // Get tunnel URL for WebSocket endpoint (used for the tunnel, but we convert to direct endpoint)
        val tunnelUrl = configJson.optString("tunnel_url", server)
        val peerEndpoint = buildPeerEndpoint(tunnelUrl, server)

        return WireGuardConfig(
            privateKey = wgConfig.getString("private_key"),
            address = wgConfig.getString("address"),
            dns = wgConfig.optString("dns", ""),
            peerPublicKey = peer.getString("public_key"),
            peerEndpoint = peerEndpoint,
            allowedIPs = allowedIPs
        )
    }

    private fun buildPeerEndpoint(tunnelUrl: String, fallback: String): String {
        // The peer endpoint should be the server's WireGuard port
        // In this architecture, we tunnel WireGuard over WebSocket,
        // so the endpoint is typically the local tunnel proxy
        // For now, use a standard WireGuard port setup
        return try {
            val url = URL(normalizeServerUrl(tunnelUrl.ifEmpty { fallback }))
            "${url.host}:51820"
        } catch (e: Exception) {
            "127.0.0.1:51820"
        }
    }

    private fun normalizeServerUrl(server: String): String {
        var url = server.trimEnd('/')
        if (!url.startsWith("http://") && !url.startsWith("https://")) {
            url = if (url.contains(":") && !url.substringAfter(":").equals("443")) {
                "http://$url"
            } else {
                "https://$url"
            }
        }
        return url
    }

    private fun httpPost(url: URL, body: String): String {
        val connection = url.openConnection() as HttpURLConnection
        return try {
            connection.requestMethod = "POST"
            connection.setRequestProperty("Content-Type", "application/json")
            connection.doOutput = true

            OutputStreamWriter(connection.outputStream).use { writer ->
                writer.write(body)
            }

            if (connection.responseCode != HttpURLConnection.HTTP_OK) {
                throw Exception("HTTP ${connection.responseCode}: ${connection.responseMessage}")
            }

            BufferedReader(InputStreamReader(connection.inputStream)).use { reader ->
                reader.readText()
            }
        } finally {
            connection.disconnect()
        }
    }

    private fun httpGet(url: URL, token: String): String {
        val connection = url.openConnection() as HttpURLConnection
        return try {
            connection.requestMethod = "GET"
            connection.setRequestProperty("Authorization", "Bearer $token")

            if (connection.responseCode != HttpURLConnection.HTTP_OK) {
                throw Exception("HTTP ${connection.responseCode}: ${connection.responseMessage}")
            }

            BufferedReader(InputStreamReader(connection.inputStream)).use { reader ->
                reader.readText()
            }
        } finally {
            connection.disconnect()
        }
    }

    private fun startForegroundNotification(message: String) {
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE
        )

        val notification = NotificationCompat.Builder(this, WireSocketApp.VPN_NOTIFICATION_CHANNEL_ID)
            .setContentTitle("WireSocket VPN")
            .setContentText(message)
            .setSmallIcon(R.drawable.ic_vpn)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(WireSocketApp.VPN_NOTIFICATION_ID, notification,
                android.content.pm.ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(WireSocketApp.VPN_NOTIFICATION_ID, notification)
        }
    }

    private fun updateNotification(message: String) {
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            Intent(this, MainActivity::class.java),
            PendingIntent.FLAG_IMMUTABLE
        )

        val notification = NotificationCompat.Builder(this, WireSocketApp.VPN_NOTIFICATION_CHANNEL_ID)
            .setContentTitle("WireSocket VPN")
            .setContentText(message)
            .setSmallIcon(R.drawable.ic_vpn)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()

        val manager = getSystemService(android.app.NotificationManager::class.java)
        manager.notify(WireSocketApp.VPN_NOTIFICATION_ID, notification)
    }

    data class WireGuardConfig(
        val privateKey: String,
        val address: String,
        val dns: String,
        val peerPublicKey: String,
        val peerEndpoint: String,
        val allowedIPs: List<String>
    )
}
