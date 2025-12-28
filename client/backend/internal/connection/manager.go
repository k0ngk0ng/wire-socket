package connection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	ServerAddress string `json:"server_address"` // API address (e.g., "192.168.1.100:8080")
	TunnelURL     string `json:"tunnel_url"`     // Tunnel URL (e.g., "https://vpn.example.com/tunnel")
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
	wgConfig, token, tunnelURL, routes, err := m.authenticate(req)
	if err != nil {
		m.setError(fmt.Errorf("authentication failed: %w", err))
		return
	}

	m.token = token
	m.assignedIP = wgConfig.Address

	// Step 2: Start wstunnel client (built-in)
	// Use tunnel URL from server response, fallback to request's TunnelURL or ServerAddress
	wsURL := tunnelURL
	if wsURL == "" {
		wsURL, err = buildWebSocketURL(req.TunnelURL, req.ServerAddress)
		if err != nil {
			m.setError(fmt.Errorf("invalid tunnel URL: %w", err))
			return
		}
	}

	wstunnelClient := wstunnel.NewClient(wstunnel.Config{
		LocalAddr: "127.0.0.1:51820",
		ServerURL: wsURL,
		Insecure:  true, // TODO: Add proper TLS verification
	})

	if err := wstunnelClient.Start(); err != nil {
		m.setError(fmt.Errorf("failed to start tunnel client: %w", err))
		return
	}

	m.wstunnelClient = wstunnelClient

	// Give wstunnel a moment to start
	time.Sleep(1 * time.Second)

	// Step 3: Create and configure WireGuard interface
	wgInterface, err := wireguard.NewInterface("")
	if err != nil {
		wstunnelClient.Stop()
		m.setError(fmt.Errorf("failed to create WireGuard interface: %w", err))
		return
	}

	// Set endpoint to localhost (wstunnel)
	wgConfig.Peer.Endpoint = "127.0.0.1:51820"

	// Use routes from server if provided, otherwise use AllowedIPs
	if len(routes) > 0 {
		wgConfig.Peer.AllowedIPs = strings.Join(routes, ",")
		fmt.Printf("Using routes from server: %v\n", routes)
	}

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
func (m *Manager) authenticate(req ConnectRequest) (*wireguard.WGConfig, string, string, []string, error) {
	// Build API URL - normalize the server address
	apiBase := normalizeServerURL(req.ServerAddress)
	apiURL := apiBase + "/api/auth/login"

	// Prepare login request
	loginData := map[string]interface{}{
		"username": req.Username,
		"password": req.Password,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return nil, "", "", nil, err
	}

	// Send login request
	resp, err := http.Post(apiURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, "", "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", "", nil, fmt.Errorf("authentication failed with status: %d", resp.StatusCode)
	}

	// Parse response
	var loginResp struct {
		Token string `json:"token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, "", "", nil, err
	}

	// Get WireGuard config
	configURL := apiBase + "/api/config"
	configReq, _ := http.NewRequest("GET", configURL, nil)
	configReq.Header.Set("Authorization", "Bearer "+loginResp.Token)

	client := &http.Client{}
	configResp, err := client.Do(configReq)
	if err != nil {
		return nil, "", "", nil, err
	}
	defer configResp.Body.Close()

	if configResp.StatusCode != http.StatusOK {
		return nil, "", "", nil, fmt.Errorf("failed to get config with status: %d", configResp.StatusCode)
	}

	var configData struct {
		Config    wireguard.WGConfig `json:"config"`
		TunnelURL string             `json:"tunnel_url"`
		Routes    []string           `json:"routes"`
	}

	if err := json.NewDecoder(configResp.Body).Decode(&configData); err != nil {
		return nil, "", "", nil, err
	}

	return &configData.Config, loginResp.Token, configData.TunnelURL, configData.Routes, nil
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

// normalizeServerURL normalizes a server address to a full URL
// Supported formats:
//   - https://example.com -> https://example.com
//   - http://example.com -> http://example.com
//   - example.com -> https://example.com (defaults to https)
//   - example.com:8080 -> http://example.com:8080 (non-standard port defaults to http)
//   - 192.168.1.100:8080 -> http://192.168.1.100:8080
func normalizeServerURL(addr string) string {
	// Remove trailing slash
	addr = strings.TrimSuffix(addr, "/")

	// If already has scheme, return as-is
	if strings.HasPrefix(addr, "https://") || strings.HasPrefix(addr, "http://") {
		return addr
	}

	// Check if it has a port
	parts := splitHostPort(addr)
	if len(parts) == 2 {
		port := parts[1]
		// Standard HTTPS port or no port -> use https
		if port == "443" {
			return "https://" + parts[0]
		}
		// Non-standard port -> use http
		return "http://" + addr
	}

	// No port specified -> default to https
	return "https://" + addr
}

// getConfigDir returns the configuration directory path
func getConfigDir() (string, error) {
	// When running as a system service (root), use a system-level directory
	// On macOS/Linux, running as root means os.Geteuid() == 0
	if os.Geteuid() == 0 {
		switch runtime.GOOS {
		case "darwin":
			return "/var/lib/wiresocket", nil
		case "linux":
			return "/var/lib/wiresocket", nil
		case "windows":
			return filepath.Join(os.Getenv("ProgramData"), "WireSocket"), nil
		}
	}

	// For regular user, use home directory
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to system directory
		return "/var/lib/wiresocket", nil
	}

	return filepath.Join(home, ".wire-socket"), nil
}

// splitHostPort splits a host:port string
func splitHostPort(addr string) []string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return []string{addr[:i], addr[i+1:]}
		}
	}
	return []string{addr}
}

// buildWebSocketURL converts a tunnel URL (https:// or http://) to WebSocket URL (wss:// or ws://)
// If tunnelURL is empty, it falls back to using serverAddress with default port 443
// Supported formats:
//   - https://example.com/tunnel -> wss://example.com/tunnel
//   - http://example.com/tunnel -> ws://example.com/tunnel
//   - wss://example.com/tunnel -> wss://example.com/tunnel (unchanged)
//   - example.com/tunnel -> wss://example.com/tunnel
//   - example.com:8443/tunnel -> wss://example.com:8443/tunnel
func buildWebSocketURL(tunnelURL, serverAddress string) (string, error) {
	// If tunnel URL is provided, use it
	if tunnelURL != "" {
		return convertToWebSocketURL(tunnelURL)
	}

	// Fall back to server address
	if serverAddress == "" {
		return "", errors.New("either tunnel_url or server_address must be provided")
	}

	// Use the server address as tunnel URL (convert to WebSocket)
	return convertToWebSocketURL(serverAddress)
}

// convertToWebSocketURL converts http(s) URLs to ws(s) URLs
func convertToWebSocketURL(rawURL string) (string, error) {
	// Handle URLs without scheme
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	// Convert scheme
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
		// Already a WebSocket URL
	default:
		return "", fmt.Errorf("unsupported URL scheme: %s", parsed.Scheme)
	}

	// Ensure port is set for wss
	if parsed.Port() == "" {
		if parsed.Scheme == "wss" {
			parsed.Host = parsed.Host + ":443"
		} else {
			parsed.Host = parsed.Host + ":80"
		}
	}

	return parsed.String(), nil
}
