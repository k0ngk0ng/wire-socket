package mobile

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	sdk "github.com/k0ngk0ng/wire-socket/sdk"
)

// Client is the mobile-friendly VPN client.
// Use NewClient() to create an instance.
type Client struct {
	inner        *sdk.Client
	eventHandler EventHandler
	logHandler   LogHandler
	mu           sync.RWMutex
}

// NewClient creates a new mobile VPN client.
func NewClient() (*Client, error) {
	return NewClientWithConfig("")
}

// NewClientWithConfig creates a new mobile VPN client with custom config directory.
func NewClientWithConfig(configDir string) (*Client, error) {
	opts := sdk.DefaultOptions()
	if configDir != "" {
		opts.ConfigDir = configDir
	}

	inner, err := sdk.New(opts)
	if err != nil {
		return nil, err
	}

	c := &Client{
		inner: inner,
	}

	// Set up internal event handler to forward to mobile handler
	inner.OnEvent(c.handleEvent)

	return c, nil
}

// SetEventHandler sets the handler for VPN events.
// This must be called before Connect() to receive all events.
func (c *Client) SetEventHandler(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandler = handler
}

// SetLogHandler sets the handler for log messages.
func (c *Client) SetLogHandler(handler LogHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logHandler = handler
}

// Connect initiates a VPN connection.
// server: VPN server address (e.g., "https://vpn.example.com")
// username: authentication username
// password: authentication password
// autoReconnect: whether to automatically reconnect on connection loss
func (c *Client) Connect(server, username, password string, autoReconnect bool) error {
	config := sdk.DefaultConnectConfig()
	config.Server = server
	config.Username = username
	config.Password = password
	config.AutoReconnect = autoReconnect

	return c.inner.Connect(config)
}

// ConnectWithOptions initiates a VPN connection with additional options.
// optionsJSON: JSON object with optional fields:
//   - excludedRoutes: []string - CIDRs to exclude from VPN
//   - reconnectIntervalMs: int - initial reconnect interval in milliseconds
//   - maxReconnectIntervalMs: int - max reconnect interval in milliseconds
func (c *Client) ConnectWithOptions(server, username, password string, optionsJSON string) error {
	config := sdk.DefaultConnectConfig()
	config.Server = server
	config.Username = username
	config.Password = password

	if optionsJSON != "" {
		var opts struct {
			ExcludedRoutes         []string `json:"excludedRoutes"`
			AutoReconnect          *bool    `json:"autoReconnect"`
			ReconnectIntervalMs    int      `json:"reconnectIntervalMs"`
			MaxReconnectIntervalMs int      `json:"maxReconnectIntervalMs"`
		}
		if err := json.Unmarshal([]byte(optionsJSON), &opts); err != nil {
			return fmt.Errorf("invalid options JSON: %w", err)
		}

		if len(opts.ExcludedRoutes) > 0 {
			config.ExcludedRoutes = opts.ExcludedRoutes
		}
		if opts.AutoReconnect != nil {
			config.AutoReconnect = *opts.AutoReconnect
		}
		if opts.ReconnectIntervalMs > 0 {
			config.ReconnectInterval = time.Duration(opts.ReconnectIntervalMs) * time.Millisecond
		}
		if opts.MaxReconnectIntervalMs > 0 {
			config.MaxReconnectInterval = time.Duration(opts.MaxReconnectIntervalMs) * time.Millisecond
		}
	}

	return c.inner.Connect(config)
}

// Disconnect closes the VPN connection.
func (c *Client) Disconnect() error {
	return c.inner.Disconnect()
}

// GetState returns the current connection state.
// Returns: "disconnected", "connecting", "connected", "failed", "reconnecting"
func (c *Client) GetState() string {
	return string(c.inner.GetState())
}

// IsConnected returns true if VPN is connected.
func (c *Client) IsConnected() bool {
	return c.inner.IsConnected()
}

// GetStatus returns the current status as JSON.
// JSON fields: state, server, assignedIP, connectedAtUnix, durationMs, error
func (c *Client) GetStatus() string {
	status := c.inner.GetStatus()
	result := statusJSON{
		State:           string(status.State),
		Server:          status.Server,
		AssignedIP:      status.AssignedIP,
		ConnectedAtUnix: status.ConnectedAt.Unix(),
		DurationMs:      status.Duration.Milliseconds(),
		Error:           status.Error,
	}

	data, _ := json.Marshal(result)
	return string(data)
}

