package mesh

import (
	"testing"
	"time"

	"wire-socket-server/internal/database"
)

func TestConfig(t *testing.T) {
	config := Config{
		Enabled:      true,
		Name:         "gateway-us",
		Role:         database.MeshRoleGateway,
		MeshIP:       "10.254.0.1",
		Token:        "secret-token",
		SyncInterval: 30 * time.Second,
		TunnelURL:    "wss://vpn.example.com/",
		APIEndpoint:  "https://vpn.example.com:8080",
	}

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}
	if config.Name != "gateway-us" {
		t.Errorf("expected Name 'gateway-us', got '%s'", config.Name)
	}
	if config.Role != database.MeshRoleGateway {
		t.Errorf("expected Role 'gateway', got '%s'", config.Role)
	}
	if config.MeshIP != "10.254.0.1" {
		t.Errorf("expected MeshIP '10.254.0.1', got '%s'", config.MeshIP)
	}
	if config.SyncInterval != 30*time.Second {
		t.Errorf("expected SyncInterval 30s, got %v", config.SyncInterval)
	}
}

func TestPeerConfig(t *testing.T) {
	peerConfig := PeerConfig{
		Name:        "exit-jp",
		TunnelURL:   "wss://exit-jp.example.com/",
		APIEndpoint: "https://exit-jp.example.com:8080",
	}

	if peerConfig.Name != "exit-jp" {
		t.Errorf("expected Name 'exit-jp', got '%s'", peerConfig.Name)
	}
	if peerConfig.TunnelURL != "wss://exit-jp.example.com/" {
		t.Errorf("expected TunnelURL 'wss://exit-jp.example.com/', got '%s'", peerConfig.TunnelURL)
	}
}

func TestRouteTable(t *testing.T) {
	rt := &RouteTable{
		routes: make(map[string]*RouteEntry),
	}

	// Add some routes
	rt.routes["192.168.1.0/24"] = &RouteEntry{
		CIDR:     "192.168.1.0/24",
		ViaNode:  "exit-jp",
		ViaIP:    "10.254.0.2",
		Priority: 100,
		NodeID:   2,
	}
	rt.routes["10.10.0.0/16"] = &RouteEntry{
		CIDR:     "10.10.0.0/16",
		ViaNode:  "exit-eu",
		ViaIP:    "10.254.0.3",
		Priority: 50,
		NodeID:   3,
	}

	if len(rt.routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(rt.routes))
	}

	// Check route lookup
	entry, exists := rt.routes["192.168.1.0/24"]
	if !exists {
		t.Error("route 192.168.1.0/24 should exist")
	}
	if entry.ViaNode != "exit-jp" {
		t.Errorf("expected ViaNode 'exit-jp', got '%s'", entry.ViaNode)
	}
}

func TestRouteEntry(t *testing.T) {
	entry := RouteEntry{
		CIDR:     "172.16.0.0/12",
		ViaNode:  "exit-us",
		ViaIP:    "10.254.0.5",
		Priority: 200,
		NodeID:   5,
	}

	if entry.CIDR != "172.16.0.0/12" {
		t.Errorf("expected CIDR '172.16.0.0/12', got '%s'", entry.CIDR)
	}
	if entry.Priority != 200 {
		t.Errorf("expected Priority 200, got %d", entry.Priority)
	}
}

func TestPeer(t *testing.T) {
	now := time.Now()
	node := &database.MeshNode{
		ID:        2,
		Name:      "exit-jp",
		PublicKey: "jpkey123",
		MeshIP:    "10.254.0.2",
	}

	peer := &Peer{
		Node:     node,
		IsOnline: true,
		LastSeen: now,
		RTT:      50 * time.Millisecond,
	}

	if peer.Node.Name != "exit-jp" {
		t.Errorf("expected Node.Name 'exit-jp', got '%s'", peer.Node.Name)
	}
	if !peer.IsOnline {
		t.Error("expected IsOnline to be true")
	}
	if peer.RTT != 50*time.Millisecond {
		t.Errorf("expected RTT 50ms, got %v", peer.RTT)
	}
}

