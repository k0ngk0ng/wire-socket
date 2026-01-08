// Package wireguard provides WireGuard functionality with support for both
// kernel-mode and userspace implementations.
package wireguard

import (
	"fmt"
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Mode represents the WireGuard operation mode
type Mode string

const (
	// ModeKernel uses the kernel WireGuard implementation (requires wireguard-tools)
	ModeKernel Mode = "kernel"
	// ModeUserspace uses the pure Go userspace implementation (no dependencies)
	ModeUserspace Mode = "userspace"
)

// Config represents WireGuard interface configuration
type Config struct {
	PrivateKey string
	Address    string // CIDR notation, e.g., "10.0.0.1/24"
	ListenPort int
	DNS        string
	MTU        int
}

// PeerConfig represents a WireGuard peer configuration
type PeerConfig struct {
	PublicKey           string
	Endpoint            string // host:port
	AllowedIPs          []string
	PersistentKeepalive time.Duration
}

// Stats represents traffic statistics
type Stats struct {
	RxBytes uint64
	TxBytes uint64
	RxSpeed uint64 // bytes/sec
	TxSpeed uint64 // bytes/sec
}

// PeerStats represents statistics for a peer
type PeerStats struct {
	PublicKey     string
	Endpoint      string
	LastHandshake time.Time
	RxBytes       int64
	TxBytes       int64
}

// Backend defines the interface for WireGuard implementations
type Backend interface {
	// Configure sets up the WireGuard interface
	Configure(cfg Config) error

	// AddPeer adds a peer to the interface
	AddPeer(peer PeerConfig) error

	// RemovePeer removes a peer by public key
	RemovePeer(publicKey string) error

	// GetStats returns traffic statistics
	GetStats() (Stats, error)

	// GetPeerStats returns statistics for all peers
	GetPeerStats() ([]PeerStats, error)

	// GetPublicKey returns the public key derived from the private key
	GetPublicKey() string

	// Close shuts down the WireGuard interface
	Close() error
}

// ServerBackend extends Backend with server-specific functionality
type ServerBackend interface {
	Backend

	// GetListenPort returns the UDP listen port
	GetListenPort() int

	// GetDeviceName returns the interface name
	GetDeviceName() string
}

// ClientBackend extends Backend with client-specific functionality
type ClientBackend interface {
	Backend

	// SetRoutes configures routing for the VPN
	SetRoutes(routes []net.IPNet) error
}

// ParseCIDR parses a CIDR string into IP and network
func ParseCIDR(cidr string) (net.IP, *net.IPNet, error) {
	return net.ParseCIDR(cidr)
}

// GenerateKeyPair generates a new WireGuard key pair
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	privKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate private key: %w", err)
	}

	pubKey := privKey.PublicKey()

	return privKey.String(), pubKey.String(), nil
}
