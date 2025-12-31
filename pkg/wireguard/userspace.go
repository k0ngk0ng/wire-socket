// Package wireguard provides userspace WireGuard implementation
package wireguard

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// UserspaceBackend implements Backend using pure Go WireGuard
type UserspaceBackend struct {
	mu         sync.RWMutex
	name       string
	tunDevice  tun.Device
	wgDevice   *device.Device
	listenPort int
	privateKey wgtypes.Key
	address    string

	// Stats tracking
	lastStats  Stats
	lastUpdate time.Time
}

// UserspaceConfig configures the userspace backend
type UserspaceConfig struct {
	InterfaceName string
	MTU           int
}

// NewUserspaceBackend creates a new userspace WireGuard backend
func NewUserspaceBackend(cfg UserspaceConfig) (*UserspaceBackend, error) {
	name := cfg.InterfaceName
	if name == "" {
		name = "wg0"
	}

	mtu := cfg.MTU
	if mtu == 0 {
		mtu = device.DefaultMTU
	}

	// Create TUN device
	tunDev, err := tun.CreateTUN(name, mtu)
	if err != nil {
		return nil, fmt.Errorf("failed to create TUN device: %w", err)
	}

	// Get actual interface name (may differ on some platforms)
	actualName, err := tunDev.Name()
	if err != nil {
		tunDev.Close()
		return nil, fmt.Errorf("failed to get TUN device name: %w", err)
	}

	// Create WireGuard device
	// Using LogLevelVerbose for debugging - change to LogLevelError in production
	logger := device.NewLogger(device.LogLevelVerbose, fmt.Sprintf("(%s) ", actualName))
	wgDev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	return &UserspaceBackend{
		name:       actualName,
		tunDevice:  tunDev,
		wgDevice:   wgDev,
		lastUpdate: time.Now(),
	}, nil
}

// Configure sets up the WireGuard interface
func (u *UserspaceBackend) Configure(cfg Config) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Parse private key
	privateKey, err := wgtypes.ParseKey(cfg.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	u.privateKey = privateKey

	// Build UAPI configuration
	var uapiConfig strings.Builder
	uapiConfig.WriteString(fmt.Sprintf("private_key=%s\n", hexKey(privateKey)))

	if cfg.ListenPort > 0 {
		uapiConfig.WriteString(fmt.Sprintf("listen_port=%d\n", cfg.ListenPort))
		u.listenPort = cfg.ListenPort
	}

	// Apply configuration via UAPI
	if err := u.wgDevice.IpcSet(uapiConfig.String()); err != nil {
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Bring device up
	if err := u.wgDevice.Up(); err != nil {
		return fmt.Errorf("failed to bring device up: %w", err)
	}

	u.address = cfg.Address

	// Set interface address
	if cfg.Address != "" {
		if err := setTunAddress(u.name, cfg.Address); err != nil {
			return fmt.Errorf("failed to set address: %w", err)
		}
	}

	return nil
}

// AddPeer adds a peer to the interface
func (u *UserspaceBackend) AddPeer(peer PeerConfig) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	peerKey, err := wgtypes.ParseKey(peer.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse peer public key: %w", err)
	}

	var uapiConfig strings.Builder
	uapiConfig.WriteString(fmt.Sprintf("public_key=%s\n", hexKey(peerKey)))

	// Parse and set endpoint
	if peer.Endpoint != "" {
		host, port, err := net.SplitHostPort(peer.Endpoint)
		if err != nil {
			return fmt.Errorf("failed to parse endpoint: %w", err)
		}
		// Resolve the hostname if needed
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("failed to resolve endpoint: %w", err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("no IPs found for endpoint: %s", host)
		}
		uapiConfig.WriteString(fmt.Sprintf("endpoint=%s:%s\n", ips[0].String(), port))
	}

	// Add allowed IPs
	for _, allowedIP := range peer.AllowedIPs {
		prefix, err := netip.ParsePrefix(allowedIP)
		if err != nil {
			return fmt.Errorf("failed to parse allowed IP %s: %w", allowedIP, err)
		}
		uapiConfig.WriteString(fmt.Sprintf("allowed_ip=%s\n", prefix.String()))
	}

	// Set persistent keepalive
	if peer.PersistentKeepalive > 0 {
		uapiConfig.WriteString(fmt.Sprintf("persistent_keepalive_interval=%d\n", int(peer.PersistentKeepalive.Seconds())))
	}

	if err := u.wgDevice.IpcSet(uapiConfig.String()); err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	return nil
}

// RemovePeer removes a peer by public key
func (u *UserspaceBackend) RemovePeer(publicKey string) error {
	u.mu.Lock()
	defer u.mu.Unlock()

	peerKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse peer public key: %w", err)
	}

	uapiConfig := fmt.Sprintf("public_key=%s\nremove=true\n", hexKey(peerKey))

	if err := u.wgDevice.IpcSet(uapiConfig); err != nil {
		return fmt.Errorf("failed to remove peer: %w", err)
	}

	return nil
}

