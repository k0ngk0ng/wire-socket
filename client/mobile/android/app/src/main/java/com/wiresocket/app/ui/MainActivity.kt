package com.wiresocket.app.ui

import android.app.Activity
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.wiresocket.app.data.ConnectionState
import com.wiresocket.app.data.SettingsRepository
import com.wiresocket.app.data.VpnStateHolder
import com.wiresocket.app.service.WireSocketVpnService
import com.wiresocket.app.ui.theme.WireSocketTheme
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {

    private lateinit var settingsRepository: SettingsRepository
    private var pendingConnection: ConnectionParams? = null

    private data class ConnectionParams(
        val server: String,
        val username: String,
        val password: String
    )

    private val vpnPermissionLauncher = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == Activity.RESULT_OK) {
            pendingConnection?.let { params ->
                startVpnService(params.server, params.username, params.password)
            }
        } else {
            Toast.makeText(this, "VPN permission denied", Toast.LENGTH_SHORT).show()
        }
        pendingConnection = null
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        settingsRepository = SettingsRepository(this)

        setContent {
            WireSocketTheme {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    MainScreen(
                        settingsRepository = settingsRepository,
                        onConnect = { server, username, password ->
                            requestVpnPermission(server, username, password)
                        },
                        onDisconnect = {
                            stopVpnService()
                        }
                    )
                }
            }
        }
    }

    private fun requestVpnPermission(server: String, username: String, password: String) {
        val intent = VpnService.prepare(this)
        if (intent != null) {
            pendingConnection = ConnectionParams(server, username, password)
            vpnPermissionLauncher.launch(intent)
        } else {
            startVpnService(server, username, password)
        }
    }

    private fun startVpnService(server: String, username: String, password: String) {
        val intent = Intent(this, WireSocketVpnService::class.java).apply {
            action = WireSocketVpnService.ACTION_CONNECT
            putExtra(WireSocketVpnService.EXTRA_SERVER, server)
            putExtra(WireSocketVpnService.EXTRA_USERNAME, username)
            putExtra(WireSocketVpnService.EXTRA_PASSWORD, password)
        }
        startForegroundService(intent)
    }

    private fun stopVpnService() {
        val intent = Intent(this, WireSocketVpnService::class.java).apply {
            action = WireSocketVpnService.ACTION_DISCONNECT
        }
        startService(intent)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MainScreen(
    settingsRepository: SettingsRepository,
    onConnect: (String, String, String) -> Unit,
    onDisconnect: () -> Unit
) {
    val vpnStatus by VpnStateHolder.status.collectAsStateWithLifecycle()
    val savedServer by settingsRepository.serverFlow.collectAsStateWithLifecycle(initialValue = "")
    val savedUsername by settingsRepository.usernameFlow.collectAsStateWithLifecycle(initialValue = "")
    val savedPassword = remember { settingsRepository.getPassword() }

    var server by remember(savedServer) { mutableStateOf(savedServer) }
    var username by remember(savedUsername) { mutableStateOf(savedUsername) }
    var password by remember(savedPassword) { mutableStateOf(savedPassword) }

    val scope = rememberCoroutineScope()
    val isConnected = vpnStatus.state == ConnectionState.CONNECTED
    val isConnecting = vpnStatus.state == ConnectionState.CONNECTING ||
                       vpnStatus.state == ConnectionState.RECONNECTING

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("WireSocket VPN") },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.primaryContainer
                )
            )
        }
    ) { paddingValues ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(paddingValues)
                .padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            // Status Card
            StatusCard(vpnStatus)

            Spacer(modifier = Modifier.height(24.dp))

            // Connection Form
            if (!isConnected && !isConnecting) {
                OutlinedTextField(
                    value = server,
                    onValueChange = { server = it },
                    label = { Text("Server") },
                    placeholder = { Text("https://vpn.example.com") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )

                Spacer(modifier = Modifier.height(12.dp))

                OutlinedTextField(
                    value = username,
                    onValueChange = { username = it },
                    label = { Text("Username") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true
                )

                Spacer(modifier = Modifier.height(12.dp))

                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("Password") },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password)
                )

                Spacer(modifier = Modifier.height(24.dp))
            }

            // Connect/Disconnect Button
            Button(
                onClick = {
                    if (isConnected || isConnecting) {
                        onDisconnect()
                    } else {
                        scope.launch {
                            settingsRepository.saveCredentials(server, username, password)
                        }
                        onConnect(server, username, password)
                    }
                },
                modifier = Modifier
                    .fillMaxWidth()
                    .height(56.dp),
                colors = if (isConnected) {
                    ButtonDefaults.buttonColors(containerColor = MaterialTheme.colorScheme.error)
                } else {
                    ButtonDefaults.buttonColors()
                },
                enabled = !isConnecting && (isConnected || (server.isNotBlank() && username.isNotBlank() && password.isNotBlank()))
            ) {
                if (isConnecting) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(24.dp),
                        color = MaterialTheme.colorScheme.onPrimary
                    )
                    Spacer(modifier = Modifier.width(8.dp))
                    Text("Connecting...")
                } else if (isConnected) {
                    Text("Disconnect")
                } else {
                    Text("Connect")
                }
            }

            // Error message
            if (vpnStatus.state == ConnectionState.FAILED && vpnStatus.error.isNotBlank()) {
                Spacer(modifier = Modifier.height(16.dp))
                Card(
                    colors = CardDefaults.cardColors(
                        containerColor = MaterialTheme.colorScheme.errorContainer
                    )
                ) {
                    Text(
                        text = vpnStatus.error,
                        modifier = Modifier.padding(16.dp),
                        color = MaterialTheme.colorScheme.onErrorContainer
                    )
                }
            }
        }
    }
}

