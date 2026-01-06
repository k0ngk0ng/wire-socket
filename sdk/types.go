// Package sdk provides a Go SDK for building WireSocket VPN clients.
// It allows developers to create custom UIs while leveraging the core VPN functionality.
package sdk

import (
	"time"
)

// State represents the VPN connection state
type State string

const (
	StateDisconnected State = "disconnected"
	StateConnecting   State = "connecting"
	StateConnected    State = "connected"
	StateFailed       State = "failed"
	StateReconnecting State = "reconnecting"
)

// ConnectConfig contains connection parameters
type ConnectConfig struct {
	// Server is the VPN server address (e.g., "https://vpn.example.com")
	Server string

	// Username for authentication
	Username string

	// Password for authentication
	Password string

	// ExcludedRoutes is a list of CIDRs to exclude from VPN tunnel
	ExcludedRoutes []string

	// AutoReconnect enables automatic reconnection on connection loss
	AutoReconnect bool

	// ReconnectInterval is the initial interval between reconnection attempts
	ReconnectInterval time.Duration

	// MaxReconnectInterval is the maximum interval between reconnection attempts
	MaxReconnectInterval time.Duration
}

// Status represents the current VPN connection status
type Status struct {
	// State is the current connection state
	State State

	// Server is the connected server address
	Server string

	// AssignedIP is the IP address assigned by the VPN server
	AssignedIP string

	// ConnectedAt is when the connection was established
	ConnectedAt time.Time

	// Duration is how long the connection has been active
	Duration time.Duration

	// Error contains error message if State is StateFailed
	Error string
}

// Stats contains traffic statistics
type Stats struct {
	// RxBytes is total bytes received
	RxBytes uint64

	// TxBytes is total bytes transmitted
	TxBytes uint64

	// RxSpeed is current receive speed in bytes/sec
	RxSpeed uint64

	// TxSpeed is current transmit speed in bytes/sec
	TxSpeed uint64
}

// ServerInfo contains information about a saved server
type ServerInfo struct {
	// Address is the server address
	Address string

	// Username is the saved username
	Username string

	// LastConnected is when the server was last connected
	LastConnected time.Time
}

// RouteInfo contains route information
type RouteInfo struct {
	// AvailableRoutes is the list of routes provided by server
	AvailableRoutes []string

	// ExcludedRoutes is the list of routes excluded by user
	ExcludedRoutes []string

	// ActiveRoutes is the list of routes actually applied
	ActiveRoutes []string
}

// Event types for the event system
type EventType string

const (
	EventConnecting    EventType = "connecting"
	EventConnected     EventType = "connected"
	EventDisconnected  EventType = "disconnected"
	EventReconnecting  EventType = "reconnecting"
	EventError         EventType = "error"
	EventStatsUpdated  EventType = "stats_updated"
	EventRoutesChanged EventType = "routes_changed"
)

// Event represents a VPN event
type Event struct {
	// Type is the event type
	Type EventType

	// Timestamp is when the event occurred
	Timestamp time.Time

	// Status is the current status (available for most events)
	Status *Status

	// Stats is traffic stats (available for EventStatsUpdated)
	Stats *Stats

	// Error is error message (available for EventError)
	Error error
}

// EventHandler is a callback function for handling events
type EventHandler func(event Event)

// Options contains SDK configuration options
type Options struct {
	// ConfigDir is the directory for storing configuration files
	// Default: ~/.wire-socket (user) or /var/lib/wire-socket (root)
	ConfigDir string

	// StatsInterval is how often to update traffic stats
	// Default: 3 seconds
	StatsInterval time.Duration

	// Logger is a custom logger function
	// Default: log.Printf
	Logger func(format string, args ...interface{})

	// Debug enables debug logging
	Debug bool
}

// DefaultOptions returns default SDK options
func DefaultOptions() Options {
	return Options{
		StatsInterval: 3 * time.Second,
		Debug:         false,
	}
}

// DefaultConnectConfig returns default connection config
func DefaultConnectConfig() ConnectConfig {
	return ConnectConfig{
		AutoReconnect:        true,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectInterval: 60 * time.Second,
	}
}
