package mobile

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
)

// TunnelConfig contains the WireGuard configuration for the tunnel.
type TunnelConfig struct {
	PrivateKey    string   `json:"privateKey"`
	Address       string   `json:"address"`
	DNS           string   `json:"dns"`
	PeerPublicKey string   `json:"peerPublicKey"`
	PeerEndpoint  string   `json:"peerEndpoint"`
	AllowedIPs    []string `json:"allowedIPs"`
	MTU           int      `json:"mtu"`
}

// TunnelStats contains traffic statistics for the tunnel.
type TunnelStats struct {
	RxBytes uint64 `json:"rxBytes"`
	TxBytes uint64 `json:"txBytes"`
}

// Tunnel manages a WireGuard tunnel using a platform-provided TUN device.
// This is designed for use with iOS NEPacketTunnelProvider and Android VpnService.
type Tunnel struct {
	mu       sync.RWMutex
	device   *device.Device
	tunDev   tun.Device
	config   *TunnelConfig
	running  bool
	closeCh  chan struct{}
}

// NewTunnel creates a new tunnel instance.
// Call StartWithFD to start the tunnel with a platform-provided file descriptor.
func NewTunnel() *Tunnel {
	return &Tunnel{
		closeCh: make(chan struct{}),
	}
}

// StartWithFD starts the WireGuard tunnel using a file descriptor from the platform.
// On Android, this is the FD from VpnService.Builder.establish().
// On iOS, this is the FD from NEPacketTunnelProvider.packetFlow.
// configJSON is a JSON-encoded TunnelConfig.
func (t *Tunnel) StartWithFD(fd int, configJSON string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return fmt.Errorf("tunnel already running")
	}

	// Parse config
	var config TunnelConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if config.MTU == 0 {
		config.MTU = 1420
	}

	// Create TUN device from file descriptor
	file := os.NewFile(uintptr(fd), "tun")
	if file == nil {
		return fmt.Errorf("invalid file descriptor")
	}

	tunDev, err := tun.CreateTUNFromFile(file, config.MTU)
	if err != nil {
		file.Close()
		return fmt.Errorf("failed to create TUN from fd: %w", err)
	}

	// Create WireGuard device
	logger := device.NewLogger(device.LogLevelError, "(wg) ")
	wgDev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	// Configure WireGuard
	if err := t.configureDevice(wgDev, &config); err != nil {
		wgDev.Close()
		tunDev.Close()
		return fmt.Errorf("failed to configure device: %w", err)
	}

	// Bring device up
	if err := wgDev.Up(); err != nil {
		wgDev.Close()
		tunDev.Close()
		return fmt.Errorf("failed to bring device up: %w", err)
	}

	t.device = wgDev
	t.tunDev = tunDev
	t.config = &config
	t.running = true
	t.closeCh = make(chan struct{})

	return nil
}

// configureDevice configures the WireGuard device with the given config.
func (t *Tunnel) configureDevice(wgDev *device.Device, config *TunnelConfig) error {
	// Build UAPI configuration
	uapi := fmt.Sprintf("private_key=%s\n", hexEncode(config.PrivateKey))

	// Add peer
	uapi += fmt.Sprintf("public_key=%s\n", hexEncode(config.PeerPublicKey))
	if config.PeerEndpoint != "" {
		uapi += fmt.Sprintf("endpoint=%s\n", config.PeerEndpoint)
	}
	for _, allowedIP := range config.AllowedIPs {
		uapi += fmt.Sprintf("allowed_ip=%s\n", allowedIP)
	}
	uapi += "persistent_keepalive_interval=25\n"

	return wgDev.IpcSet(uapi)
}

// Stop stops the tunnel and releases resources.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return
	}

	close(t.closeCh)

	if t.device != nil {
		t.device.Close()
		t.device = nil
	}

	if t.tunDev != nil {
		t.tunDev.Close()
		t.tunDev = nil
	}

	t.running = false
}

// IsRunning returns true if the tunnel is running.
func (t *Tunnel) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

// GetStats returns the current tunnel statistics.
func (t *Tunnel) GetStats() string {
	t.mu.RLock()
	dev := t.device
	t.mu.RUnlock()

	stats := TunnelStats{}
	if dev != nil {
		// Get stats from WireGuard device
		ipcOutput, err := dev.IpcGet()
		if err == nil {
			for _, line := range splitLines(ipcOutput) {
				if len(line) > 9 && line[:9] == "rx_bytes=" {
					fmt.Sscanf(line, "rx_bytes=%d", &stats.RxBytes)
				}
				if len(line) > 9 && line[:9] == "tx_bytes=" {
					fmt.Sscanf(line, "tx_bytes=%d", &stats.TxBytes)
				}
			}
		}
	}

	data, _ := json.Marshal(stats)
	return string(data)
}

// WaitForClose blocks until the tunnel is closed or the timeout expires.
// Returns true if the tunnel was closed, false if timeout.
func (t *Tunnel) WaitForClose(timeoutMs int) bool {
	t.mu.RLock()
	closeCh := t.closeCh
	t.mu.RUnlock()

	if closeCh == nil {
		return true
	}

	select {
	case <-closeCh:
		return true
	case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
		return false
	}
}

// hexEncode converts a base64-encoded WireGuard key to hex format for UAPI.
func hexEncode(base64Key string) string {
	keyBytes, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		// If not valid base64, assume it's already in hex or raw format
		return base64Key
	}
	return hex.EncodeToString(keyBytes)
}

// splitLines splits a string by newlines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
