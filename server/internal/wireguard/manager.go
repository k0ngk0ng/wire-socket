package wireguard

import (
	"fmt"
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Manager handles WireGuard server operations
type Manager struct {
	client     *wgctrl.Client
	deviceName string
}

// NewManager creates a new WireGuard manager
func NewManager(deviceName string) (*Manager, error) {
	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create wgctrl client: %w", err)
	}

	return &Manager{
		client:     client,
		deviceName: deviceName,
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

	return nil
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
