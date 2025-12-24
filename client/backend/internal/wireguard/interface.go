package wireguard

import (
	"fmt"
	"net"
	"time"

	wg "wire-socket/pkg/wireguard"
)

// Mode represents the WireGuard operation mode
type Mode = wg.Mode

const (
	// ModeKernel uses kernel WireGuard
	ModeKernel = wg.ModeKernel
	// ModeUserspace uses pure Go userspace WireGuard
	ModeUserspace = wg.ModeUserspace
)

// WGConfig represents a WireGuard configuration
type WGConfig struct {
	PrivateKey string     `json:"private_key"`
	Address    string     `json:"address"`
	DNS        string     `json:"dns"`
	Peer       PeerConfig `json:"peer"`
}

// PeerConfig represents peer configuration
type PeerConfig struct {
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint"`
	AllowedIPs string `json:"allowed_ips"`
}

// Interface represents a WireGuard interface
type Interface struct {
	Name    string
	backend wg.ClientBackend
	mode    Mode
	stats   Stats
}

// Stats represents traffic statistics
type Stats struct {
	RxBytes uint64
	TxBytes uint64
	RxSpeed uint64 // bytes/sec
	TxSpeed uint64 // bytes/sec
}

// InterfaceConfig configures the WireGuard interface
type InterfaceConfig struct {
	Name string
	Mode Mode // "kernel" or "userspace"
}

// NewInterface creates a new WireGuard interface with kernel mode (default, for compatibility)
func NewInterface(name string) (*Interface, error) {
	return NewInterfaceWithConfig(InterfaceConfig{
		Name: name,
		Mode: ModeUserspace, // Default to userspace for client (no WireGuard installation required)
	})
}

// NewInterfaceWithConfig creates a new WireGuard interface with the specified configuration
func NewInterfaceWithConfig(cfg InterfaceConfig) (*Interface, error) {
	name := cfg.Name
	if name == "" {
		name = "wg-vpn"
	}

	mode := cfg.Mode
	if mode == "" {
		mode = ModeUserspace
	}

	var backend wg.ClientBackend
	var err error

	switch mode {
	case ModeUserspace:
		backend, err = wg.NewUserspaceBackend(wg.UserspaceConfig{
			InterfaceName: name,
			MTU:           1420,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create userspace backend: %w", err)
		}
	case ModeKernel:
		backend, err = wg.NewKernelBackend(wg.KernelConfig{
			InterfaceName: name,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kernel backend: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown mode: %s", mode)
	}

	return &Interface{
		Name:    name,
		backend: backend,
		mode:    mode,
	}, nil
}

// Configure configures the WireGuard interface
func (i *Interface) Configure(config *WGConfig) error {
	// Configure the backend
	err := i.backend.Configure(wg.Config{
		PrivateKey: config.PrivateKey,
		Address:    config.Address,
		DNS:        config.DNS,
	})
	if err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Parse allowed IPs
	allowedIPs, err := parseAllowedIPs(config.Peer.AllowedIPs)
	if err != nil {
		return fmt.Errorf("failed to parse allowed IPs: %w", err)
	}

	// Add peer
	err = i.backend.AddPeer(wg.PeerConfig{
		PublicKey:           config.Peer.PublicKey,
		Endpoint:            config.Peer.Endpoint,
		AllowedIPs:          allowedIPs,
		PersistentKeepalive: 25 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	// Set up routes for the allowed IPs
	var routes []net.IPNet
	for _, ip := range allowedIPs {
		_, ipNet, err := net.ParseCIDR(ip)
		if err != nil {
			continue
		}
		routes = append(routes, *ipNet)
	}

	if len(routes) > 0 {
		if err := i.backend.SetRoutes(routes); err != nil {
			// Log but don't fail - routing may need elevated privileges
			fmt.Printf("Warning: failed to set routes: %v\n", err)
		}
	}

	return nil
}

// GetStats returns current traffic statistics
func (i *Interface) GetStats() (Stats, error) {
	stats, err := i.backend.GetStats()
	if err != nil {
		return Stats{}, err
	}

	return Stats{
		RxBytes: stats.RxBytes,
		TxBytes: stats.TxBytes,
		RxSpeed: stats.RxSpeed,
		TxSpeed: stats.TxSpeed,
	}, nil
}

// GetMode returns the current WireGuard mode
func (i *Interface) GetMode() Mode {
	return i.mode
}

// Destroy removes the WireGuard interface
func (i *Interface) Destroy() error {
	if i.backend != nil {
		return i.backend.Close()
	}
	return nil
}

// parseAllowedIPs parses a comma-separated list of CIDR notations
func parseAllowedIPs(s string) ([]string, error) {
	var result []string
	for _, cidr := range splitAndTrim(s, ",") {
		if cidr == "" {
			continue
		}
		// Validate CIDR
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		result = append(result, cidr)
	}
	return result, nil
}

// splitAndTrim splits a string and trims whitespace from each part
func splitAndTrim(s, sep string) []string {
	var result []string
	for i := 0; i < len(s); {
		j := i
		for j < len(s) && string(s[j]) != sep {
			j++
		}
		part := s[i:j]
		// Trim whitespace
		start, end := 0, len(part)
		for start < end && (part[start] == ' ' || part[start] == '\t') {
			start++
		}
		for end > start && (part[end-1] == ' ' || part[end-1] == '\t') {
			end--
		}
		if start < end {
			result = append(result, part[start:end])
		}
		i = j + 1
	}
	return result
}
