// Package wireguard provides kernel-mode WireGuard implementation using wgctrl
package wireguard

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// KernelBackend implements Backend using kernel WireGuard via wgctrl
type KernelBackend struct {
	mu         sync.RWMutex
	client     *wgctrl.Client
	name       string
	listenPort int
	privateKey wgtypes.Key
	address    string

	// Stats tracking
	lastStats  Stats
	lastUpdate time.Time
}

// KernelConfig configures the kernel backend
type KernelConfig struct {
	InterfaceName string
}

// NewKernelBackend creates a new kernel WireGuard backend
func NewKernelBackend(cfg KernelConfig) (*KernelBackend, error) {
	name := cfg.InterfaceName
	if name == "" {
		name = "wg0"
	}

	// Create kernel interface first
	if err := createKernelInterface(name); err != nil {
		return nil, fmt.Errorf("failed to create kernel interface: %w", err)
	}

	client, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create wgctrl client: %w", err)
	}

	return &KernelBackend{
		client:     client,
		name:       name,
		lastUpdate: time.Now(),
	}, nil
}

// Configure sets up the WireGuard interface
func (k *KernelBackend) Configure(cfg Config) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	privateKey, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	k.privateKey = privateKey

	port := cfg.ListenPort
	wgConfig := wgtypes.Config{
		PrivateKey: &privateKey,
		ListenPort: &port,
	}

	if err := k.client.ConfigureDevice(k.name, wgConfig); err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	k.listenPort = port
	k.address = cfg.Address

	// Set address on interface
	if cfg.Address != "" {
		if err := setKernelInterfaceAddress(k.name, cfg.Address); err != nil {
			return fmt.Errorf("failed to set address: %w", err)
		}
	}

	// Bring interface up
	if err := bringKernelInterfaceUp(k.name); err != nil {
		return fmt.Errorf("failed to bring interface up: %w", err)
	}

	return nil
}

// AddPeer adds a peer to the interface
func (k *KernelBackend) AddPeer(peer PeerConfig) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	peerKey, err := wgtypes.ParseKey(peer.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse peer public key: %w", err)
	}

	var allowedIPs []net.IPNet
	for _, ip := range peer.AllowedIPs {
		_, ipNet, err := net.ParseCIDR(ip)
		if err != nil {
			return fmt.Errorf("failed to parse allowed IP %s: %w", ip, err)
		}
		allowedIPs = append(allowedIPs, *ipNet)
	}

	var endpoint *net.UDPAddr
	if peer.Endpoint != "" {
		endpoint, err = net.ResolveUDPAddr("udp", peer.Endpoint)
		if err != nil {
			return fmt.Errorf("failed to resolve endpoint: %w", err)
		}
	}

	keepalive := peer.PersistentKeepalive
	if keepalive == 0 {
		keepalive = 25 * time.Second
	}

	peerConfig := wgtypes.PeerConfig{
		PublicKey:                   peerKey,
		Endpoint:                    endpoint,
		AllowedIPs:                  allowedIPs,
		PersistentKeepaliveInterval: &keepalive,
	}

	wgConfig := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerConfig},
	}

	if err := k.client.ConfigureDevice(k.name, wgConfig); err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	return nil
}

// RemovePeer removes a peer by public key
func (k *KernelBackend) RemovePeer(publicKey string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	peerKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse peer public key: %w", err)
	}

	peerConfig := wgtypes.PeerConfig{
		PublicKey: peerKey,
		Remove:    true,
	}

	wgConfig := wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peerConfig},
	}

	if err := k.client.ConfigureDevice(k.name, wgConfig); err != nil {
		return fmt.Errorf("failed to remove peer: %w", err)
	}

	return nil
}

