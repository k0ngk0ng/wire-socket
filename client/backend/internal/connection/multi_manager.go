package connection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"wire-socket-client/internal/wireguard"
	"wire-socket-client/internal/wstunnel"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// TunnelInfo represents a tunnel from the auth service
type TunnelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Region      string `json:"region"`
	InternalURL string `json:"internal_url"` // API endpoint
}

// AuthSession holds the authenticated session with auth service
type AuthSession struct {
	AuthURL   string       `json:"auth_url"`
	Token     string       `json:"token"`
	Expires   int64        `json:"expires"`
	UserID    uint         `json:"user_id"`
	Username  string       `json:"username"`
	IsAdmin   bool         `json:"is_admin"`
	Tunnels   []TunnelInfo `json:"tunnels"`
	Password  string       `json:"-"` // Kept for tunnel login, not serialized
}

// TunnelConnection represents a single tunnel connection
type TunnelConnection struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Region      string    `json:"region"`
	State       State     `json:"state"`
	AssignedIP  string    `json:"assigned_ip,omitempty"`
	ConnectedAt time.Time `json:"connected_at,omitempty"`
	RxBytes     uint64    `json:"rx_bytes"`
	TxBytes     uint64    `json:"tx_bytes"`
	Error       string    `json:"error,omitempty"`
}

// MultiTunnelStatus represents status of all tunnel connections
type MultiTunnelStatus struct {
	Authenticated bool               `json:"authenticated"`
	AuthURL       string             `json:"auth_url,omitempty"`
	Username      string             `json:"username,omitempty"`
	Tunnels       []TunnelInfo       `json:"tunnels,omitempty"`       // Available tunnels
	Connections   []TunnelConnection `json:"connections"`             // Active connections
	TotalRx       uint64             `json:"total_rx"`
	TotalTx       uint64             `json:"total_tx"`
}

// MultiManager manages authentication and multiple tunnel connections
type MultiManager struct {
	mu           sync.RWMutex
	session      *AuthSession
	connections  map[string]*tunnelConn
	configPath   string
	interfaceIdx int32
}

// tunnelConn is internal connection state
type tunnelConn struct {
	TunnelConnection
	wgInterface  *wireguard.Interface
	tunnelClient *wstunnel.Client
	stopCh       chan struct{}
	privateKey   wgtypes.Key
	serverPubKey string
}

// NewMultiManager creates a new multi-tunnel manager
func NewMultiManager(configPath string) *MultiManager {
	return &MultiManager{
		connections: make(map[string]*tunnelConn),
		configPath:  configPath,
	}
}

// LoginRequest for authenticating with auth service
type LoginRequest struct {
	AuthURL  string `json:"auth_url"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login authenticates with the auth service and gets available tunnels
func (m *MultiManager) Login(req LoginRequest) (*AuthSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Call auth service login
	loginReq := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{
		Username: req.Username,
		Password: req.Password,
	}

	jsonBody, _ := json.Marshal(loginReq)
	apiURL := fmt.Sprintf("%s/api/auth/login", req.AuthURL)

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to auth service: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("%s", errResp.Error)
		}
		return nil, fmt.Errorf("login failed: status %d", resp.StatusCode)
	}

	var loginResp struct {
		Token    string       `json:"token"`
		Expires  int64        `json:"expires"`
		UserID   uint         `json:"user_id"`
		Username string       `json:"username"`
		IsAdmin  bool         `json:"is_admin"`
		Tunnels  []TunnelInfo `json:"tunnels"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	// Store session
	m.session = &AuthSession{
		AuthURL:  req.AuthURL,
		Token:    loginResp.Token,
		Expires:  loginResp.Expires,
		UserID:   loginResp.UserID,
		Username: loginResp.Username,
		IsAdmin:  loginResp.IsAdmin,
		Tunnels:  loginResp.Tunnels,
		Password: req.Password, // Keep for tunnel login
	}

	return m.session, nil
}