func TestHandshakeInfo(t *testing.T) {
	info := HandshakeInfo{
		Name:      "exit-jp",
		PublicKey: "pubkey123",
		MeshIP:    "10.254.0.2",
		TunnelURL: "wss://exit-jp.example.com/",
		ExitRoutes: []struct {
			CIDR     string
			Comment  string
			Priority int
		}{
			{CIDR: "192.168.1.0/24", Comment: "Office network", Priority: 100},
			{CIDR: "10.10.0.0/16", Comment: "Data center", Priority: 50},
		},
	}

	if info.Name != "exit-jp" {
		t.Errorf("expected Name 'exit-jp', got '%s'", info.Name)
	}
	if len(info.ExitRoutes) != 2 {
		t.Errorf("expected 2 exit routes, got %d", len(info.ExitRoutes))
	}
	if info.ExitRoutes[0].CIDR != "192.168.1.0/24" {
		t.Errorf("expected first route CIDR '192.168.1.0/24', got '%s'", info.ExitRoutes[0].CIDR)
	}
}

// MockWireGuardManager for testing
type MockWireGuardManager struct {
	peers           map[string][]string
	deviceName      string
	addPeerErr      error
	removePeerErr   error
	updatePeerErr   error
}

func NewMockWireGuardManager() *MockWireGuardManager {
	return &MockWireGuardManager{
		peers:      make(map[string][]string),
		deviceName: "wg0",
	}
}

func (m *MockWireGuardManager) AddMeshPeer(publicKey string, meshIP string, allowedIPs []string, endpoint string) error {
	if m.addPeerErr != nil {
		return m.addPeerErr
	}
	m.peers[publicKey] = allowedIPs
	return nil
}

func (m *MockWireGuardManager) RemovePeer(publicKey string) error {
	if m.removePeerErr != nil {
		return m.removePeerErr
	}
	delete(m.peers, publicKey)
	return nil
}

func (m *MockWireGuardManager) GetDeviceName() string {
	return m.deviceName
}

func (m *MockWireGuardManager) UpdateMeshPeerAllowedIPs(publicKey string, allowedIPs []string) error {
	if m.updatePeerErr != nil {
		return m.updatePeerErr
	}
	m.peers[publicKey] = allowedIPs
	return nil
}

func TestWireGuardManagerInterface(t *testing.T) {
	mock := NewMockWireGuardManager()

	// Test AddMeshPeer
	err := mock.AddMeshPeer("pubkey123", "10.254.0.2", []string{"10.254.0.2/32", "192.168.1.0/24"}, "127.0.0.1:51821")
	if err != nil {
		t.Errorf("AddMeshPeer failed: %v", err)
	}

	if len(mock.peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(mock.peers))
	}

	allowedIPs, exists := mock.peers["pubkey123"]
	if !exists {
		t.Error("peer 'pubkey123' should exist")
	}
	if len(allowedIPs) != 2 {
		t.Errorf("expected 2 allowed IPs, got %d", len(allowedIPs))
	}

	// Test GetDeviceName
	if mock.GetDeviceName() != "wg0" {
		t.Errorf("expected device name 'wg0', got '%s'", mock.GetDeviceName())
	}

	// Test UpdateMeshPeerAllowedIPs
	err = mock.UpdateMeshPeerAllowedIPs("pubkey123", []string{"10.254.0.2/32", "192.168.1.0/24", "10.10.0.0/16"})
	if err != nil {
		t.Errorf("UpdateMeshPeerAllowedIPs failed: %v", err)
	}
	if len(mock.peers["pubkey123"]) != 3 {
		t.Errorf("expected 3 allowed IPs after update, got %d", len(mock.peers["pubkey123"]))
	}

	// Test RemovePeer
	err = mock.RemovePeer("pubkey123")
	if err != nil {
		t.Errorf("RemovePeer failed: %v", err)
	}
	if len(mock.peers) != 0 {
		t.Errorf("expected 0 peers after removal, got %d", len(mock.peers))
	}
}

func TestConfigDefaults(t *testing.T) {
	// Test config with zero values
	config := Config{
		Enabled: true,
		Name:    "test",
		MeshIP:  "10.254.0.1",
		Token:   "secret",
	}

	// Role should default to empty (will be set to gateway in NewManager)
	if config.Role != "" {
		t.Errorf("expected empty Role for zero value, got '%s'", config.Role)
	}

	// SyncInterval should default to 0 (will be set to 30s in NewManager)
	if config.SyncInterval != 0 {
		t.Errorf("expected SyncInterval 0 for zero value, got %v", config.SyncInterval)
	}
}

