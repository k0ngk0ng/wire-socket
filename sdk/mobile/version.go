package mobile

// Version is the SDK version
const Version = "0.8.0"

// GetVersion returns the SDK version string.
func GetVersion() string {
	return Version
}

// State constants for convenience (can be used in switch statements)
const (
	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateFailed       = "failed"
	StateReconnecting = "reconnecting"
)

// Event type constants
const (
	EventConnecting    = "connecting"
	EventConnected     = "connected"
	EventDisconnected  = "disconnected"
	EventReconnecting  = "reconnecting"
	EventError         = "error"
	EventStatsUpdated  = "stats_updated"
	EventRoutesChanged = "routes_changed"
)
