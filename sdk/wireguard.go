package sdk

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"

	wg "github.com/k0ngk0ng/wire-socket/pkg/wireguard"
)

// wgBackendImpl implements wgBackend using the pkg/wireguard package
type wgBackendImpl struct {
	backend wg.ClientBackend
	name    string
}

// newWGBackend creates a new WireGuard backend using the shared package
func newWGBackend(name string) (wgBackend, error) {
	if name == "" {
		if runtime.GOOS == "darwin" {
			name = "utun"
		} else {
			name = "wg-vpn"
		}
	}

	backend, err := wg.NewUserspaceBackend(wg.UserspaceConfig{
		InterfaceName: name,
		MTU:           1420,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create WireGuard backend: %w", err)
	}

	return &wgBackendImpl{
		backend: backend,
		name:    name,
	}, nil
}

func (w *wgBackendImpl) Configure(cfg *wgConfig) error {
	// Configure the interface
	err := w.backend.Configure(wg.Config{
		PrivateKey: cfg.PrivateKey,
		Address:    cfg.Address,
		DNS:        cfg.DNS,
	})
	if err != nil {
		return fmt.Errorf("failed to configure interface: %w", err)
	}

	// Add peer
	err = w.backend.AddPeer(wg.PeerConfig{
		PublicKey:           cfg.PeerPublicKey,
		Endpoint:            cfg.PeerEndpoint,
		AllowedIPs:          cfg.AllowedIPs,
		PersistentKeepalive: 25 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	// Set up routes
	var routes []net.IPNet
	for _, ip := range cfg.AllowedIPs {
		_, ipNet, err := net.ParseCIDR(ip)
		if err != nil {
			continue
		}
		routes = append(routes, *ipNet)
	}

	if len(routes) > 0 {
		if err := w.backend.SetRoutes(routes); err != nil {
			// Log but don't fail - routing may need elevated privileges
			fmt.Printf("Warning: failed to set routes: %v\n", err)
		}
	}

	return nil
}

func (w *wgBackendImpl) GetStats() (wgStats, error) {
	stats, err := w.backend.GetStats()
	if err != nil {
		return wgStats{}, err
	}

	return wgStats{
		RxBytes: stats.RxBytes,
		TxBytes: stats.TxBytes,
		RxSpeed: stats.RxSpeed,
		TxSpeed: stats.TxSpeed,
	}, nil
}

func (w *wgBackendImpl) UpdateAllowedIPs(peerPublicKey string, allowedIPs []string) error {
	// Remove existing peer
	if err := w.backend.RemovePeer(peerPublicKey); err != nil {
		return fmt.Errorf("failed to remove peer: %w", err)
	}

	// Add peer with new AllowedIPs
	err := w.backend.AddPeer(wg.PeerConfig{
		PublicKey:           peerPublicKey,
		Endpoint:            "127.0.0.1:51820", // Tunnel endpoint
		AllowedIPs:          allowedIPs,
		PersistentKeepalive: 25 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to add peer: %w", err)
	}

	// Update routes
	var routes []net.IPNet
	for _, ip := range allowedIPs {
		_, ipNet, err := net.ParseCIDR(ip)
		if err != nil {
			continue
		}
		routes = append(routes, *ipNet)
	}

	if len(routes) > 0 {
		if err := w.backend.SetRoutes(routes); err != nil {
			fmt.Printf("Warning: failed to set routes: %v\n", err)
		}
	}

	return nil
}

func (w *wgBackendImpl) Close() error {
	if w.backend != nil {
		return w.backend.Close()
	}
	return nil
}

// parseAllowedIPs parses a comma-separated list of CIDR notations
func parseAllowedIPs(s string) ([]string, error) {
	var result []string
	for _, cidr := range strings.Split(s, ",") {
		cidr = strings.TrimSpace(cidr)
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