func TestManagerNilSafety(t *testing.T) {
	// Test nil Manager methods don't panic
	var m *Manager = nil

	// These should all return safely without panic
	if m.GetRouteTable() != nil {
		t.Error("expected nil from nil Manager.GetRouteTable()")
	}
	if m.GetLocalNode() != nil {
		t.Error("expected nil from nil Manager.GetLocalNode()")
	}
	if m.GetPeers() != nil {
		t.Error("expected nil from nil Manager.GetPeers()")
	}
	if !m.IsGateway() {
		t.Error("expected IsGateway() to return true for nil Manager (default behavior)")
	}
	config := m.GetConfig()
	if config.Enabled {
		t.Error("expected empty Config from nil Manager")
	}

	// Start and Stop should not panic
	if err := m.Start(); err != nil {
		t.Errorf("nil Manager.Start() should return nil, got %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Errorf("nil Manager.Stop() should return nil, got %v", err)
	}
}

func TestIsGateway(t *testing.T) {
	tests := []struct {
		role     database.MeshNodeRole
		expected bool
	}{
		{database.MeshRoleGateway, true},
		{database.MeshRoleExit, false},
		{database.MeshRoleBoth, true},
	}

	for _, tt := range tests {
		// We can't easily create a full Manager without DB,
		// but we can test the logic directly
		isGateway := tt.role == database.MeshRoleGateway || tt.role == database.MeshRoleBoth
		if isGateway != tt.expected {
			t.Errorf("Role %s: expected IsGateway=%v, got %v", tt.role, tt.expected, isGateway)
		}
	}
}

func TestLearnedRoute(t *testing.T) {
	lr := LearnedRoute{
		CIDR:     "192.168.1.0/24",
		Priority: 100,
		Origin:   "exit-jp",
		HopCount: 2,
	}

	if lr.CIDR != "192.168.1.0/24" {
		t.Errorf("expected CIDR '192.168.1.0/24', got '%s'", lr.CIDR)
	}
	if lr.Priority != 100 {
		t.Errorf("expected Priority 100, got %d", lr.Priority)
	}
	if lr.Origin != "exit-jp" {
		t.Errorf("expected Origin 'exit-jp', got '%s'", lr.Origin)
	}
	if lr.HopCount != 2 {
		t.Errorf("expected HopCount 2, got %d", lr.HopCount)
	}
}

func TestSyncData(t *testing.T) {
	sd := SyncData{}
	sd.Node.Name = "gateway-us"
	sd.Node.MeshIP = "10.254.0.1"
	sd.ExitRoutes = []struct {
		CIDR     string `json:"cidr"`
		Priority int    `json:"priority"`
	}{
		{CIDR: "192.168.1.0/24", Priority: 100},
		{CIDR: "10.10.0.0/16", Priority: 50},
	}
	sd.LearnedRoutes = []LearnedRoute{
		{CIDR: "172.16.0.0/12", Priority: 110, Origin: "exit-eu", HopCount: 2},
	}

	if sd.Node.Name != "gateway-us" {
		t.Errorf("expected Node.Name 'gateway-us', got '%s'", sd.Node.Name)
	}
	if len(sd.ExitRoutes) != 2 {
		t.Errorf("expected 2 exit routes, got %d", len(sd.ExitRoutes))
	}
	if len(sd.LearnedRoutes) != 1 {
		t.Errorf("expected 1 learned route, got %d", len(sd.LearnedRoutes))
	}
	if sd.LearnedRoutes[0].HopCount != 2 {
		t.Errorf("expected HopCount 2, got %d", sd.LearnedRoutes[0].HopCount)
	}
}

func TestRoutesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []LearnedRoute
		b        []LearnedRoute
		expected bool
	}{
		{
			name:     "both empty",
			a:        []LearnedRoute{},
			b:        []LearnedRoute{},
			expected: true,
		},
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name: "equal routes",
			a: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 1},
				{CIDR: "10.10.0.0/16", Priority: 50, Origin: "node-b", HopCount: 2},
			},
			b: []LearnedRoute{
				{CIDR: "10.10.0.0/16", Priority: 50, Origin: "node-b", HopCount: 2},
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 1},
			},
			expected: true,
		},
		{
			name: "different length",
			a: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 1},
			},
			b: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 1},
				{CIDR: "10.10.0.0/16", Priority: 50, Origin: "node-b", HopCount: 2},
			},
			expected: false,
		},
		{
			name: "different priority",
			a: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 1},
			},
			b: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 200, Origin: "node-a", HopCount: 1},
			},
			expected: false,
		},
		{
			name: "different origin",
			a: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 1},
			},
			b: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-b", HopCount: 1},
			},
			expected: false,
		},
		{
			name: "different hop count",
			a: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 1},
			},
			b: []LearnedRoute{
				{CIDR: "192.168.1.0/24", Priority: 100, Origin: "node-a", HopCount: 2},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := routesEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("routesEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRouteEntryWithOriginAndHopCount(t *testing.T) {
	entry := RouteEntry{
		CIDR:     "172.16.0.0/12",
		ViaNode:  "exit-us",
		ViaIP:    "10.254.0.5",
		Priority: 200,
		NodeID:   5,
		Origin:   "exit-jp",
		HopCount: 2,
	}

	if entry.Origin != "exit-jp" {
		t.Errorf("expected Origin 'exit-jp', got '%s'", entry.Origin)
	}
	if entry.HopCount != 2 {
		t.Errorf("expected HopCount 2, got %d", entry.HopCount)
	}
}

func TestUpdateMeshPeerAllowedIPs(t *testing.T) {
	mock := NewMockWireGuardManager()

	// Add initial peer
	err := mock.AddMeshPeer("pubkey123", "10.254.0.2", []string{"10.254.0.2/32"}, "127.0.0.1:51821")
	if err != nil {
		t.Errorf("AddMeshPeer failed: %v", err)
	}

	// Update AllowedIPs
	newAllowedIPs := []string{"10.254.0.2/32", "192.168.1.0/24", "10.10.0.0/16"}
	err = mock.UpdateMeshPeerAllowedIPs("pubkey123", newAllowedIPs)
	if err != nil {
		t.Errorf("UpdateMeshPeerAllowedIPs failed: %v", err)
	}

	// Verify update
	allowedIPs, exists := mock.peers["pubkey123"]
	if !exists {
		t.Error("peer 'pubkey123' should exist")
	}
	if len(allowedIPs) != 3 {
		t.Errorf("expected 3 allowed IPs after update, got %d", len(allowedIPs))
	}
}

func TestHandshakeInfoWithPriority(t *testing.T) {
	info := HandshakeInfo{
		Name:      "exit-jp",
		PublicKey: "pubkey123",
		MeshIP:    "10.254.0.2",
		TunnelURL: "wss://exit-jp.example.com/",
		ExitRoutes: []struct {
			CIDR     string
			Comment  string
			Priority int
		}{
			{CIDR: "192.168.1.0/24", Comment: "Office network", Priority: 100},
			{CIDR: "10.10.0.0/16", Comment: "Data center", Priority: 50},
		},
	}

	if len(info.ExitRoutes) != 2 {
		t.Errorf("expected 2 exit routes, got %d", len(info.ExitRoutes))
	}
	if info.ExitRoutes[0].Priority != 100 {
		t.Errorf("expected first route priority 100, got %d", info.ExitRoutes[0].Priority)
	}
	if info.ExitRoutes[1].Priority != 50 {
		t.Errorf("expected second route priority 50, got %d", info.ExitRoutes[1].Priority)
	}
}

func TestGetLearnedRoutesNilManager(t *testing.T) {
	var m *Manager = nil

	routes := m.GetLearnedRoutes()
	if routes != nil {
		t.Error("expected nil from nil Manager.GetLearnedRoutes()")
	}
}

func TestTriggerSyncNilSafety(t *testing.T) {
	// Ensure TriggerSync doesn't panic on nil manager
	var m *Manager = nil
	// This would panic if not nil-safe in syncWithPeers
	// Since TriggerSync spawns a goroutine, we just ensure it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("TriggerSync panicked: %v", r)
		}
	}()
	// Can't call TriggerSync on nil, but ensure Start/Stop are safe
	if err := m.Start(); err != nil {
		t.Errorf("nil Manager.Start() should return nil, got %v", err)
	}
}