// Logout clears the session and disconnects all tunnels
func (m *MultiManager) Logout() {
	m.DisconnectAll()
	m.mu.Lock()
	m.session = nil
	m.mu.Unlock()
}

// GetSession returns the current auth session
func (m *MultiManager) GetSession() *AuthSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.session
}

// TunnelConnectRequest for connecting to a specific tunnel
type TunnelConnectRequest struct {
	TunnelID string `json:"tunnel_id"`
}

// Connect connects to a specific tunnel using the authenticated session
func (m *MultiManager) Connect(req TunnelConnectRequest) error {
	m.mu.Lock()

	if m.session == nil {
		m.mu.Unlock()
		return fmt.Errorf("not authenticated - login first")
	}

	// Find tunnel info
	var tunnelInfo *TunnelInfo
	for _, t := range m.session.Tunnels {
		if t.ID == req.TunnelID {
			tunnelInfo = &t
			break
		}
	}

	if tunnelInfo == nil {
		m.mu.Unlock()
		return fmt.Errorf("tunnel %s not found or not accessible", req.TunnelID)
	}

	// Check if already connected
	if conn, exists := m.connections[req.TunnelID]; exists {
		if conn.State == StateConnected || conn.State == StateConnecting {
			m.mu.Unlock()
			return nil
		}
	}

	// Create new connection
	conn := &tunnelConn{
		TunnelConnection: TunnelConnection{
			ID:     tunnelInfo.ID,
			Name:   tunnelInfo.Name,
			URL:    tunnelInfo.URL,
			Region: tunnelInfo.Region,
			State:  StateConnecting,
		},
		stopCh: make(chan struct{}),
	}

	m.connections[req.TunnelID] = conn

	// Get credentials for connection
	username := m.session.Username
	password := m.session.Password
	internalURL := tunnelInfo.InternalURL

	m.mu.Unlock()

	// Start connection in background
	go m.connectTunnel(conn, internalURL, username, password)

	return nil
}