// GetStats returns traffic statistics
func (u *UserspaceBackend) GetStats() (Stats, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	// Get stats via UAPI
	ipcOutput, err := u.wgDevice.IpcGet()
	if err != nil {
		return Stats{}, fmt.Errorf("failed to get device stats: %w", err)
	}

	var totalRx, totalTx uint64
	for _, line := range strings.Split(ipcOutput, "\n") {
		if strings.HasPrefix(line, "rx_bytes=") {
			fmt.Sscanf(line, "rx_bytes=%d", &totalRx)
		}
		if strings.HasPrefix(line, "tx_bytes=") {
			fmt.Sscanf(line, "tx_bytes=%d", &totalTx)
		}
	}

	now := time.Now()
	elapsed := now.Sub(u.lastUpdate).Seconds()

	var rxSpeed, txSpeed uint64
	if elapsed > 0 && u.lastUpdate != (time.Time{}) {
		rxSpeed = uint64(float64(totalRx-u.lastStats.RxBytes) / elapsed)
		txSpeed = uint64(float64(totalTx-u.lastStats.TxBytes) / elapsed)
	}

	stats := Stats{
		RxBytes: totalRx,
		TxBytes: totalTx,
		RxSpeed: rxSpeed,
		TxSpeed: txSpeed,
	}

	u.lastStats = stats
	u.lastUpdate = now

	return stats, nil
}

// GetPeerStats returns statistics for all peers
func (u *UserspaceBackend) GetPeerStats() ([]PeerStats, error) {
	u.mu.RLock()
	defer u.mu.RUnlock()

	ipcOutput, err := u.wgDevice.IpcGet()
	if err != nil {
		return nil, fmt.Errorf("failed to get device stats: %w", err)
	}

	var peers []PeerStats
	var currentPeer *PeerStats

	for _, line := range strings.Split(ipcOutput, "\n") {
		if strings.HasPrefix(line, "public_key=") {
			if currentPeer != nil {
				peers = append(peers, *currentPeer)
			}
			currentPeer = &PeerStats{}
			var hexPubKey string
			fmt.Sscanf(line, "public_key=%s", &hexPubKey)
			// Convert hex back to base64
			currentPeer.PublicKey = hexToBase64Key(hexPubKey)
		}
		if currentPeer != nil {
			if strings.HasPrefix(line, "endpoint=") {
				fmt.Sscanf(line, "endpoint=%s", &currentPeer.Endpoint)
			}
			if strings.HasPrefix(line, "last_handshake_time_sec=") {
				var sec int64
				fmt.Sscanf(line, "last_handshake_time_sec=%d", &sec)
				if sec > 0 {
					currentPeer.LastHandshake = time.Unix(sec, 0)
				}
			}
			if strings.HasPrefix(line, "rx_bytes=") {
				fmt.Sscanf(line, "rx_bytes=%d", &currentPeer.RxBytes)
			}
			if strings.HasPrefix(line, "tx_bytes=") {
				fmt.Sscanf(line, "tx_bytes=%d", &currentPeer.TxBytes)
			}
		}
	}

	if currentPeer != nil {
		peers = append(peers, *currentPeer)
	}

	return peers, nil
}

// GetPublicKey returns the public key derived from the private key
func (u *UserspaceBackend) GetPublicKey() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.privateKey.PublicKey().String()
}

// GetListenPort returns the UDP listen port
func (u *UserspaceBackend) GetListenPort() int {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.listenPort
}

// GetDeviceName returns the interface name
func (u *UserspaceBackend) GetDeviceName() string {
	return u.name
}

// Close shuts down the WireGuard interface
func (u *UserspaceBackend) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.wgDevice != nil {
		u.wgDevice.Close()
		u.wgDevice = nil
	}

	if u.tunDevice != nil {
		if err := u.tunDevice.Close(); err != nil {
			log.Printf("Failed to close TUN device: %v", err)
		}
		u.tunDevice = nil
	}

	return nil
}

// SetRoutes configures routing for the VPN (client-specific)
func (u *UserspaceBackend) SetRoutes(routes []net.IPNet) error {
	// Platform-specific route configuration
	return setRoutes(u.name, routes)
}

// hexKey converts a wgtypes.Key to hex string (for UAPI)
func hexKey(key wgtypes.Key) string {
	return fmt.Sprintf("%x", key[:])
}

// hexToBase64Key converts hex key back to base64
func hexToBase64Key(hex string) string {
	var keyBytes [32]byte
	fmt.Sscanf(hex, "%x", &keyBytes)
	key := wgtypes.Key(keyBytes)
	return key.String()
}