// GetStats returns traffic statistics as JSON.
// JSON fields: rxBytes, txBytes, rxSpeed, txSpeed
func (c *Client) GetStats() string {
	stats := c.inner.GetStats()
	result := statsJSON{
		RxBytes: int64(stats.RxBytes),
		TxBytes: int64(stats.TxBytes),
		RxSpeed: int64(stats.RxSpeed),
		TxSpeed: int64(stats.TxSpeed),
	}

	data, _ := json.Marshal(result)
	return string(data)
}

// GetRoutes returns route information as JSON.
// JSON fields: availableRoutes, excludedRoutes, activeRoutes (all []string)
func (c *Client) GetRoutes() string {
	routes := c.inner.GetRoutes()
	result := routesJSON{
		AvailableRoutes: routes.AvailableRoutes,
		ExcludedRoutes:  routes.ExcludedRoutes,
		ActiveRoutes:    routes.ActiveRoutes,
	}

	data, _ := json.Marshal(result)
	return string(data)
}

// SetExcludedRoutes sets routes to exclude from VPN tunnel.
// routesJSON: JSON array of CIDR strings, e.g., ["10.0.0.0/8", "192.168.0.0/16"]
func (c *Client) SetExcludedRoutes(routesJSON string) error {
	var routes []string
	if err := json.Unmarshal([]byte(routesJSON), &routes); err != nil {
		return fmt.Errorf("invalid routes JSON: %w", err)
	}
	return c.inner.SetExcludedRoutes(routes)
}

// Close releases all resources.
func (c *Client) Close() error {
	return c.inner.Close()
}

// handleEvent converts SDK events to mobile-friendly format
func (c *Client) handleEvent(event sdk.Event) {
	c.mu.RLock()
	handler := c.eventHandler
	c.mu.RUnlock()

	if handler == nil {
		return
	}

	eventData := eventJSON{
		Type:        string(event.Type),
		TimestampMs: event.Timestamp.UnixMilli(),
	}

	if event.Status != nil {
		eventData.Status = &statusJSON{
			State:           string(event.Status.State),
			Server:          event.Status.Server,
			AssignedIP:      event.Status.AssignedIP,
			ConnectedAtUnix: event.Status.ConnectedAt.Unix(),
			DurationMs:      event.Status.Duration.Milliseconds(),
			Error:           event.Status.Error,
		}
	}

	if event.Stats != nil {
		eventData.Stats = &statsJSON{
			RxBytes: int64(event.Stats.RxBytes),
			TxBytes: int64(event.Stats.TxBytes),
			RxSpeed: int64(event.Stats.RxSpeed),
			TxSpeed: int64(event.Stats.TxSpeed),
		}
	}

	if event.Error != nil {
		eventData.Error = event.Error.Error()
	}

	data, _ := json.Marshal(eventData)
	handler.OnEvent(string(event.Type), string(data))
}

// JSON types for serialization
type statusJSON struct {
	State           string `json:"state"`
	Server          string `json:"server,omitempty"`
	AssignedIP      string `json:"assignedIP,omitempty"`
	ConnectedAtUnix int64  `json:"connectedAtUnix,omitempty"`
	DurationMs      int64  `json:"durationMs,omitempty"`
	Error           string `json:"error,omitempty"`
}

type statsJSON struct {
	RxBytes int64 `json:"rxBytes"`
	TxBytes int64 `json:"txBytes"`
	RxSpeed int64 `json:"rxSpeed"`
	TxSpeed int64 `json:"txSpeed"`
}

type routesJSON struct {
	AvailableRoutes []string `json:"availableRoutes"`
	ExcludedRoutes  []string `json:"excludedRoutes"`
	ActiveRoutes    []string `json:"activeRoutes"`
}

type eventJSON struct {
	Type        string      `json:"type"`
	TimestampMs int64       `json:"timestampMs"`
	Status      *statusJSON `json:"status,omitempty"`
	Stats       *statsJSON  `json:"stats,omitempty"`
	Error       string      `json:"error,omitempty"`
}