@Composable
fun StatusCard(status: com.wiresocket.app.data.VpnStatus) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(
            containerColor = when (status.state) {
                ConnectionState.CONNECTED -> MaterialTheme.colorScheme.primaryContainer
                ConnectionState.CONNECTING, ConnectionState.RECONNECTING -> MaterialTheme.colorScheme.secondaryContainer
                ConnectionState.FAILED -> MaterialTheme.colorScheme.errorContainer
                else -> MaterialTheme.colorScheme.surfaceVariant
            }
        )
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Text(
                text = when (status.state) {
                    ConnectionState.CONNECTED -> "Connected"
                    ConnectionState.CONNECTING -> "Connecting"
                    ConnectionState.DISCONNECTING -> "Disconnecting"
                    ConnectionState.RECONNECTING -> "Reconnecting"
                    ConnectionState.FAILED -> "Failed"
                    ConnectionState.DISCONNECTED -> "Disconnected"
                },
                style = MaterialTheme.typography.headlineMedium
            )

            if (status.state == ConnectionState.CONNECTED) {
                Spacer(modifier = Modifier.height(8.dp))
                Text(
                    text = status.assignedIp,
                    style = MaterialTheme.typography.bodyLarge
                )
                Spacer(modifier = Modifier.height(8.dp))
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceEvenly
                ) {
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text("↓ ${formatBytes(status.rxBytes)}")
                        Text(
                            "${formatSpeed(status.rxSpeed)}/s",
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                    Column(horizontalAlignment = Alignment.CenterHorizontally) {
                        Text("↑ ${formatBytes(status.txBytes)}")
                        Text(
                            "${formatSpeed(status.txSpeed)}/s",
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                }
            }
        }
    }
}

private fun formatBytes(bytes: Long): String {
    return when {
        bytes < 1024 -> "$bytes B"
        bytes < 1024 * 1024 -> "${bytes / 1024} KB"
        bytes < 1024 * 1024 * 1024 -> "${bytes / (1024 * 1024)} MB"
        else -> "${bytes / (1024 * 1024 * 1024)} GB"
    }
}

private fun formatSpeed(bytesPerSec: Long): String {
    return formatBytes(bytesPerSec)
}
