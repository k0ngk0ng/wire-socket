package database

import (
	"testing"
	"time"
)

func TestMeshNodeRole(t *testing.T) {
	tests := []struct {
		role     MeshNodeRole
		expected string
	}{
		{MeshRoleGateway, "gateway"},
		{MeshRoleExit, "exit"},
		{MeshRoleBoth, "both"},
	}

	for _, tt := range tests {
		if string(tt.role) != tt.expected {
			t.Errorf("MeshNodeRole %v: expected %s, got %s", tt.role, tt.expected, string(tt.role))
		}
	}
}

func TestMeshNode(t *testing.T) {
	node := MeshNode{
		ID:        1,
		Name:      "gateway-us",
		PublicKey: "abc123pubkey",
		MeshIP:    "10.254.0.1",
		TunnelURL: "wss://vpn.example.com/",
		IsLocal:   true,
		IsOnline:  true,
	}

	if node.Name != "gateway-us" {
		t.Errorf("expected Name 'gateway-us', got '%s'", node.Name)
	}
	if node.PublicKey != "abc123pubkey" {
		t.Errorf("expected PublicKey 'abc123pubkey', got '%s'", node.PublicKey)
	}
	if node.MeshIP != "10.254.0.1" {
		t.Errorf("expected MeshIP '10.254.0.1', got '%s'", node.MeshIP)
	}
	if !node.IsLocal {
		t.Error("expected IsLocal to be true")
	}
	if !node.IsOnline {
		t.Error("expected IsOnline to be true")
	}
}

func TestMeshNodeLastSeen(t *testing.T) {
	now := time.Now()
	node := MeshNode{
		Name:     "test-node",
		LastSeen: &now,
	}

	if node.LastSeen == nil {
		t.Error("LastSeen should not be nil")
	}
	if !node.LastSeen.Equal(now) {
		t.Errorf("LastSeen mismatch: expected %v, got %v", now, *node.LastSeen)
	}
}

func TestExitRoute(t *testing.T) {
	route := ExitRoute{
		ID:       1,
		NodeID:   1,
		CIDR:     "192.168.1.0/24",
		Comment:  "Office network",
		Enabled:  true,
		Priority: 100,
	}

	if route.CIDR != "192.168.1.0/24" {
		t.Errorf("expected CIDR '192.168.1.0/24', got '%s'", route.CIDR)
	}
	if route.Comment != "Office network" {
		t.Errorf("expected Comment 'Office network', got '%s'", route.Comment)
	}
	if !route.Enabled {
		t.Error("expected Enabled to be true")
	}
	if route.Priority != 100 {
		t.Errorf("expected Priority 100, got %d", route.Priority)
	}
}

func TestExitRouteDefaults(t *testing.T) {
	// Test that default values work as expected
	route := ExitRoute{
		NodeID: 1,
		CIDR:   "10.0.0.0/8",
	}

	// Default Enabled should be false (Go zero value)
	// In GORM with `default:true`, this would be true after DB creation
	if route.Enabled {
		t.Error("Go zero value for Enabled should be false")
	}

	// Default Priority is 0 (Go zero value)
	// In GORM with `default:100`, this would be 100 after DB creation
	if route.Priority != 0 {
		t.Errorf("Go zero value for Priority should be 0, got %d", route.Priority)
	}
}

func TestMeshNodeWithExitRoutes(t *testing.T) {
	node := MeshNode{
		ID:        1,
		Name:      "exit-jp",
		PublicKey: "jpkey123",
		MeshIP:    "10.254.0.2",
		ExitRoutes: []ExitRoute{
			{ID: 1, NodeID: 1, CIDR: "192.168.1.0/24", Enabled: true},
			{ID: 2, NodeID: 1, CIDR: "10.10.0.0/16", Enabled: true},
			{ID: 3, NodeID: 1, CIDR: "172.16.0.0/12", Enabled: false},
		},
	}

	if len(node.ExitRoutes) != 3 {
		t.Errorf("expected 3 exit routes, got %d", len(node.ExitRoutes))
	}

	// Count enabled routes
	enabledCount := 0
	for _, route := range node.ExitRoutes {
		if route.Enabled {
			enabledCount++
		}
	}
	if enabledCount != 2 {
		t.Errorf("expected 2 enabled routes, got %d", enabledCount)
	}
}
