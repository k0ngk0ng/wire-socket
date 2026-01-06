package com.wiresocket.app.data

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * Connection state matching SDK states
 */
enum class ConnectionState {
    DISCONNECTED,
    CONNECTING,
    CONNECTED,
    DISCONNECTING,
    RECONNECTING,
    FAILED
}

/**
 * VPN connection status
 */
data class VpnStatus(
    val state: ConnectionState = ConnectionState.DISCONNECTED,
    val server: String = "",
    val assignedIp: String = "",
    val connectedAtUnix: Long = 0,
    val durationMs: Long = 0,
    val error: String = "",
    val rxBytes: Long = 0,
    val txBytes: Long = 0,
    val rxSpeed: Long = 0,
    val txSpeed: Long = 0
)

/**
 * Singleton to share VPN state between UI and Service
 */
object VpnStateHolder {
    private val _status = MutableStateFlow(VpnStatus())
    val status: StateFlow<VpnStatus> = _status.asStateFlow()

    private val _isServiceRunning = MutableStateFlow(false)
    val isServiceRunning: StateFlow<Boolean> = _isServiceRunning.asStateFlow()

    fun updateStatus(newStatus: VpnStatus) {
        _status.value = newStatus
    }

    fun updateState(state: ConnectionState) {
        _status.value = _status.value.copy(state = state)
    }

    fun updateStats(rxBytes: Long, txBytes: Long, rxSpeed: Long, txSpeed: Long) {
        _status.value = _status.value.copy(
            rxBytes = rxBytes,
            txBytes = txBytes,
            rxSpeed = rxSpeed,
            txSpeed = txSpeed
        )
    }

    fun setError(error: String) {
        _status.value = _status.value.copy(
            state = ConnectionState.FAILED,
            error = error
        )
    }

    fun setServiceRunning(running: Boolean) {
        _isServiceRunning.value = running
    }

    fun reset() {
        _status.value = VpnStatus()
    }
}
