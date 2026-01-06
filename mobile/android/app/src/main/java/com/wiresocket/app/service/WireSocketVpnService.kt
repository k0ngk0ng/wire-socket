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
import mobile.EventHandler
import org.json.JSONObject
import java.util.concurrent.atomic.AtomicBoolean

class WireSocketVpnService : VpnService(), EventHandler {

    companion object {
        private const val TAG = "WireSocketVpnService"

        const val ACTION_CONNECT = "com.wiresocket.CONNECT"
        const val ACTION_DISCONNECT = "com.wiresocket.DISCONNECT"

        const val EXTRA_SERVER = "server"
        const val EXTRA_USERNAME = "username"
        const val EXTRA_PASSWORD = "password"
    }

    private var vpnInterface: ParcelFileDescriptor? = null
    private var sdkClient: Mobile.Client? = null
    private val isRunning = AtomicBoolean(false)

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

        VpnStateHolder.updateState(ConnectionState.CONNECTING)
        startForegroundNotification("Connecting...")

        Thread {
            try {
                // Initialize SDK client
                sdkClient = Mobile.newClient().also { client ->
                    client.setEventHandler(this)
                }

                // Connect via SDK (handles auth, tunnel, WireGuard config)
                sdkClient?.connect(server, username, password, true)

            } catch (e: Exception) {
                Log.e(TAG, "Connection failed", e)
                VpnStateHolder.setError(e.message ?: "Connection failed")
                isRunning.set(false)
                stopForeground(STOP_FOREGROUND_REMOVE)
                stopSelf()
            }
        }.start()
    }

    private fun disconnect() {
        Log.d(TAG, "Disconnecting...")
        VpnStateHolder.updateState(ConnectionState.DISCONNECTING)

        try {
            sdkClient?.disconnect()
            sdkClient?.close()
            sdkClient = null
        } catch (e: Exception) {
            Log.e(TAG, "Error disconnecting SDK", e)
        }

        vpnInterface?.close()
        vpnInterface = null

        isRunning.set(false)
        VpnStateHolder.updateState(ConnectionState.DISCONNECTED)
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    // EventHandler implementation - called by Go SDK
    override fun onEvent(eventType: String, jsonData: String) {
        Log.d(TAG, "SDK Event: $eventType")

        try {
            val json = JSONObject(jsonData)

            when (eventType) {
                "connecting" -> {
                    VpnStateHolder.updateState(ConnectionState.CONNECTING)
                    updateNotification("Connecting...")
                }
                "connected" -> {
                    val status = json.optJSONObject("status")
                    val assignedIp = status?.optString("assignedIP") ?: ""
                    val server = status?.optString("server") ?: ""

                    // Setup VPN interface after SDK connects
                    setupVpnInterface(assignedIp)

                    VpnStateHolder.updateStatus(VpnStatus(
                        state = ConnectionState.CONNECTED,
                        server = server,
                        assignedIp = assignedIp,
                        connectedAtUnix = status?.optLong("connectedAtUnix") ?: 0
                    ))
                    updateNotification("Connected: $assignedIp")
                }
                "disconnected" -> {
                    VpnStateHolder.updateState(ConnectionState.DISCONNECTED)
                    disconnect()
                }
                "reconnecting" -> {
                    VpnStateHolder.updateState(ConnectionState.RECONNECTING)
                    updateNotification("Reconnecting...")
                }
                "error" -> {
                    val error = json.optString("error", "Unknown error")
                    VpnStateHolder.setError(error)
                    disconnect()
                }
                "stats_updated" -> {
                    val stats = json.optJSONObject("stats")
                    if (stats != null) {
                        VpnStateHolder.updateStats(
                            rxBytes = stats.optLong("rxBytes"),
                            txBytes = stats.optLong("txBytes"),
                            rxSpeed = stats.optLong("rxSpeed"),
                            txSpeed = stats.optLong("txSpeed")
                        )
                    }
                }
            }
        } catch (e: Exception) {
            Log.e(TAG, "Error parsing event", e)
        }
    }

    private fun setupVpnInterface(assignedIp: String) {
        // Note: In a real implementation, the SDK would provide the full
        // WireGuard config and we'd create the TUN interface here.
        // For now, the SDK uses userspace networking (netstack).

        // This is a placeholder - actual VPN routing would require:
        // 1. Getting the WireGuard config from SDK
        // 2. Creating TUN interface with Builder
        // 3. Routing packets through the tunnel

        Log.d(TAG, "VPN interface setup for IP: $assignedIp")
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
}
