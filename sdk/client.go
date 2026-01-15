package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Client is the main SDK client for WireSocket VPN
type Client struct {
	mu      sync.RWMutex
	options Options

	// Connection state
	state     State
	config    *ConnectConfig
	status    Status
	stats     Stats
	connectedAt time.Time

	// WireGuard and tunnel
	wgBackend    wgBackend
	tunnelClient *tunnelClient

	// Event handling
	eventHandlers []EventHandler
	eventMu       sync.RWMutex

	// Control
	stopChan     chan struct{}
	statsStop    chan struct{}
	reconnecting bool

	// Persistence
	configDir       string
	excludedRoutes  []string
	availableRoutes []string
	activeRoutes    []string

	// Auth
	token         string
	assignedIP    string
	peerPublicKey string
}

// New creates a new SDK client with the given options
func New(opts ...Options) (*Client, error) {
	var options Options
	if len(opts) > 0 {
		options = opts[0]
	} else {
		options = DefaultOptions()
	}

	// Set default logger
	if options.Logger == nil {
		options.Logger = log.Printf
	}

	// Determine config directory
	configDir := options.ConfigDir
	if configDir == "" {
		var err error
		configDir, err = getDefaultConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get config directory: %w", err)
		}
	}

	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	c := &Client{
		options:   options,
		state:     StateDisconnected,
		configDir: configDir,
	}

	// Load saved route settings
	c.loadRouteSettings()

	return c, nil
}

// Connect initiates a VPN connection
func (c *Client) Connect(config ConnectConfig) error {
	c.mu.Lock()
	if c.state == StateConnected || c.state == StateConnecting {
		c.mu.Unlock()
		return fmt.Errorf("already connected or connecting")
	}

	c.state = StateConnecting
	c.config = &config
	c.stopChan = make(chan struct{})
	c.mu.Unlock()

	c.emitEvent(EventConnecting, nil, nil)

	// Perform connection in background
	go c.doConnect(config)

	return nil
}

// Disconnect closes the VPN connection
func (c *Client) Disconnect() error {
	c.mu.Lock()

	if c.state == StateDisconnected {
		c.mu.Unlock()
		return nil
	}

	// Signal stop
	if c.stopChan != nil {
		close(c.stopChan)
		c.stopChan = nil
	}

	// Stop stats collection
	if c.statsStop != nil {
		close(c.statsStop)
		c.statsStop = nil
	}

	// Capture references before unlocking
	tunnel := c.tunnelClient
	wg := c.wgBackend
	c.tunnelClient = nil
	c.wgBackend = nil
	c.state = StateDisconnected
	c.token = ""
	c.assignedIP = ""
	c.config = nil
	c.mu.Unlock()

	// Stop tunnel and WireGuard outside the lock to avoid blocking reads
	if tunnel != nil {
		tunnel.Stop()
	}
	if wg != nil {
		wg.Close()
	}

	c.emitEvent(EventDisconnected, nil, nil)

	return nil
}

// GetStatus returns the current connection status
func (c *Client) GetStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := Status{
		State: c.state,
	}

	if c.config != nil {
		status.Server = c.config.Server
	}

	if c.state == StateConnected {
		status.AssignedIP = c.assignedIP
		status.ConnectedAt = c.connectedAt
		status.Duration = time.Since(c.connectedAt)
	}

	if c.state == StateFailed {
		status.Error = c.status.Error
	}

	return status
}

// GetStats returns traffic statistics
func (c *Client) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// GetRoutes returns current route information
func (c *Client) GetRoutes() RouteInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return RouteInfo{
		AvailableRoutes: append([]string{}, c.availableRoutes...),
		ExcludedRoutes:  append([]string{}, c.excludedRoutes...),
		ActiveRoutes:    append([]string{}, c.activeRoutes...),
	}
}

// SetExcludedRoutes sets routes to exclude from VPN tunnel
func (c *Client) SetExcludedRoutes(routes []string) error {
	c.mu.Lock()
	c.excludedRoutes = routes
	c.mu.Unlock()

	// Save to disk
	if err := c.saveRouteSettings(); err != nil {
		return err
	}

	// Apply if connected
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state == StateConnected {
		return c.applyRoutes()
	}

	return nil
}

