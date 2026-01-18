package mesh

import (
	"testing"
)

func TestTunnelClientConfig(t *testing.T) {
	// Test creating tunnel client config parameters
	tunnelURL := "wss://vpn.example.com/"
	localPrivateKey := "privatekey123"
	remotePublicKey := "remotepubkey456"

	if tunnelURL == "" {
		t.Error("tunnelURL should not be empty")
	}
	if localPrivateKey == "" {
		t.Error("localPrivateKey should not be empty")
	}
	if remotePublicKey == "" {
		t.Error("remotePublicKey should not be empty")
	}
}

func TestNewTunnelClient(t *testing.T) {
	client, err := NewTunnelClient("wss://vpn.example.com/", "privkey", "pubkey")
	if err != nil {
		t.Fatalf("NewTunnelClient failed: %v", err)
	}

	if client == nil {
		t.Fatal("expected non-nil TunnelClient")
	}

	// Check that the local UDP port was allocated
	endpoint := client.GetLocalEndpoint()
	if endpoint == "" {
		t.Error("GetLocalEndpoint should return a non-empty string")
	}

	// Endpoint should be in format "127.0.0.1:port"
	if len(endpoint) < 12 { // "127.0.0.1:1" minimum
		t.Errorf("endpoint too short: %s", endpoint)
	}

	// Clean up
	client.Close()
}

func TestTunnelClientClose(t *testing.T) {
	client, err := NewTunnelClient("wss://vpn.example.com/", "privkey", "pubkey")
	if err != nil {
		t.Fatalf("NewTunnelClient failed: %v", err)
	}

	// Close should not panic
	client.Close()

	// Double close should also not panic
	client.Close()
}

func TestTunnelClientStart(t *testing.T) {
	client, err := NewTunnelClient("wss://vpn.example.com/", "privkey", "pubkey")
	if err != nil {
		t.Fatalf("NewTunnelClient failed: %v", err)
	}
	defer client.Close()

	// Start should return nil (placeholder implementation)
	err = client.Start()
	if err != nil {
		t.Errorf("Start should not return error, got: %v", err)
	}
}

func TestGetLocalEndpointFormat(t *testing.T) {
	client, err := NewTunnelClient("wss://test.example.com/", "key1", "key2")
	if err != nil {
		t.Fatalf("NewTunnelClient failed: %v", err)
	}
	defer client.Close()

	endpoint := client.GetLocalEndpoint()

	// Should start with 127.0.0.1:
	if len(endpoint) < 10 {
		t.Errorf("endpoint format invalid: %s", endpoint)
		return
	}

	prefix := endpoint[:10]
	if prefix != "127.0.0.1:" {
		t.Errorf("endpoint should start with '127.0.0.1:', got '%s'", prefix)
	}
}
