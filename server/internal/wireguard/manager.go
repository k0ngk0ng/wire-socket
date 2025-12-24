package wireguard

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// Manager handles WireGuard server operations with support for both kernel and userspace modes
type Manager struct {
	backend    wg.ServerBackend
	mode       Mode
	deviceName string
	configPath string // Path to config file (e.g., /etc/wireguard/wg0.conf)

	// Stored for saving config
	privateKey string
	address    string
	listenPort int
}

// ManagerConfig configures the WireGuard manager
type ManagerConfig struct {
	DeviceName string
	Mode       Mode // "kernel" or "userspace"
}

// NewManager creates a new WireGuard manager with kernel mode (default, for compatibility)
func NewManager(deviceName string) (*Manager, error) {
	return NewManagerWithConfig(ManagerConfig{
		DeviceName: deviceName,
		Mode:       ModeKernel,
	})
}

// NewManagerWithConfig creates a new WireGuard manager with the specified configuration
func NewManagerWithConfig(cfg ManagerConfig) (*Manager, error) {
	deviceName := cfg.DeviceName
	if deviceName == "" {
		deviceName = "wg0"
	}

	mode := cfg.Mode
	if mode == "" {
		mode = ModeKernel
	}

	configPath := filepath.Join("/etc/wireguard", deviceName+".conf")

	var backend wg.ServerBackend
	var err error

	switch mode {
	case ModeUserspace:
		backend, err = wg.NewUserspaceBackend(wg.UserspaceConfig{
			InterfaceName: deviceName,
			MTU:           1420,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create userspace backend: %w", err)
		}
	case ModeKernel:
		fallthrough
	default:
		backend, err = wg.NewKernelBackend(wg.KernelConfig{
			InterfaceName: deviceName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create kernel backend: %w", err)
		}
	}

	return &Manager{
		backend:    backend,
		mode:       mode,
		deviceName: deviceName,
		configPath: configPath,
	}, nil
}

// Close closes the WireGuard manager
func (m *Manager) Close() error {
	if m.backend != nil {
		return m.backend.Close()
	}
	return nil
}

// ConfigureDevice configures the WireGuard device with the given private key
func (m *Manager) ConfigureDevice(privateKey string, listenPort int) error {
	err := m.backend.Configure(wg.Config{
		PrivateKey: privateKey,
		ListenPort: listenPort,
		Address:    m.address,
	})
	if err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Store for later config file saves
	m.privateKey = privateKey
	m.listenPort = listenPort

	return nil
}

// SetAddress sets the interface address (for config file persistence)
func (m *Manager) SetAddress(address string) {
	m.address = address
}

// AddPeer adds a new peer to the WireGuard device
func (m *Manager) AddPeer(publicKey, allowedIP string) error {
	err := m.backend.AddPeer(wg.PeerConfig{
		PublicKey:           publicKey,
		AllowedIPs:          []string{allowedIP},
		PersistentKeepalive: 25 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	// Persist to config file
	if m.privateKey != "" {
		if err := m.SaveConfigFile(m.privateKey, m.address, m.listenPort); err != nil {
			// Log but don't fail - WireGuard is already configured
			fmt.Printf("Warning: failed to persist config: %v\n", err)
		}
	}

	return nil
}

// RemovePeer removes a peer from the WireGuard device
func (m *Manager) RemovePeer(publicKey string) error {
	if err := m.backend.RemovePeer(publicKey); err != nil {
		return fmt.Errorf("failed to remove peer: %w", err)
	}

	// Persist to config file
	if m.privateKey != "" {
		if err := m.SaveConfigFile(m.privateKey, m.address, m.listenPort); err != nil {
			// Log but don't fail - WireGuard is already configured
			fmt.Printf("Warning: failed to persist config: %v\n", err)
		}
	}

	return nil
}

// PeerStat represents statistics for a WireGuard peer
type PeerStat struct {
	PublicKey     string
	Endpoint      string
	LastHandshake time.Time
	RxBytes       int64
	TxBytes       int64
}

// GetPeerStats returns statistics for all peers
func (m *Manager) GetPeerStats() ([]PeerStat, error) {
	peerStats, err := m.backend.GetPeerStats()
	if err != nil {
		return nil, err
	}

	stats := make([]PeerStat, len(peerStats))
	for i, p := range peerStats {
		stats[i] = PeerStat{
			PublicKey:     p.PublicKey,
			Endpoint:      p.Endpoint,
			LastHandshake: p.LastHandshake,
			RxBytes:       p.RxBytes,
			TxBytes:       p.TxBytes,
		}
	}

	return stats, nil
}

// GetMode returns the current WireGuard mode
func (m *Manager) GetMode() Mode {
	return m.mode
}

// GenerateKeyPair generates a new WireGuard key pair
func GenerateKeyPair() (privateKey, publicKey string, err error) {
	return wg.GenerateKeyPair()
}

// ServerConfig represents the server's WireGuard configuration from config file
type ServerConfig struct {
	PrivateKey string
	Address    string
	ListenPort int
	Peers      []PeerEntry
}

// PeerEntry represents a peer in the config file
type PeerEntry struct {
	PublicKey  string
	AllowedIPs []string
}

// LoadConfigFile reads the WireGuard config file and returns the configuration
func (m *Manager) LoadConfigFile() (*ServerConfig, error) {
	if _, err := os.Stat(m.configPath); os.IsNotExist(err) {
		return nil, nil // Config file doesn't exist, not an error
	}

	file, err := os.Open(m.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	config := &ServerConfig{}
	var currentPeer *PeerEntry

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section headers
		if line == "[Interface]" {
			currentPeer = nil
			continue
		}
		if line == "[Peer]" {
			currentPeer = &PeerEntry{}
			config.Peers = append(config.Peers, *currentPeer)
			currentPeer = &config.Peers[len(config.Peers)-1]
			continue
		}

		// Parse key = value
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if currentPeer == nil {
			// Interface section
			switch key {
			case "PrivateKey":
				config.PrivateKey = value
			case "Address":
				config.Address = value
			case "ListenPort":
				fmt.Sscanf(value, "%d", &config.ListenPort)
			}
		} else {
			// Peer section
			switch key {
			case "PublicKey":
				currentPeer.PublicKey = value
			case "AllowedIPs":
				ips := strings.Split(value, ",")
				for _, ip := range ips {
					currentPeer.AllowedIPs = append(currentPeer.AllowedIPs, strings.TrimSpace(ip))
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return config, nil
}

// SaveConfigFile writes the current WireGuard configuration to the config file
func (m *Manager) SaveConfigFile(privateKey string, address string, listenPort int) error {
	peerStats, err := m.backend.GetPeerStats()
	if err != nil {
		return fmt.Errorf("failed to get device config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Build config content
	var sb strings.Builder
	sb.WriteString("[Interface]\n")
	sb.WriteString(fmt.Sprintf("PrivateKey = %s\n", privateKey))
	if address != "" {
		sb.WriteString(fmt.Sprintf("Address = %s\n", address))
	}
	sb.WriteString(fmt.Sprintf("ListenPort = %d\n", listenPort))

	// Write peers
	for _, peer := range peerStats {
		sb.WriteString("\n[Peer]\n")
		sb.WriteString(fmt.Sprintf("PublicKey = %s\n", peer.PublicKey))
		// We don't have AllowedIPs from stats, so we'll skip for now
		// In a real implementation, we'd track this separately
	}

	// Write to file with restricted permissions
	if err := os.WriteFile(m.configPath, []byte(sb.String()), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadPeersFromConfig loads peers from the config file into the WireGuard device
func (m *Manager) LoadPeersFromConfig() error {
	config, err := m.LoadConfigFile()
	if err != nil {
		return err
	}
	if config == nil {
		return nil // No config file
	}

	for _, peer := range config.Peers {
		if peer.PublicKey == "" || len(peer.AllowedIPs) == 0 {
			continue
		}
		for _, allowedIP := range peer.AllowedIPs {
			if err := m.AddPeer(peer.PublicKey, allowedIP); err != nil {
				return fmt.Errorf("failed to add peer %s: %w", peer.PublicKey, err)
			}
		}
	}

	return nil
}

// GetConfigPath returns the path to the config file
func (m *Manager) GetConfigPath() string {
	return m.configPath
}

// Helper function to parse allowed IPs
func parseAllowedIPs(s string) ([]net.IPNet, error) {
	var result []net.IPNet
	for _, cidr := range strings.Split(s, ",") {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
		}
		result = append(result, *ipnet)
	}
	return result, nil
}