// OnEvent registers an event handler
func (c *Client) OnEvent(handler EventHandler) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	c.eventHandlers = append(c.eventHandlers, handler)
}

// IsConnected returns true if VPN is connected
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state == StateConnected
}

// GetState returns the current state
func (c *Client) GetState() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Close cleans up resources
func (c *Client) Close() error {
	return c.Disconnect()
}

// ======== Internal methods ========

func (c *Client) doConnect(config ConnectConfig) {
	// Step 1: Authenticate
	wgConfig, token, tunnelURL, routes, err := c.authenticate(config)
	if err != nil {
		c.setError(fmt.Errorf("authentication failed: %w", err))
		return
	}

	c.mu.Lock()
	c.token = token
	c.assignedIP = wgConfig.Address
	c.peerPublicKey = wgConfig.PeerPublicKey
	c.availableRoutes = routes
	c.mu.Unlock()

	// Step 2: Start tunnel
	wsURL, err := buildWebSocketURL(tunnelURL, config.Server)
	if err != nil {
		c.setError(fmt.Errorf("invalid tunnel URL: %w", err))
		return
	}

	tunnel := newTunnelClient(wsURL, true)
	if err := tunnel.Start(); err != nil {
		c.setError(fmt.Errorf("failed to start tunnel: %w", err))
		return
	}
	c.tunnelClient = tunnel

	// Step 3: Create WireGuard interface
	wg, err := newWGBackend("")
	if err != nil {
		tunnel.Stop()
		c.setError(fmt.Errorf("failed to create WireGuard: %w", err))
		return
	}

	// Configure WireGuard
	wgConfig.PeerEndpoint = fmt.Sprintf("127.0.0.1:%d", tunnel.LocalPort())

	// Apply route exclusions
	activeRoutes := c.filterRoutes(routes)
	if len(activeRoutes) > 0 {
		wgConfig.AllowedIPs = activeRoutes
	}

	c.mu.Lock()
	c.activeRoutes = activeRoutes
	c.mu.Unlock()

	if err := wg.Configure(wgConfig); err != nil {
		tunnel.Stop()
		wg.Close()
		c.setError(fmt.Errorf("failed to configure WireGuard: %w", err))
		return
	}

	c.wgBackend = wg

	// Step 4: Mark connected
	c.mu.Lock()
	c.state = StateConnected
	c.connectedAt = time.Now()
	c.mu.Unlock()

	c.log("VPN connected! Assigned IP: %s", c.assignedIP)
	c.emitEvent(EventConnected, nil, nil)

	// Start stats collection
	c.startStatsCollection()

	// Start connection monitor if auto-reconnect is enabled
	if config.AutoReconnect {
		go c.connectionMonitor(config)
	}
}

func (c *Client) authenticate(config ConnectConfig) (*wgConfig, string, string, []string, error) {
	apiBase := normalizeServerURL(config.Server)

	// Login
	loginURL := apiBase + "/api/auth/login"
	loginData := map[string]string{
		"username": config.Username,
		"password": config.Password,
	}

	jsonData, _ := json.Marshal(loginData)
	resp, err := http.Post(loginURL, "application/json", bytes.NewReader(jsonData))
	if err != nil {
		return nil, "", "", nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", "", nil, fmt.Errorf("login failed with status: %d", resp.StatusCode)
	}

	var loginResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		return nil, "", "", nil, err
	}

	// Get config
	configURL := apiBase + "/api/config"
	req, _ := http.NewRequest("GET", configURL, nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)

	client := &http.Client{}
	configResp, err := client.Do(req)
	if err != nil {
		return nil, "", "", nil, err
	}
	defer configResp.Body.Close()

	if configResp.StatusCode != http.StatusOK {
		return nil, "", "", nil, fmt.Errorf("config failed with status: %d", configResp.StatusCode)
	}

	var configData struct {
		Config struct {
			PrivateKey string `json:"private_key"`
			Address    string `json:"address"`
			DNS        string `json:"dns"`
			Peer       struct {
				PublicKey  string `json:"public_key"`
				AllowedIPs string `json:"allowed_ips"`
				Endpoint   string `json:"endpoint"`
			} `json:"peer"`
		} `json:"config"`
		TunnelURL string   `json:"tunnel_url"`
		Routes    []string `json:"routes"`
	}

	if err := json.NewDecoder(configResp.Body).Decode(&configData); err != nil {
		return nil, "", "", nil, err
	}

	wgCfg := &wgConfig{
		PrivateKey:    configData.Config.PrivateKey,
		Address:       configData.Config.Address,
		DNS:           configData.Config.DNS,
		PeerPublicKey: configData.Config.Peer.PublicKey,
		AllowedIPs:    strings.Split(configData.Config.Peer.AllowedIPs, ","),
	}

	return wgCfg, loginResp.Token, configData.TunnelURL, configData.Routes, nil
}

