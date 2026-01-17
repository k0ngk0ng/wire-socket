// Package sdkadapter provides an adapter that wraps the SDK client
// to provide backward compatibility with the existing API interface.
package sdkadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	sdk "github.com/k0ngk0ng/wire-socket/sdk"
)

// State represents the connection state (matches SDK states)
type State = sdk.State

const (
	StateDisconnected = sdk.StateDisconnected
	StateConnecting   = sdk.StateConnecting
	StateConnected    = sdk.StateConnected
	StateFailed       = sdk.StateFailed
)

// ServerConfig represents a saved server configuration
type ServerConfig struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Address  string    `json:"address"`
	Username string    `json:"username"`
	LastUsed time.Time `json:"last_used,omitempty"`
}

// RouteSettings stores user's route preferences
type RouteSettings struct {
	ExcludedRoutes []string `json:"excluded_routes"`
}

// ConnectRequest represents connection parameters
type ConnectRequest struct {
	ServerAddress string `json:"server_address"`
	TunnelURL     string `json:"tunnel_url"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

// Status represents the current connection status
type Status struct {
	State           State     `json:"state"`
	ServerName      string    `json:"server_name,omitempty"`
	AssignedIP      string    `json:"assigned_ip,omitempty"`
	PublicIP        string    `json:"public_ip,omitempty"`
	ConnectedSince  time.Time `json:"connected_since,omitempty"`
	RxBytes         uint64    `json:"rx_bytes"`
	TxBytes         uint64    `json:"tx_bytes"`
	RxSpeed         uint64    `json:"rx_speed"`
	TxSpeed         uint64    `json:"tx_speed"`
	Latency         int       `json:"latency"`
	Error           string    `json:"error,omitempty"`
	AvailableRoutes []string  `json:"available_routes,omitempty"`
	ActiveRoutes    []string  `json:"active_routes,omitempty"`
	Token           string    `json:"token,omitempty"`
}

// ChangePasswordRequest represents the change password request
type ChangePasswordRequest struct {
	ServerAddress   string `json:"server_address"`
	Token           string `json:"token"`
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// Manager wraps the SDK client and provides backward-compatible interface
type Manager struct {
	client *sdk.Client
	mu     sync.RWMutex

	// Current connection info
	currentServer *ServerConfig
	lastRequest   *ConnectRequest

	// Saved servers
	servers    []ServerConfig
	configPath string
}

// NewManager creates a new SDK-backed manager
func NewManager() (*Manager, error) {
	// Create SDK client with default options
	client, err := sdk.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create SDK client: %w", err)
	}

	// Determine config directory
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	m := &Manager{
		client:     client,
		configPath: filepath.Join(configDir, "servers.json"),
	}

	// Load saved servers
	m.loadServers()

	return m, nil
}

// Connect initiates a VPN connection
func (m *Manager) Connect(req ConnectRequest) error {
	m.mu.Lock()
	m.lastRequest = &req
	m.currentServer = &ServerConfig{
		Name:     req.ServerAddress,
		Address:  req.ServerAddress,
		Username: req.Username,
		LastUsed: time.Now(),
	}
	m.mu.Unlock()

	// Convert to SDK config
	config := sdk.DefaultConnectConfig()
	config.Server = req.ServerAddress
	config.Username = req.Username
	config.Password = req.Password
	config.AutoReconnect = true

	err := m.client.Connect(config)
	if err != nil {
		return err
	}

	// Save server after successful connection initiation
	go func() {
		// Wait for connection to complete
		for i := 0; i < 60; i++ {
			time.Sleep(500 * time.Millisecond)
			if m.client.IsConnected() {
				m.mu.RLock()
				serverCopy := m.currentServer
				m.mu.RUnlock()
				if serverCopy != nil {
					m.saveServer(*serverCopy)
				}
				break
			}
			if m.client.GetState() == sdk.StateFailed {
				break
			}
		}
	}()

	return nil
}

// Disconnect closes the VPN connection
func (m *Manager) Disconnect() error {
	m.mu.Lock()
	m.currentServer = nil
	m.lastRequest = nil
	m.mu.Unlock()

	return m.client.Disconnect()
}

// GetStatus returns the current connection status
func (m *Manager) GetStatus() Status {
	sdkStatus := m.client.GetStatus()
	sdkStats := m.client.GetStats()
	sdkRoutes := m.client.GetRoutes()

	m.mu.RLock()
	var serverName string
	if m.currentServer != nil {
		serverName = m.currentServer.Name
	}
	m.mu.RUnlock()

	status := Status{
		State:           sdkStatus.State,
		ServerName:      serverName,
		AssignedIP:      sdkStatus.AssignedIP,
		ConnectedSince:  sdkStatus.ConnectedAt,
		RxBytes:         sdkStats.RxBytes,
		TxBytes:         sdkStats.TxBytes,
		RxSpeed:         sdkStats.RxSpeed,
		TxSpeed:         sdkStats.TxSpeed,
		AvailableRoutes: sdkRoutes.AvailableRoutes,
		ActiveRoutes:    sdkRoutes.ActiveRoutes,
		Error:           sdkStatus.Error,
	}

	return status
}

// GetServers returns saved server configurations
func (m *Manager) GetServers() []ServerConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]ServerConfig{}, m.servers...)
}

// GetRouteSettings returns current route settings
func (m *Manager) GetRouteSettings() RouteSettings {
	routes := m.client.GetRoutes()
	return RouteSettings{
		ExcludedRoutes: routes.ExcludedRoutes,
	}
}

// SetExcludedRoutes sets routes to exclude from VPN
func (m *Manager) SetExcludedRoutes(excluded []string) error {
	return m.client.SetExcludedRoutes(excluded)
}

// GetAvailableRoutes returns routes from server
func (m *Manager) GetAvailableRoutes() []string {
	return m.client.GetRoutes().AvailableRoutes
}

// GetActiveRoutes returns currently active routes
func (m *Manager) GetActiveRoutes() []string {
	return m.client.GetRoutes().ActiveRoutes
}

// ChangePassword changes the user's password on the server
func (m *Manager) ChangePassword(req ChangePasswordRequest) error {
	apiBase := normalizeServerURL(req.ServerAddress)
	apiURL := apiBase + "/api/auth/change-password"

	body := map[string]string{
		"current_password": req.CurrentPassword,
		"new_password":     req.NewPassword,
	}

	jsonData, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+req.Token)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if result.Error != "" {
			return fmt.Errorf("%s", result.Error)
		}
		return fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Close cleans up resources
func (m *Manager) Close() {
	m.client.Close()
}

// OnEvent registers an event handler (expose SDK functionality)
func (m *Manager) OnEvent(handler sdk.EventHandler) {
	m.client.OnEvent(handler)
}

// GetSDKClient returns the underlying SDK client for advanced usage
func (m *Manager) GetSDKClient() *sdk.Client {
	return m.client
}

// ======== Internal helpers ========

func (m *Manager) saveServer(server ServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

func getConfigDir() (string, error) {
	if os.Geteuid() == 0 {
		switch runtime.GOOS {
		case "darwin", "linux":
			return "/var/lib/wire-socket", nil
		case "windows":
			return filepath.Join(os.Getenv("ProgramData"), "WireSocket"), nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "/var/lib/wire-socket", nil
	}

	return filepath.Join(home, ".wire-socket"), nil
}

func normalizeServerURL(addr string) string {
	addr = trimSuffix(addr, "/")

	if hasPrefix(addr, "https://") || hasPrefix(addr, "http://") {
		return addr
	}

	if contains(addr, ":") {
		parts := split(addr, ":")
		if len(parts) == 2 && parts[1] != "443" {
			return "http://" + addr
		}
	}

	return "https://" + addr
}

// Simple string helpers to avoid importing strings package
func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func split(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+len(sep)] == sep {
			result = append(result, s[start:i])
			start = i + len(sep)
			i = start - 1
		}
	}
	result = append(result, s[start:])
	return result
}
