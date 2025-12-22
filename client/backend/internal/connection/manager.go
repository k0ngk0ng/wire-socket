package connection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"wire-socket-client/internal/wireguard"
	"wire-socket-client/internal/wstunnel"
)

// State represents the connection state
type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
	StateFailed       State = "failed"
)

// ServerConfig represents a saved server configuration
type ServerConfig struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"` // Server API address
	Username    string    `json:"username"`
	LastUsed    time.Time `json:"last_used,omitempty"`
}

// ConnectRequest represents connection parameters
type ConnectRequest struct {
	ServerAddress string `json:"server_address"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

// Status represents the current connection status
type Status struct {
	State         State     `json:"state"`
	ServerName    string    `json:"server_name,omitempty"`
	AssignedIP    string    `json:"assigned_ip,omitempty"`
	PublicIP      string    `json:"public_ip,omitempty"`
	ConnectedSince time.Time `json:"connected_since,omitempty"`
	RxBytes       uint64    `json:"rx_bytes"`
	TxBytes       uint64    `json:"tx_bytes"`
	RxSpeed       uint64    `json:"rx_speed"` // bytes/sec
	TxSpeed       uint64    `json:"tx_speed"`
	Latency       int       `json:"latency"` // ms
	Error         string    `json:"error,omitempty"`
}

// Manager manages VPN connections
type Manager struct {
	mu            sync.RWMutex
	state         State
	currentServer *ServerConfig
	wgInterface   *wireguard.Interface
	wstunnelClient *wstunnel.Client
	token         string
	assignedIP    string
	connectedAt   time.Time
	lastError     error
	servers       []ServerConfig
	configPath    string
}

// NewManager creates a new connection manager
func NewManager() (*Manager, error) {
	// Determine config directory
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "servers.json")

	m := &Manager{
		state:      StateDisconnected,
		configPath: configPath,
	}

	// Load saved servers
	if err := m.loadServers(); err != nil {
		// It's okay if file doesn't exist yet
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load servers: %w", err)
		}
	}

	return m, nil
}

// Connect initiates a VPN connection
func (m *Manager) Connect(req ConnectRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == StateConnected || m.state == StateConnecting {
		return errors.New("already connected or connecting")
	}

	m.state = StateConnecting
	m.lastError = nil

	// Perform connection in background
	go m.doConnect(req)

	return nil
}

func (m *Manager) doConnect(req ConnectRequest) {
	// Step 1: Authenticate with server and get WireGuard config
	wgConfig, token, err := m.authenticate(req)
	if err != nil {
		m.setError(fmt.Errorf("authentication failed: %w", err))
		return
	}

	m.token = token
	m.assignedIP = wgConfig.Address

	// Step 2: Start wstunnel client
	wstunnelClient, err := wstunnel.NewClient(req.ServerAddress, 51820)
	if err != nil {
		m.setError(fmt.Errorf("failed to create wstunnel client: %w", err))
		return
	}

	if err := wstunnelClient.Start(); err != nil {
		m.setError(fmt.Errorf("failed to start wstunnel: %w", err))
		return
	}

	m.wstunnelClient = wstunnelClient

	// Give wstunnel a moment to start
	time.Sleep(1 * time.Second)

	// Step 3: Create and configure WireGuard interface
	wgInterface, err := wireguard.NewInterface("wg-vpn")
	if err != nil {
		wstunnelClient.Stop()
		m.setError(fmt.Errorf("failed to create WireGuard interface: %w", err))
		return
	}

	// Set endpoint to localhost (wstunnel)
	wgConfig.Peer.Endpoint = "127.0.0.1:51820"

	if err := wgInterface.Configure(wgConfig); err != nil {
		wstunnelClient.Stop()
		wgInterface.Destroy()
		m.setError(fmt.Errorf("failed to configure WireGuard: %w", err))
		return
	}

	m.wgInterface = wgInterface

	// Step 4: Mark as connected
	m.mu.Lock()
	m.state = StateConnected
	m.connectedAt = time.Now()
	m.currentServer = &ServerConfig{
		Name:     req.ServerAddress,
		Address:  req.ServerAddress,
		Username: req.Username,
		LastUsed: time.Now(),
	}
	m.mu.Unlock()

	// Save server config
	m.saveServer(*m.currentServer)

	fmt.Println("VPN connected successfully!")
}

// Disconnect closes the VPN connection
func (m *Manager) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state == StateDisconnected {
		return nil
	}

	// Stop wstunnel
	if m.wstunnelClient != nil {
		m.wstunnelClient.Stop()
		m.wstunnelClient = nil
	}

	// Destroy WireGuard interface
	if m.wgInterface != nil {
		m.wgInterface.Destroy()
		m.wgInterface = nil
	}

	m.state = StateDisconnected
	m.currentServer = nil
	m.token = ""
	m.assignedIP = ""

	return nil
}

// GetStatus returns the current connection status
func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := Status{
		State: m.state,
	}

	if m.currentServer != nil {
		status.ServerName = m.currentServer.Name
	}

	if m.state == StateConnected {
		status.AssignedIP = m.assignedIP
		status.ConnectedSince = m.connectedAt

		// Get traffic stats from WireGuard
		if m.wgInterface != nil {
			stats, err := m.wgInterface.GetStats()
			if err == nil {
				status.RxBytes = stats.RxBytes
				status.TxBytes = stats.TxBytes
				status.RxSpeed = stats.RxSpeed
				status.TxSpeed = stats.TxSpeed
			}
		}
	}

	if m.lastError != nil {
		status.Error = m.lastError.Error()
	}

	return status
}

// authenticate performs authentication with the server
func (m *Manager) authenticate(req ConnectRequest) (*wireguard.WGConfig, string, error) {
	// Build API URL
	apiURL := fmt.Sprintf("https://%s/api/auth/login", req.ServerAddress)
	if req.ServerAddress[:4] != "http" {
		apiURL = fmt.Sprintf("http://%s/api/auth/login", req.ServerAddress)
	}

	// Prepare login request
	loginData := map[string]interface{}{
		"username": req.Username,
		"password": req.Password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return nil, "", err
	}

	// Send login request
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("authentication failed with status: %d", resp.StatusCode)
	}

	// Parse response
	var loginResp struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, "", err
	}

	// Get WireGuard config
	configURL := fmt.Sprintf("http://%s/api/config", req.ServerAddress)
	configReq, _ := http.NewRequest("GET", configURL, nil)
	configReq.Header.Set("Authorization", "Bearer "+loginResp.Token)

	client := &http.Client{}
	configResp, err := client.Do(configReq)
	if err != nil {
		return nil, "", err
	}
	defer configResp.Body.Close()

	if configResp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("failed to get config with status: %d", configResp.StatusCode)
	}

	var configData struct {
		Config wireguard.WGConfig `json:"config"`
	}

	if err := json.NewDecoder(configResp.Body).Decode(&configData); err != nil {
		return nil, "", err
	}

	return &configData.Config, loginResp.Token, nil
}

func (m *Manager) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.state = StateFailed
	m.lastError = err
	fmt.Printf("Connection error: %v\n", err)
}

// Server management functions

func (m *Manager) GetServers() []ServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return append([]ServerConfig{}, m.servers...)
}

func (m *Manager) saveServer(server ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Add or update server
	found := false
	for i, s := range m.servers {
		if s.Address == server.Address && s.Username == server.Username {
			m.servers[i] = server
			found = true
			break
		}
	}

	if !found {
		m.servers = append(m.servers, server)
	}

	return m.saveServersLocked()
}

func (m *Manager) loadServers() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &m.servers)
}

func (m *Manager) saveServersLocked() error {
	data, err := json.Marshal(m.servers)
	if err != nil {
		return err
	}

	return os.WriteFile(m.configPath, data, 0600)
}

func (m *Manager) Close() {
	m.Disconnect()
}

// getConfigDir returns the configuration directory path
func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".vpn-client"), nil
}
