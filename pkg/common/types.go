// Package common provides shared types and utilities
package common

import "time"

// VerifyRequest is sent from tunnel to auth for user verification
type VerifyRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TunnelID string `json:"tunnel_id"`
}

// VerifyResponse is returned from auth to tunnel
type VerifyResponse struct {
	Valid          bool     `json:"valid"`
	UserID         uint     `json:"user_id"`
	Username       string   `json:"username"`
	AllowedTunnels []string `json:"allowed_tunnels"` // Tunnel IDs user can access
	Error          string   `json:"error,omitempty"`
}

// TunnelInfo represents a registered tunnel node
type TunnelInfo struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`          // Public URL for clients
	InternalURL string    `json:"internal_url"` // Internal URL for auth communication
	Region      string    `json:"region"`
	Status      string    `json:"status"` // online, offline
	LastSeen    time.Time `json:"last_seen"`
}

// TunnelRegisterRequest is sent from tunnel to auth for registration
type TunnelRegisterRequest struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	InternalURL string `json:"internal_url"`
	Region      string `json:"region"`
	Token       string `json:"token"` // Pre-shared secret
}

// TunnelHeartbeat is sent periodically from tunnel to auth
type TunnelHeartbeat struct {
	ID             string `json:"id"`
	ConnectedPeers int    `json:"connected_peers"`
	Uptime         int64  `json:"uptime_seconds"`
}

// ClientTunnelConfig is returned to client after login
type ClientTunnelConfig struct {
	Interface InterfaceConfig `json:"interface"`
	Peer      PeerConfig      `json:"peer"`
	TunnelURL string          `json:"tunnel_url"`
}

// InterfaceConfig is the client WireGuard interface config
type InterfaceConfig struct {
	Address    string   `json:"address"`
	PrivateKey string   `json:"private_key,omitempty"` // Client generates this
	DNS        []string `json:"dns,omitempty"`
}

// PeerConfig is the server WireGuard peer config
type PeerConfig struct {
	PublicKey  string   `json:"public_key"`
	Endpoint   string   `json:"endpoint"`
	AllowedIPs []string `json:"allowed_ips"`
}
