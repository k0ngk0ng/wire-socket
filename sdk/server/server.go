// Package server provides the core VPN server functionality.
// This includes WireGuard server management, peer handling, and configuration.
package server

import (
	"fmt"
	"sync"

	"github.com/k0ngk0ng/wire-socket/sdk/wireguard"
)

// Config contains the server configuration.
type Config struct {
	// DeviceName is the WireGuard interface name (e.g., "wg0")
	DeviceName string

	// ListenPort is the UDP port for WireGuard
	ListenPort int

	// Address is the server's VPN address in CIDR notation (e.g., "10.0.0.1/24")
	Address string

	// PrivateKey is the server's WireGuard private key (base64)
	// If empty, a new key pair will be generated
	PrivateKey string

	// Mode is the WireGuard operation mode ("kernel" or "userspace")
	Mode wireguard.Mode

	// Subnet is the VPN subnet for IP allocation (e.g., "10.0.0.0/24")
	Subnet string

	// DNS is the DNS server to push to clients
	DNS string
}

// DefaultConfig returns a default server configuration.
func DefaultConfig() Config {
	return Config{
		DeviceName: "wg0",
		ListenPort: 51820,
		Address:    "10.0.0.1/24",
		Mode:       wireguard.ModeUserspace,
		Subnet:     "10.0.0.0/24",
		DNS:        "1.1.1.1",
	}
}

// Server represents a WireGuard VPN server.
type Server struct {
	mu      sync.RWMutex
	config  Config
	backend wireguard.ServerBackend
	peers   map[string]*Peer // publicKey -> Peer
	running bool

	// Server keys
	privateKey string
	publicKey  string
}

// Peer represents a connected VPN peer.
type Peer struct {
	PublicKey  string
	AllowedIP  string
	AssignedIP string
}

// New creates a new VPN server with the given configuration.
func New(cfg Config) (*Server, error) {
	if cfg.DeviceName == "" {
		cfg.DeviceName = "wg0"
	}
	if cfg.ListenPort == 0 {
		cfg.ListenPort = 51820
	}
	if cfg.Mode == "" {
		cfg.Mode = wireguard.ModeUserspace
	}

	return &Server{
		config: cfg,
		peers:  make(map[string]*Peer),
	}, nil
}

// Start initializes and starts the WireGuard server.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("server already running")
	}

	// Generate or use provided keys
	privateKey := s.config.PrivateKey
	if privateKey == "" {
		var err error
		privateKey, s.publicKey, err = wireguard.GenerateKeyPair()
		if err != nil {
			return fmt.Errorf("failed to generate key pair: %w", err)
		}
	} else {
		// Derive public key from private key
		// For now, we'll need to get this from configuration or generate
		s.publicKey = "" // Will be set by backend after configure
	}
	s.privateKey = privateKey

	// Create backend
	var backend wireguard.ServerBackend
	var err error

	switch s.config.Mode {
	case wireguard.ModeUserspace:
		backend, err = wireguard.NewUserspaceBackend(wireguard.UserspaceConfig{
			InterfaceName: s.config.DeviceName,
			MTU:           1420,
		})
	case wireguard.ModeKernel:
		backend, err = wireguard.NewKernelBackend(wireguard.KernelConfig{
			InterfaceName: s.config.DeviceName,
		})
	default:
		return fmt.Errorf("unknown mode: %s", s.config.Mode)
	}

	if err != nil {
		return fmt.Errorf("failed to create backend: %w", err)
	}

	// Configure backend
	err = backend.Configure(wireguard.Config{
		PrivateKey: privateKey,
		Address:    s.config.Address,
		ListenPort: s.config.ListenPort,
	})
	if err != nil {
		backend.Close()
		return fmt.Errorf("failed to configure backend: %w", err)
	}

	// Get public key from backend
	s.publicKey = backend.GetPublicKey()
	s.backend = backend
	s.running = true

	return nil
}

// Stop shuts down the WireGuard server.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if s.backend != nil {
		if err := s.backend.Close(); err != nil {
			return fmt.Errorf("failed to close backend: %w", err)
		}
		s.backend = nil
	}

	s.running = false
	return nil
}

// IsRunning returns whether the server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// GetPublicKey returns the server's public key.
func (s *Server) GetPublicKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publicKey
}

// GetListenPort returns the server's listen port.
func (s *Server) GetListenPort() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.backend != nil {
		return s.backend.GetListenPort()
	}
	return s.config.ListenPort
}

// AddPeer adds a new peer to the server.
func (s *Server) AddPeer(publicKey, allowedIP string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("server not running")
	}

	err := s.backend.AddPeer(wireguard.PeerConfig{
		PublicKey:  publicKey,
		AllowedIPs: []string{allowedIP},
	})
	if err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	s.peers[publicKey] = &Peer{
		PublicKey:  publicKey,
		AllowedIP:  allowedIP,
		AssignedIP: allowedIP,
	}

	return nil
}

// RemovePeer removes a peer from the server.
func (s *Server) RemovePeer(publicKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return fmt.Errorf("server not running")
	}

	if err := s.backend.RemovePeer(publicKey); err != nil {
		return fmt.Errorf("failed to remove peer: %w", err)
	}

	delete(s.peers, publicKey)
	return nil
}

// GetPeers returns all connected peers.
func (s *Server) GetPeers() []*Peer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]*Peer, 0, len(s.peers))
	for _, p := range s.peers {
		peers = append(peers, p)
	}
	return peers
}

// GetPeerStats returns statistics for all peers.
func (s *Server) GetPeerStats() ([]wireguard.PeerStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.running || s.backend == nil {
		return nil, fmt.Errorf("server not running")
	}

	return s.backend.GetPeerStats()
}

// GetConfig returns the server configuration.
func (s *Server) GetConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}