// GetStats returns traffic statistics
func (k *KernelBackend) GetStats() (Stats, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	device, err := k.client.Device(k.name)
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get device: %w", err)
	}

	var totalRx, totalTx uint64
	for _, peer := range device.Peers {
		totalRx += uint64(peer.ReceiveBytes)
		totalTx += uint64(peer.TransmitBytes)
	}

	now := time.Now()
	elapsed := now.Sub(k.lastUpdate).Seconds()

	var rxSpeed, txSpeed uint64
	if elapsed > 0 && k.lastUpdate != (time.Time{}) {
		rxSpeed = uint64(float64(totalRx-k.lastStats.RxBytes) / elapsed)
		txSpeed = uint64(float64(totalTx-k.lastStats.TxBytes) / elapsed)
	}

	stats := Stats{
		RxBytes: totalRx,
		TxBytes: totalTx,
		RxSpeed: rxSpeed,
		TxSpeed: txSpeed,
	}

	k.lastStats = stats
	k.lastUpdate = now

	return stats, nil
}

// GetPeerStats returns statistics for all peers
func (k *KernelBackend) GetPeerStats() ([]PeerStats, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	device, err := k.client.Device(k.name)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	peers := make([]PeerStats, 0, len(device.Peers))
	for _, peer := range device.Peers {
		var endpoint string
		if peer.Endpoint != nil {
			endpoint = peer.Endpoint.String()
		}
		peers = append(peers, PeerStats{
			PublicKey:     peer.PublicKey.String(),
			Endpoint:      endpoint,
			LastHandshake: peer.LastHandshakeTime,
			RxBytes:       peer.ReceiveBytes,
			TxBytes:       peer.TransmitBytes,
		})
	}

	return peers, nil
}

// GetPublicKey returns the public key derived from the private key
func (k *KernelBackend) GetPublicKey() string {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.privateKey.PublicKey().String()
}

// GetListenPort returns the UDP listen port
func (k *KernelBackend) GetListenPort() int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.listenPort
}

// GetDeviceName returns the interface name
func (k *KernelBackend) GetDeviceName() string {
	return k.name
}

// Close shuts down the WireGuard interface
func (k *KernelBackend) Close() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if k.client != nil {
		k.client.Close()
		k.client = nil
	}

	return destroyKernelInterface(k.name)
}

// SetRoutes configures routing for the VPN (client-specific)
func (k *KernelBackend) SetRoutes(routes []net.IPNet) error {
	return setRoutes(k.name, routes)
}

// Platform-specific helpers for kernel mode

func createKernelInterface(name string) error {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("ip", "link", "add", name, "type", "wireguard")
		output, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "File exists") {
				return nil // Interface already exists
			}
			return fmt.Errorf("failed to create interface: %s: %w", string(output), err)
		}
		return nil
	case "darwin", "windows":
		// These platforms typically use userspace wireguard-go
		// Kernel interface creation is not applicable
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func setKernelInterfaceAddress(name, address string) error {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("ip", "addr", "add", address, "dev", name)
		output, err := cmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "File exists") {
				return nil
			}
			return fmt.Errorf("failed to set address: %s: %w", string(output), err)
		}
		return nil
	case "darwin":
		ip, _, err := net.ParseCIDR(address)
		if err != nil {
			return err
		}
		cmd := exec.Command("ifconfig", name, "inet", ip.String(), ip.String(), "alias")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to set address: %s: %w", string(output), err)
		}
		return nil
	case "windows":
		return nil // Handled by wintun
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func bringKernelInterfaceUp(name string) error {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("ip", "link", "set", name, "up")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to bring interface up: %s: %w", string(output), err)
		}
		return nil
	case "darwin":
		cmd := exec.Command("ifconfig", name, "up")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to bring interface up: %s: %w", string(output), err)
		}
		return nil
	case "windows":
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func destroyKernelInterface(name string) error {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("ip", "link", "del", name)
		cmd.Run() // Ignore errors - interface might not exist
		return nil
	case "darwin", "windows":
		return nil // Userspace cleanup
	default:
		return nil
	}
}