func (c *Client) setError(err error) {
	c.mu.Lock()
	c.state = StateFailed
	c.status.Error = err.Error()
	c.mu.Unlock()

	c.log("Connection error: %v", err)
	c.emitEvent(EventError, nil, err)
}

func (c *Client) emitEvent(eventType EventType, stats *Stats, err error) {
	c.mu.RLock()
	status := c.GetStatus()
	c.mu.RUnlock()

	c.emitEventWithStatus(eventType, &status, stats, err)
}

func (c *Client) emitEventLocked(eventType EventType, stats *Stats, err error) {
	status := Status{
		State: c.state,
	}
	if c.config != nil {
		status.Server = c.config.Server
	}

	c.emitEventWithStatus(eventType, &status, stats, err)
}

func (c *Client) emitEventWithStatus(eventType EventType, status *Status, stats *Stats, err error) {
	event := Event{
		Type:      eventType,
		Timestamp: time.Now(),
		Status:    status,
		Stats:     stats,
		Error:     err,
	}

	c.eventMu.RLock()
	handlers := append([]EventHandler{}, c.eventHandlers...)
	c.eventMu.RUnlock()

	for _, handler := range handlers {
		go handler(event)
	}
}

func (c *Client) startStatsCollection() {
	c.statsStop = make(chan struct{})
	interval := c.options.StatsInterval
	if interval == 0 {
		interval = 3 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-c.statsStop:
				return
			case <-ticker.C:
				c.updateStats()
			}
		}
	}()
}

func (c *Client) updateStats() {
	c.mu.RLock()
	wg := c.wgBackend
	c.mu.RUnlock()

	if wg == nil {
		return
	}

	stats, err := wg.GetStats()
	if err != nil {
		return
	}

	c.mu.Lock()
	c.stats = Stats{
		RxBytes: stats.RxBytes,
		TxBytes: stats.TxBytes,
		RxSpeed: stats.RxSpeed,
		TxSpeed: stats.TxSpeed,
	}
	statsCopy := c.stats
	c.mu.Unlock()

	c.emitEvent(EventStatsUpdated, &statsCopy, nil)
}

func (c *Client) connectionMonitor(config ConnectConfig) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case <-ticker.C:
			c.mu.RLock()
			state := c.state
			tunnel := c.tunnelClient
			c.mu.RUnlock()

			if state != StateConnected {
				continue
			}

			// Check if tunnel is still running
			if tunnel != nil && !tunnel.IsRunning() {
				c.log("Connection lost, attempting to reconnect...")
				c.handleReconnect(config)
			}
		}
	}
}

func (c *Client) handleReconnect(config ConnectConfig) {
	c.mu.Lock()
	if c.reconnecting {
		c.mu.Unlock()
		return
	}
	c.reconnecting = true
	c.state = StateReconnecting

	// Capture references before unlocking
	tunnel := c.tunnelClient
	wg := c.wgBackend
	c.tunnelClient = nil
	c.wgBackend = nil
	c.mu.Unlock()

	c.emitEvent(EventReconnecting, nil, nil)

	// Cleanup current connection outside the lock
	if tunnel != nil {
		tunnel.Stop()
	}
	if wg != nil {
		wg.Close()
	}

	interval := config.ReconnectInterval
	if interval == 0 {
		interval = 5 * time.Second
	}
	maxInterval := config.MaxReconnectInterval
	if maxInterval == 0 {
		maxInterval = 60 * time.Second
	}

	for {
		select {
		case <-c.stopChan:
			c.mu.Lock()
			c.reconnecting = false
			c.mu.Unlock()
			return
		default:
		}

		c.log("Reconnecting in %v...", interval)
		time.Sleep(interval)

		c.mu.Lock()
		c.state = StateConnecting
		c.mu.Unlock()

		// Try to reconnect
		c.doConnect(config)

		c.mu.RLock()
		state := c.state
		c.mu.RUnlock()

		if state == StateConnected {
			c.mu.Lock()
			c.reconnecting = false
			c.mu.Unlock()
			return
		}

		// Exponential backoff
		interval = interval * 2
		if interval > maxInterval {
			interval = maxInterval
		}
	}
}

