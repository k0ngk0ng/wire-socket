package mobile

// Version is the SDK version, can be overridden at build time with:
//   -ldflags="-X github.com/k0ngk0ng/wire-socket/sdk/mobile.Version=x.y.z"
var Version = "dev"

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