// connectTunnel handles the actual connection process
func (m *MultiManager) connectTunnel(conn *tunnelConn, internalURL, username, password string) {
	// 1. Generate WireGuard keypair
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		m.setError(conn, fmt.Sprintf("failed to generate key: %v", err))
		return
	}
	conn.privateKey = privateKey
	publicKey := privateKey.PublicKey().String()

	// 2. Login to tunnel server
	loginReq := struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		PublicKey string `json:"public_key"`
	}{
		Username:  username,
		Password:  password,
		PublicKey: publicKey,
	}

	jsonBody, _ := json.Marshal(loginReq)
	apiURL := fmt.Sprintf("%s/api/auth/login", internalURL)

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		m.setError(conn, fmt.Sprintf("tunnel login failed: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		m.setError(conn, fmt.Sprintf("tunnel login failed: status %d", resp.StatusCode))
		return
	}

	var loginResp struct {
		Interface struct {
			Address string   `json:"address"`
			DNS     []string `json:"dns"`
		} `json:"interface"`
		Peer struct {
			PublicKey  string   `json:"public_key"`
			Endpoint   string   `json:"endpoint"`
			AllowedIPs []string `json:"allowed_ips"`
		} `json:"peer"`
		TunnelURL string `json:"tunnel_url"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		m.setError(conn, fmt.Sprintf("failed to parse response: %v", err))
		return
	}

	conn.AssignedIP = loginResp.Interface.Address
	conn.URL = loginResp.TunnelURL
	conn.serverPubKey = loginResp.Peer.PublicKey

	// 3. Create WireGuard interface
	interfaceName := m.nextInterfaceName()
	wgInterface, err := wireguard.NewInterface(interfaceName)
	if err != nil {
		m.setError(conn, fmt.Sprintf("failed to create interface: %v", err))
		return
	}
	conn.wgInterface = wgInterface

	// 4. Configure WireGuard with peer
	allowedIPs := "0.0.0.0/0"
	if len(loginResp.Peer.AllowedIPs) > 0 {
		allowedIPs = joinStrings(loginResp.Peer.AllowedIPs, ",")
	}

	dns := ""
	if len(loginResp.Interface.DNS) > 0 {
		dns = joinStrings(loginResp.Interface.DNS, ",")
	}

	wgConfig := &wireguard.WGConfig{
		PrivateKey: privateKey.String(),
		Address:    conn.AssignedIP,
		DNS:        dns,
		Peer: wireguard.PeerConfig{
			PublicKey:  loginResp.Peer.PublicKey,
			Endpoint:   loginResp.Peer.Endpoint,
			AllowedIPs: allowedIPs,
		},
	}

	if err := wgInterface.Configure(wgConfig); err != nil {
		wgInterface.Destroy()
		m.setError(conn, fmt.Sprintf("failed to configure interface: %v", err))
		return
	}

	// 5. Start WebSocket tunnel
	tunnelClient := wstunnel.NewClient(wstunnel.Config{
		ServerURL: loginResp.TunnelURL,
		LocalAddr: "127.0.0.1:0",
	})

	if err := tunnelClient.Start(); err != nil {
		wgInterface.Destroy()
		m.setError(conn, fmt.Sprintf("failed to start tunnel: %v", err))
		return
	}
	conn.tunnelClient = tunnelClient

	// 6. Mark as connected
	m.mu.Lock()
	conn.State = StateConnected
	conn.ConnectedAt = time.Now()
	m.mu.Unlock()

	// Start stats collection
	go m.collectStats(conn)
}

// nextInterfaceName generates the next interface name
func (m *MultiManager) nextInterfaceName() string {
	idx := atomic.AddInt32(&m.interfaceIdx, 1) - 1
	if runtime.GOOS == "darwin" {
		return fmt.Sprintf("utun%d", 10+idx)
	}
	return fmt.Sprintf("wg%d", idx)
}

// setError sets connection error state
func (m *MultiManager) setError(conn *tunnelConn, errMsg string) {
	m.mu.Lock()
	conn.State = StateFailed
	conn.Error = errMsg
	m.mu.Unlock()
}

// collectStats periodically collects traffic stats
func (m *MultiManager) collectStats(conn *tunnelConn) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-conn.stopCh:
			return
		case <-ticker.C:
			if conn.wgInterface != nil {
				stats, err := conn.wgInterface.GetStats()
				if err == nil {
					m.mu.Lock()
					conn.RxBytes = stats.RxBytes
					conn.TxBytes = stats.TxBytes
					m.mu.Unlock()
				}
			}
		}
	}
}

// Disconnect disconnects from a specific tunnel
func (m *MultiManager) Disconnect(tunnelID string) error {
	m.mu.Lock()
	conn, exists := m.connections[tunnelID]
	if !exists {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	// Signal stop
	select {
	case <-conn.stopCh:
	default:
		close(conn.stopCh)
	}

	// Cleanup
	if conn.tunnelClient != nil {
		conn.tunnelClient.Stop()
	}
	if conn.wgInterface != nil {
		conn.wgInterface.Destroy()
	}

	m.mu.Lock()
	conn.State = StateDisconnected
	delete(m.connections, tunnelID)
	m.mu.Unlock()

	return nil
}

// DisconnectAll disconnects from all tunnels
func (m *MultiManager) DisconnectAll() {
	m.mu.RLock()
	tunnelIDs := make([]string, 0, len(m.connections))
	for id := range m.connections {
		tunnelIDs = append(tunnelIDs, id)
	}
	m.mu.RUnlock()

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

	if m.session != nil {
		status.Authenticated = true
		status.AuthURL = m.session.AuthURL
		status.Username = m.session.Username
		status.Tunnels = m.session.Tunnels
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

// Helper function
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
