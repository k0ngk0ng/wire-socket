package connection

import (
	"sync"
	"time"
)

// TunnelConnection represents a single tunnel connection
type TunnelConnection struct {
	ID            string    `json:"id"`             // Tunnel ID (e.g., "hk-01")
	Name          string    `json:"name"`           // Display name
	URL           string    `json:"url"`            // Tunnel WebSocket URL
	ServerAddress string    `json:"server_address"` // API address
	State         State     `json:"state"`
	AssignedIP    string    `json:"assigned_ip,omitempty"`
	ConnectedAt   time.Time `json:"connected_at,omitempty"`
	RxBytes       uint64    `json:"rx_bytes"`
	TxBytes       uint64    `json:"tx_bytes"`
	Error         string    `json:"error,omitempty"`
}

// MultiTunnelStatus represents status of all tunnel connections
type MultiTunnelStatus struct {
	Connections []TunnelConnection `json:"connections"`
	TotalRx     uint64             `json:"total_rx"`
	TotalTx     uint64             `json:"total_tx"`
}

// MultiManager manages multiple VPN tunnel connections
type MultiManager struct {
	mu          sync.RWMutex
	connections map[string]*tunnelConn // keyed by tunnel ID
	configPath  string
}

// tunnelConn is internal connection state
type tunnelConn struct {
	TunnelConnection
	// wgInterface  *wireguard.Interface  // Each tunnel has its own interface
	// tunnelClient *wstunnel.Client
	stopCh chan struct{}
}

// NewMultiManager creates a new multi-tunnel manager
func NewMultiManager(configPath string) *MultiManager {
	return &MultiManager{
		connections: make(map[string]*tunnelConn),
		configPath:  configPath,
	}
}

// Connect connects to a specific tunnel
func (m *MultiManager) Connect(tunnelID, serverAddr, username, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already connected
	if conn, exists := m.connections[tunnelID]; exists {
		if conn.State == StateConnected || conn.State == StateConnecting {
			return nil // Already connected/connecting
		}
	}

	// Create new connection
	conn := &tunnelConn{
		TunnelConnection: TunnelConnection{
			ID:            tunnelID,
			ServerAddress: serverAddr,
			State:         StateConnecting,
		},
		stopCh: make(chan struct{}),
	}

	m.connections[tunnelID] = conn

	// Start connection in background
	go m.connectTunnel(conn, username, password)

	return nil
}

// connectTunnel handles the actual connection process
func (m *MultiManager) connectTunnel(conn *tunnelConn, username, password string) {
	// TODO: Implement actual connection logic
	// 1. Generate WireGuard keypair
	// 2. Login to tunnel server
	// 3. Create WireGuard interface (utun0, utun1, etc.)
	// 4. Configure WireGuard peer
	// 5. Start WebSocket tunnel
	// 6. Apply routes

	// For now, just simulate connection
	time.Sleep(time.Second)

	m.mu.Lock()
	conn.State = StateConnected
	conn.ConnectedAt = time.Now()
	m.mu.Unlock()
}

// Disconnect disconnects from a specific tunnel
func (m *MultiManager) Disconnect(tunnelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[tunnelID]
	if !exists {
		return nil
	}

	// Signal stop
	close(conn.stopCh)

	// TODO: Cleanup
	// 1. Stop WebSocket tunnel
	// 2. Remove WireGuard interface
	// 3. Cleanup routes

	conn.State = StateDisconnected
	delete(m.connections, tunnelID)

	return nil
}

// DisconnectAll disconnects from all tunnels
func (m *MultiManager) DisconnectAll() {
	m.mu.Lock()
	tunnelIDs := make([]string, 0, len(m.connections))
	for id := range m.connections {
		tunnelIDs = append(tunnelIDs, id)
	}
	m.mu.Unlock()

	for _, id := range tunnelIDs {
		m.Disconnect(id)
	}
}

// GetStatus returns status of all connections
func (m *MultiManager) GetStatus() MultiTunnelStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := MultiTunnelStatus{
		Connections: make([]TunnelConnection, 0, len(m.connections)),
	}

	for _, conn := range m.connections {
		status.Connections = append(status.Connections, conn.TunnelConnection)
		status.TotalRx += conn.RxBytes
		status.TotalTx += conn.TxBytes
	}

	return status
}

// GetConnection returns status of a specific connection
func (m *MultiManager) GetConnection(tunnelID string) *TunnelConnection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if conn, exists := m.connections[tunnelID]; exists {
		c := conn.TunnelConnection
		return &c
	}
	return nil
}

// IsConnected returns true if connected to any tunnel
func (m *MultiManager) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, conn := range m.connections {
		if conn.State == StateConnected {
			return true
		}
	}
	return false
}

// ConnectedCount returns number of connected tunnels
func (m *MultiManager) ConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, conn := range m.connections {
		if conn.State == StateConnected {
			count++
		}
	}
	return count
}