func (c *Client) filterRoutes(routes []string) []string {
	c.mu.RLock()
	excluded := make(map[string]bool)
	for _, r := range c.excludedRoutes {
		excluded[r] = true
	}
	c.mu.RUnlock()

	var result []string
	for _, route := range routes {
		if !excluded[route] {
			result = append(result, route)
		}
	}
	return result
}

func (c *Client) applyRoutes() error {
	c.mu.RLock()
	wg := c.wgBackend
	routes := c.availableRoutes
	peerPublicKey := c.peerPublicKey
	c.mu.RUnlock()

	if wg == nil {
		return nil
	}

	activeRoutes := c.filterRoutes(routes)
	if len(activeRoutes) == 0 {
		return nil
	}

	if err := wg.UpdateAllowedIPs(peerPublicKey, activeRoutes); err != nil {
		return err
	}

	c.mu.Lock()
	c.activeRoutes = activeRoutes
	c.mu.Unlock()

	c.emitEvent(EventRoutesChanged, nil, nil)
	return nil
}

func (c *Client) loadRouteSettings() {
	path := filepath.Join(c.configDir, "routes.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var settings struct {
		ExcludedRoutes []string `json:"excluded_routes"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return
	}

	c.excludedRoutes = settings.ExcludedRoutes
}

func (c *Client) saveRouteSettings() error {
	c.mu.RLock()
	settings := struct {
		ExcludedRoutes []string `json:"excluded_routes"`
	}{
		ExcludedRoutes: c.excludedRoutes,
	}
	c.mu.RUnlock()

	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}

	path := filepath.Join(c.configDir, "routes.json")
	return os.WriteFile(path, data, 0600)
}

func (c *Client) log(format string, args ...interface{}) {
	if c.options.Logger != nil {
		c.options.Logger("[WireSocket] "+format, args...)
	}
}

// ======== Helper functions ========

func getDefaultConfigDir() (string, error) {
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
	addr = strings.TrimSuffix(addr, "/")

	if strings.HasPrefix(addr, "https://") || strings.HasPrefix(addr, "http://") {
		return addr
	}

	// Check for port
	if strings.Contains(addr, ":") {
		parts := strings.Split(addr, ":")
		if len(parts) == 2 && parts[1] != "443" {
			return "http://" + addr
		}
	}

	return "https://" + addr
}

func buildWebSocketURL(tunnelURL, serverAddress string) (string, error) {
	rawURL := tunnelURL
	if rawURL == "" {
		rawURL = serverAddress
	}

	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	case "wss", "ws":
		// Already WebSocket
	default:
		return "", fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}

	if parsed.Port() == "" {
		if parsed.Scheme == "wss" {
			parsed.Host = parsed.Host + ":443"
		} else {
			parsed.Host = parsed.Host + ":80"
		}
	}

	return parsed.String(), nil
}

// ======== Internal types ========
// These are implemented in wireguard.go and tunnel.go

type wgConfig struct {
	PrivateKey    string
	Address       string
	DNS           string
	PeerPublicKey string
	PeerEndpoint  string
	AllowedIPs    []string
}

type wgStats struct {
	RxBytes uint64
	TxBytes uint64
	RxSpeed uint64
	TxSpeed uint64
}

type wgBackend interface {
	Configure(cfg *wgConfig) error
	GetStats() (wgStats, error)
	UpdateAllowedIPs(peerPublicKey string, allowedIPs []string) error
	Close() error
}
