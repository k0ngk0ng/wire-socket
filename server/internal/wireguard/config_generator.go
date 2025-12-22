package wireguard

import (
	"errors"
	"fmt"
	"net"
	"time"
	"vpn-server/internal/database"

	"gorm.io/gorm"
)

// ConfigGenerator handles dynamic WireGuard configuration generation
type ConfigGenerator struct {
	db        *database.DB
	wgManager *Manager
}

// NewConfigGenerator creates a new config generator
func NewConfigGenerator(db *database.DB, wgManager *Manager) *ConfigGenerator {
	return &ConfigGenerator{
		db:        db,
		wgManager: wgManager,
	}
}

// WGConfig represents a WireGuard configuration for a client
type WGConfig struct {
	PrivateKey string     `json:"private_key"`
	Address    string     `json:"address"`     // e.g., 10.0.0.5/32
	DNS        string     `json:"dns"`         // e.g., 1.1.1.1,8.8.8.8
	Peer       PeerConfig `json:"peer"`
}

// PeerConfig represents the server peer configuration
type PeerConfig struct {
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint"`   // e.g., vpn.example.com:51820
	AllowedIPs string `json:"allowed_ips"` // e.g., 0.0.0.0/0
}

// GenerateForUser generates a WireGuard configuration for a specific user
func (g *ConfigGenerator) GenerateForUser(userID uint, serverID uint) (*WGConfig, error) {
	// Get server details
	var server database.Server
	if err := g.db.First(&server, serverID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("server not found")
		}
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	// Generate or retrieve user's key pair
	privateKey, publicKey, err := g.getUserKeys(userID, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user keys: %w", err)
	}

	// Allocate IP from server's subnet
	ip, err := g.allocateIP(userID, serverID, publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate IP: %w", err)
	}

	// Add peer to WireGuard server
	if err := g.wgManager.AddPeer(publicKey, ip+"/32"); err != nil {
		return nil, fmt.Errorf("failed to add peer to WireGuard: %w", err)
	}

	// Generate client config
	config := &WGConfig{
		PrivateKey: privateKey,
		Address:    ip + "/32",
		DNS:        server.DNS,
		Peer: PeerConfig{
			PublicKey:  server.PublicKey,
			Endpoint:   server.Endpoint,
			AllowedIPs: "0.0.0.0/0", // Route all traffic through VPN
		},
	}

	return config, nil
}

// getUserKeys generates or retrieves user's WireGuard key pair
func (g *ConfigGenerator) getUserKeys(userID, serverID uint) (privateKey, publicKey string, err error) {
	// Check if user already has an allocated IP (which includes keys)
	var existing database.AllocatedIP
	err = g.db.Where("user_id = ? AND server_id = ?", userID, serverID).First(&existing).Error
	if err == nil {
		// User already has keys, return them
		// In a real implementation, you'd store the private key securely
		// For now, generate new keys
		return GenerateKeyPair()
	}

	// Generate new key pair
	return GenerateKeyPair()
}

// allocateIP allocates an IP address from the server's subnet
func (g *ConfigGenerator) allocateIP(userID, serverID uint, publicKey string) (string, error) {
	// Check if user already has an IP
	var existing database.AllocatedIP
	err := g.db.Where("user_id = ? AND server_id = ?", userID, serverID).First(&existing).Error
	if err == nil {
		// Update last seen and public key
		existing.PublicKey = publicKey
		now := time.Now()
		existing.LastSeen = &now
		g.db.Save(&existing)
		return existing.IPAddress, nil
	}

	// Get server details
	var server database.Server
	if err := g.db.First(&server, serverID).Error; err != nil {
		return "", fmt.Errorf("server not found: %w", err)
	}

	// Parse server subnet
	_, subnet, err := net.ParseCIDR(server.Subnet)
	if err != nil {
		return "", fmt.Errorf("invalid server subnet: %w", err)
	}

	// Get all allocated IPs for this server
	var allocated []database.AllocatedIP
	g.db.Where("server_id = ?", serverID).Find(&allocated)

	usedIPs := make(map[string]bool)
	for _, a := range allocated {
		usedIPs[a.IPAddress] = true
	}

	// Find next available IP
	ip := make(net.IP, len(subnet.IP))
	copy(ip, subnet.IP)

	// Increment IP until we find an available one
	for {
		ip = nextIP(ip, subnet)
		if !subnet.Contains(ip) {
			return "", fmt.Errorf("no available IPs in subnet")
		}

		ipStr := ip.String()

		// Skip network address, gateway (first IP), and broadcast
		if isReservedIP(ipStr, subnet) {
			continue
		}

		if !usedIPs[ipStr] {
			// Found an available IP
			alloc := database.AllocatedIP{
				UserID:    userID,
				ServerID:  serverID,
				IPAddress: ipStr,
				PublicKey: publicKey,
				AllocatedAt: time.Now(),
			}

			if err := g.db.Create(&alloc).Error; err != nil {
				return "", fmt.Errorf("failed to allocate IP: %w", err)
			}

			return ipStr, nil
		}
	}
}

// nextIP increments an IP address
func nextIP(ip net.IP, subnet *net.IPNet) net.IP {
	newIP := make(net.IP, len(ip))
	copy(newIP, ip)

	for i := len(newIP) - 1; i >= 0; i-- {
		newIP[i]++
		if newIP[i] > 0 {
			break
		}
	}

	return newIP
}

// isReservedIP checks if an IP is reserved (network address, gateway, broadcast)
func isReservedIP(ipStr string, subnet *net.IPNet) bool {
	ip := net.ParseIP(ipStr)

	// Network address
	if ip.Equal(subnet.IP) {
		return true
	}

	// Gateway (first usable IP)
	gateway := make(net.IP, len(subnet.IP))
	copy(gateway, subnet.IP)
	gateway[len(gateway)-1]++
	if ip.Equal(gateway) {
		return true
	}

	// Broadcast address (for IPv4)
	if len(ip) == net.IPv4len || ip.To4() != nil {
		broadcast := make(net.IP, len(subnet.IP))
		copy(broadcast, subnet.IP)
		for i := range broadcast {
			broadcast[i] |= ^subnet.Mask[i]
		}
		if ip.Equal(broadcast) {
			return true
		}
	}

	return false
}

// ReclaimStaleIPs removes IP allocations for inactive users
func (g *ConfigGenerator) ReclaimStaleIPs(inactiveDays int) error {
	cutoff := time.Now().Add(-time.Duration(inactiveDays) * 24 * time.Hour)

	var staleIPs []database.AllocatedIP
	if err := g.db.Where("last_seen < ? OR last_seen IS NULL", cutoff).Find(&staleIPs).Error; err != nil {
		return fmt.Errorf("failed to find stale IPs: %w", err)
	}

	for _, allocation := range staleIPs {
		// Remove peer from WireGuard
		if err := g.wgManager.RemovePeer(allocation.PublicKey); err != nil {
			// Log error but continue
			fmt.Printf("failed to remove peer %s: %v\n", allocation.PublicKey, err)
		}

		// Delete allocation from database
		if err := g.db.Delete(&allocation).Error; err != nil {
			fmt.Printf("failed to delete allocation for IP %s: %v\n", allocation.IPAddress, err)
		}
	}

	return nil
}

// ToINIFormat converts a WGConfig to WireGuard INI format
func (c *WGConfig) ToINIFormat() string {
	return fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s
DNS = %s

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = %s
PersistentKeepalive = 25
`, c.PrivateKey, c.Address, c.DNS, c.Peer.PublicKey, c.Peer.Endpoint, c.Peer.AllowedIPs)
}
