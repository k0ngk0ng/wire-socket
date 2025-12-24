package wireguard

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Manager handles WireGuard server operations
type Manager struct {
	client     *wgctrl.Client
	deviceName string
	configPath string // Path to config file (e.g., /etc/wireguard/wg0.conf)

	// Stored for saving config
	privateKey string
	address    string
	listenPort int
}

// NewManager creates a new WireGuard manager
func NewManager(deviceName string) (*Manager, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create wgctrl client: %w", err)
	}

	configPath := filepath.Join("/etc/wireguard", deviceName+".conf")

	return &Manager{
		client:     client,
		deviceName: deviceName,
		configPath: configPath,
	}, nil
}

// Close closes the WireGuard client
func (m *Manager) Close() error {
	return m.client.Close()
}

// ConfigureDevice configures the WireGuard device with the given private key
func (m *Manager) ConfigureDevice(privateKey string, listenPort int) error {
	key, err := wgtypes.ParseKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	port := listenPort
	config := wgtypes.Config{
		PrivateKey: &key,
		ListenPort: &port,
	}

	if err := m.client.ConfigureDevice(m.deviceName, config); err != nil {
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
	peerKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse peer public key: %w", err)
	}

	// Parse allowed IP (e.g., 10.0.0.5/32)
	_, ipNet, err := net.ParseCIDR(allowedIP)
	if err != nil {
		return fmt.Errorf("failed to parse allowed IP: %w", err)
	}

	keepalive := 25 * time.Second

	peerConfig := wgtypes.PeerConfig{
		PublicKey:                   peerKey,
		AllowedIPs:                  []net.IPNet{*ipNet},
		PersistentKeepaliveInterval: &keepalive,
	}

	config := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerConfig},
	}

	if err := m.client.ConfigureDevice(m.deviceName, config); err != nil {
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
	peerKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse peer public key: %w", err)
	}

	peerConfig := wgtypes.PeerConfig{
		PublicKey: peerKey,
		Remove:    true,
	}

	config := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerConfig},
	}

	if err := m.client.ConfigureDevice(m.deviceName, config); err != nil {
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

// GetDevice returns the current WireGuard device configuration
func (m *Manager) GetDevice() (*wgtypes.Device, error) {
	device, err := m.client.Device(m.deviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return device, nil
}

// GetPeerStats returns statistics for all peers
func (m *Manager) GetPeerStats() ([]PeerStat, error) {
	device, err := m.GetDevice()
	if err != nil {
		return nil, err
	}

	stats := make([]PeerStat, 0, len(device.Peers))
	for _, peer := range device.Peers {
		stats = append(stats, PeerStat{
			PublicKey:     peer.PublicKey.String(),
			Endpoint:      peer.Endpoint.String(),
			LastHandshake: peer.LastHandshakeTime,
			RxBytes:       peer.ReceiveBytes,
			TxBytes:       peer.TransmitBytes,
		})
	}

	return stats, nil
}

// PeerStat represents statistics for a WireGuard peer
type PeerStat struct {
	PublicKey     string
	Endpoint      string
	LastHandshake time.Time
	RxBytes       int64
	TxBytes       int64
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
	device, err := m.GetDevice()
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
	for _, peer := range device.Peers {
		sb.WriteString("\n[Peer]\n")
		sb.WriteString(fmt.Sprintf("PublicKey = %s\n", peer.PublicKey.String()))
		if len(peer.AllowedIPs) > 0 {
			ips := make([]string, len(peer.AllowedIPs))
			for i, ip := range peer.AllowedIPs {
				ips[i] = ip.String()
			}
			sb.WriteString(fmt.Sprintf("AllowedIPs = %s\n", strings.Join(ips, ", ")))
		}
		if peer.PersistentKeepaliveInterval > 0 {
			sb.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", int(peer.PersistentKeepaliveInterval.Seconds())))
		}
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
