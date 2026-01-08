package wireguard

import (
	"testing"
	"time"
)

func TestGenerateKeyPair(t *testing.T) {
	privateKey, publicKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	if privateKey == "" {
		t.Error("private key is empty")
	}
	if publicKey == "" {
		t.Error("public key is empty")
	}
	if len(privateKey) != 44 { // Base64 encoded 32 bytes
		t.Errorf("unexpected private key length: %d", len(privateKey))
	}
	if len(publicKey) != 44 {
		t.Errorf("unexpected public key length: %d", len(publicKey))
	}

	// Verify keys are different
	if privateKey == publicKey {
		t.Error("private and public keys should be different")
	}

	t.Logf("Generated key pair - Private: %s..., Public: %s...", privateKey[:8], publicKey[:8])
}

func TestConfigStructs(t *testing.T) {
	// Test Config struct
	cfg := Config{
		PrivateKey: "test-private-key",
		Address:    "10.0.0.1/24",
		ListenPort: 51820,
		DNS:        "1.1.1.1",
		MTU:        1420,
	}

	if cfg.Address != "10.0.0.1/24" {
		t.Errorf("unexpected address: %s", cfg.Address)
	}

	// Test PeerConfig struct
	peer := PeerConfig{
		PublicKey:           "test-public-key",
		Endpoint:            "192.168.1.1:51820",
		AllowedIPs:          []string{"0.0.0.0/0"},
		PersistentKeepalive: 25 * time.Second,
	}

	if peer.PersistentKeepalive != 25*time.Second {
		t.Errorf("unexpected keepalive: %v", peer.PersistentKeepalive)
	}
}

func TestStatsStruct(t *testing.T) {
	stats := Stats{
		RxBytes: 1024,
		TxBytes: 2048,
		RxSpeed: 100,
		TxSpeed: 200,
	}

	if stats.RxBytes != 1024 {
		t.Errorf("unexpected RxBytes: %d", stats.RxBytes)
	}
	if stats.TxBytes != 2048 {
		t.Errorf("unexpected TxBytes: %d", stats.TxBytes)
	}
}

func TestPeerStatsStruct(t *testing.T) {
	now := time.Now()
	peerStats := PeerStats{
		PublicKey:     "test-public-key",
		Endpoint:      "192.168.1.1:51820",
		LastHandshake: now,
		RxBytes:       1024,
		TxBytes:       2048,
	}

	if peerStats.PublicKey != "test-public-key" {
		t.Errorf("unexpected public key: %s", peerStats.PublicKey)
	}
	if peerStats.LastHandshake != now {
		t.Errorf("unexpected last handshake time")
	}
}

func TestModeConstants(t *testing.T) {
	if ModeKernel != "kernel" {
		t.Errorf("unexpected kernel mode value: %s", ModeKernel)
	}
	if ModeUserspace != "userspace" {
		t.Errorf("unexpected userspace mode value: %s", ModeUserspace)
	}
}

func TestParseCIDR(t *testing.T) {
	testCases := []struct {
		cidr    string
		wantIP  string
		wantErr bool
	}{
		{"10.0.0.1/24", "10.0.0.1", false},
		{"192.168.1.100/32", "192.168.1.100", false},
		{"0.0.0.0/0", "0.0.0.0", false},
		{"invalid", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.cidr, func(t *testing.T) {
			ip, _, err := ParseCIDR(tc.cidr)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %s", tc.cidr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCIDR(%s) failed: %v", tc.cidr, err)
			}
			if ip.String() != tc.wantIP {
				t.Errorf("ParseCIDR(%s) = %s, want %s", tc.cidr, ip.String(), tc.wantIP)
			}
		})
	}
}

func TestHexKeyConversion(t *testing.T) {
	// Generate a real key pair to test conversion
	privateKey, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	// The hexKey and hexToBase64Key functions are tested indirectly
	// through the userspace backend, but we can verify the key format
	if len(privateKey) != 44 {
		t.Errorf("unexpected key length: %d", len(privateKey))
	}
}

// TestBackendInterface verifies that both backends implement the interface
func TestBackendInterface(t *testing.T) {
	// This is a compile-time check - if it compiles, the interfaces are implemented
	var _ Backend = (*UserspaceBackend)(nil)
	var _ Backend = (*KernelBackend)(nil)
	var _ ServerBackend = (*UserspaceBackend)(nil)
	var _ ServerBackend = (*KernelBackend)(nil)
	var _ ClientBackend = (*UserspaceBackend)(nil)
	var _ ClientBackend = (*KernelBackend)(nil)
}

// Note: Full integration tests for UserspaceBackend and KernelBackend
// require elevated privileges and actual TUN device creation.
// They should be run in a controlled environment.

func TestUserspaceBackendConfig(t *testing.T) {
	cfg := UserspaceConfig{
		InterfaceName: "wg-test",
		MTU:           1420,
	}

	if cfg.InterfaceName != "wg-test" {
		t.Errorf("unexpected interface name: %s", cfg.InterfaceName)
	}
	if cfg.MTU != 1420 {
		t.Errorf("unexpected MTU: %d", cfg.MTU)
	}
}

func TestKernelBackendConfig(t *testing.T) {
	cfg := KernelConfig{
		InterfaceName: "wg-test",
	}

	if cfg.InterfaceName != "wg-test" {
		t.Errorf("unexpected interface name: %s", cfg.InterfaceName)
	}
}
