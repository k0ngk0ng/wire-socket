package wireguard

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
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
	Name   string
	client *wgctrl.Client
	stats  Stats
	lastStats Stats
	lastUpdate time.Time
}

// Stats represents traffic statistics
type Stats struct {
	RxBytes uint64
	TxBytes uint64
	RxSpeed uint64 // bytes/sec
	TxSpeed uint64 // bytes/sec
}

// NewInterface creates a new WireGuard interface
func NewInterface(name string) (*Interface, error) {
	// Create interface using platform-specific method
	if err := createInterface(name); err != nil {
		return nil, fmt.Errorf("failed to create interface: %w", err)
	}

	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create wgctrl client: %w", err)
	}

	return &Interface{
		Name:   name,
		client: client,
		lastUpdate: time.Now(),
	}, nil
}

// Configure configures the WireGuard interface
func (i *Interface) Configure(config *WGConfig) error {
	// Parse private key
	privateKey, err := wgtypes.ParseKey(config.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	// Parse peer public key
	peerKey, err := wgtypes.ParseKey(config.Peer.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse peer public key: %w", err)
	}

	// Parse endpoint
	endpoint, err := net.ResolveUDPAddr("udp", config.Peer.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint: %w", err)
	}

	// Parse allowed IPs
	allowedIPs, err := parseAllowedIPs(config.Peer.AllowedIPs)
	if err != nil {
		return fmt.Errorf("failed to parse allowed IPs: %w", err)
	}

	// Configure persistent keepalive
	keepalive := 25 * time.Second

	// Build configuration
	peerConfig := wgtypes.PeerConfig{
		PublicKey:                   peerKey,
		Endpoint:                    &endpoint,
		AllowedIPs:                  allowedIPs,
		PersistentKeepaliveInterval: &keepalive,
	}

	wgConfig := wgtypes.Config{
		PrivateKey: &privateKey,
		Peers:      []wgtypes.PeerConfig{peerConfig},
	}

	// Apply configuration
	if err := i.client.ConfigureDevice(i.Name, wgConfig); err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Set IP address on interface
	if err := setInterfaceAddress(i.Name, config.Address); err != nil {
		return fmt.Errorf("failed to set interface address: %w", err)
	}

	// Bring interface up
	if err := bringInterfaceUp(i.Name); err != nil {
		return fmt.Errorf("failed to bring interface up: %w", err)
	}

	return nil
}

// GetStats returns current traffic statistics
func (i *Interface) GetStats() (Stats, error) {
	device, err := i.client.Device(i.Name)
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get device: %w", err)
	}

	if len(device.Peers) == 0 {
		return Stats{}, fmt.Errorf("no peers configured")
	}

	peer := device.Peers[0]
	now := time.Now()
	elapsed := now.Sub(i.lastUpdate).Seconds()

	if elapsed > 0 {
		rxSpeed := uint64(float64(uint64(peer.ReceiveBytes)-i.lastStats.RxBytes) / elapsed)
		txSpeed := uint64(float64(uint64(peer.TransmitBytes)-i.lastStats.TxBytes) / elapsed)

		i.stats = Stats{
			RxBytes: uint64(peer.ReceiveBytes),
			TxBytes: uint64(peer.TransmitBytes),
			RxSpeed: rxSpeed,
			TxSpeed: txSpeed,
		}

		i.lastStats = Stats{
			RxBytes: uint64(peer.ReceiveBytes),
			TxBytes: uint64(peer.TransmitBytes),
		}
		i.lastUpdate = now
	}

	return i.stats, nil
}

// Destroy removes the WireGuard interface
func (i *Interface) Destroy() error {
	if i.client != nil {
		i.client.Close()
	}

	return destroyInterface(i.Name)
}

// Platform-specific interface creation functions
// These are simplified placeholders - in production, you'd use proper implementations

// parseAllowedIPs parses a comma-separated list of CIDR notations
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

func createInterface(name string) error {
	switch runtime.GOOS {
	case "linux":
		// On Linux, use ip link add
		return exec.Command("ip", "link", "add", name, "type", "wireguard").Run()
	case "darwin":
		// On macOS, WireGuard interface is created by wireguard-go
		// This is handled by the userspace implementation
		return nil
	case "windows":
		// On Windows, use wintun driver
		// This is handled by wireguard-go with wintun
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func setInterfaceAddress(name, address string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("ip", "addr", "add", address, "dev", name).Run()
	case "darwin":
		return exec.Command("ifconfig", name, "inet", address, "alias").Run()
	case "windows":
		// Windows address assignment is more complex
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func bringInterfaceUp(name string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("ip", "link", "set", name, "up").Run()
	case "darwin":
		return exec.Command("ifconfig", name, "up").Run()
	case "windows":
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func destroyInterface(name string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("ip", "link", "del", name).Run()
	case "darwin", "windows":
		// Userspace implementations cleanup automatically
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
