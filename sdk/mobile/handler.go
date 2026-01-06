// Package mobile provides gomobile-compatible bindings for the WireSocket SDK.
// This package wraps the core SDK with interfaces and types that work with
// iOS (via .framework) and Android (via .aar).
package mobile

// EventHandler is the interface for receiving VPN events.
// Implement this interface in Swift/Kotlin to receive callbacks.
type EventHandler interface {
	// OnEvent is called when a VPN event occurs.
	// eventType: "connecting", "connected", "disconnected", "reconnecting", "error", "stats_updated"
	// jsonData: JSON-encoded event data
	OnEvent(eventType string, jsonData string)
}

// LogHandler is the interface for receiving log messages.
// Implement this interface to capture SDK log output.
type LogHandler interface {
	// OnLog is called when the SDK logs a message.
	// level: "debug", "info", "warn", "error"
	// message: the log message
	OnLog(level string, message string)
}

// TunnelHandler is the interface for platform-specific tunnel operations.
// This is used for integrating with iOS NEPacketTunnelProvider or Android VpnService.
type TunnelHandler interface {
	// OnTunnelConfigured is called when WireGuard config is ready.
	// The platform should set up the TUN interface with this config.
	// configJSON contains: privateKey, address, dns, peerPublicKey, peerEndpoint, allowedIPs
	OnTunnelConfigured(configJSON string) error

	// OnTunnelStopped is called when the tunnel should be torn down.
	OnTunnelStopped()
}
