// Package mesh provides Mesh networking functionality for WireSocket Server.
// It allows multiple Server nodes to form a network where clients connected to one node
// can access networks that only other nodes can reach.
//
// Multi-hop routing: Routes learned from peers are propagated transitively.
// If Node A connects to Node B, and Node B has routes to Node C's networks,
// Node A will learn those routes with Node B as the next hop.
package mesh

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"wire-socket-server/internal/database"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Config holds Mesh configuration
type Config struct {
	Enabled      bool
	Name         string
	Role         database.MeshNodeRole
	MeshIP       string
	Token        string
	SyncInterval time.Duration
	TunnelURL    string // This node's tunnel URL (from tunnel config)
	APIEndpoint  string // This node's API endpoint
}

// PeerConfig represents a peer to connect to
type PeerConfig struct {
	Name        string
	TunnelURL   string
	APIEndpoint string
}

// Manager manages the Mesh network
type Manager struct {
	config       Config
	db           *database.DB
	localNode    *database.MeshNode
	peers        map[uint]*Peer // nodeID -> Peer
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
	wgManager    WireGuardManager
	routeTable   *RouteTable
	learnedRoutes map[string]*LearnedRoute // CIDR -> route learned from peers
	httpClient   *http.Client
}

// WireGuardManager interface for WireGuard operations
type WireGuardManager interface {
	AddMeshPeer(publicKey string, meshIP string, allowedIPs []string, endpoint string) error
	RemovePeer(publicKey string) error
	GetDeviceName() string
	UpdateMeshPeerAllowedIPs(publicKey string, allowedIPs []string) error
}

// Peer represents a connected Mesh peer
type Peer struct {
	Node           *database.MeshNode
	IsOnline       bool
	LastSeen       time.Time
	RTT            time.Duration
	tunnel         *TunnelClient
	stopCh         chan struct{}
	learnedRoutes  []LearnedRoute // Routes learned from this peer (including transitive)
}

// RouteTable holds computed routes for the Mesh network
type RouteTable struct {
	mu     sync.RWMutex
	routes map[string]*RouteEntry // CIDR -> RouteEntry
}

// RouteEntry represents a route to a destination via a Mesh node
type RouteEntry struct {
	CIDR       string
	ViaNode    string // First hop node name
	ViaIP      string // First hop node IP
	Priority   int
	NodeID     uint   // First hop node ID
	Origin     string // Origin node name (for transitive routes)
	HopCount   int    // Number of hops to reach destination
}

// LearnedRoute represents a route learned from a peer
type LearnedRoute struct {
	CIDR     string `json:"cidr"`
	Priority int    `json:"priority"`
	Origin   string `json:"origin"`   // Which node owns this route
	HopCount int    `json:"hop_count"` // Hops from the peer to the origin
}

// SyncData represents data exchanged during sync
type SyncData struct {
	Node struct {
		Name   string `json:"name"`
		MeshIP string `json:"mesh_ip"`
	} `json:"node"`
	ExitRoutes []struct {
		CIDR     string `json:"cidr"`
		Priority int    `json:"priority"`
	} `json:"exit_routes"`
	LearnedRoutes []LearnedRoute `json:"learned_routes,omitempty"` // Transitive routes
}

// NewManager creates a new Mesh Manager
func NewManager(config Config, db *database.DB, wgManager WireGuardManager) (*Manager, error) {
	if !config.Enabled {
		return nil, nil
	}

	if config.Name == "" {
		return nil, fmt.Errorf("mesh.name is required when mesh is enabled")
	}
	if config.MeshIP == "" {
		return nil, fmt.Errorf("mesh.mesh_ip is required when mesh is enabled")
	}
	if config.Token == "" {
		return nil, fmt.Errorf("mesh.token is required when mesh is enabled")
	}

	// Default role is gateway
	if config.Role == "" {
		config.Role = database.MeshRoleGateway
	}

	// Default sync interval is 30 seconds
	if config.SyncInterval == 0 {
		config.SyncInterval = 30 * time.Second
	}

	m := &Manager{
		config:        config,
		db:            db,
		peers:         make(map[uint]*Peer),
		stopCh:        make(chan struct{}),
		wgManager:     wgManager,
		learnedRoutes: make(map[string]*LearnedRoute),
		routeTable: &RouteTable{
			routes: make(map[string]*RouteEntry),
		},
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	// Initialize or load local node
	if err := m.initLocalNode(); err != nil {
		return nil, fmt.Errorf("failed to initialize local node: %w", err)
	}

	return m, nil
}

// initLocalNode initializes or loads the local Mesh node
func (m *Manager) initLocalNode() error {
	var node database.MeshNode

	// Try to find existing local node
	result := m.db.Where("is_local = ?", true).First(&node)
	if result.Error == nil {
		// Update node info if name changed
		if node.Name != m.config.Name || node.MeshIP != m.config.MeshIP {
			node.Name = m.config.Name
			node.MeshIP = m.config.MeshIP
			node.TunnelURL = m.config.TunnelURL
			node.APIEndpoint = m.config.APIEndpoint
			if err := m.db.Save(&node).Error; err != nil {
				return err
			}
		}
		m.localNode = &node
		log.Printf("[mesh] Local node loaded: %s (%s)", node.Name, node.MeshIP)
		return nil
	}

	// Generate new keypair for Mesh
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}
	publicKey := privateKey.PublicKey()

	// Create new local node
	node = database.MeshNode{
		Name:        m.config.Name,
		PublicKey:   publicKey.String(),
		PrivateKey:  privateKey.String(),
		MeshIP:      m.config.MeshIP,
		TunnelURL:   m.config.TunnelURL,
		APIEndpoint: m.config.APIEndpoint,
		IsLocal:     true,
		IsOnline:    true,
	}

	if err := m.db.Create(&node).Error; err != nil {
		return fmt.Errorf("failed to create local node: %w", err)
	}

	m.localNode = &node
	log.Printf("[mesh] Local node created: %s (%s) pubkey=%s", node.Name, node.MeshIP, node.PublicKey)
	return nil
}

// Start begins the Mesh networking operations
func (m *Manager) Start() error {
	if m == nil {
		return nil
	}

	log.Printf("[mesh] Starting Mesh Manager: %s (%s) role=%s", m.config.Name, m.config.MeshIP, m.config.Role)

	// Load existing peers from database
	if err := m.loadPeers(); err != nil {
		log.Printf("[mesh] Warning: failed to load peers: %v", err)
	}

	// Start sync goroutine
	m.wg.Add(1)
	go m.syncLoop()

	return nil
}

// Stop stops the Mesh Manager
func (m *Manager) Stop() error {
	if m == nil {
		return nil
	}

	log.Printf("[mesh] Stopping Mesh Manager")
	close(m.stopCh)

	// Stop all peer connections
	m.mu.Lock()
	for _, peer := range m.peers {
		if peer.tunnel != nil {
			peer.tunnel.Close()
		}
		if peer.stopCh != nil {
			close(peer.stopCh)
		}
	}
	m.mu.Unlock()

	m.wg.Wait()
	return nil
}

// loadPeers loads peers from database and establishes connections
func (m *Manager) loadPeers() error {
	var nodes []database.MeshNode
	if err := m.db.Where("is_local = ?", false).Preload("ExitRoutes").Find(&nodes).Error; err != nil {
		return err
	}

	for _, node := range nodes {
		nodeCopy := node
		if err := m.connectToPeer(&nodeCopy); err != nil {
			log.Printf("[mesh] Failed to connect to peer %s: %v", node.Name, err)
		}
	}

	return nil
}

// connectToPeer establishes connection to a peer node
func (m *Manager) connectToPeer(node *database.MeshNode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already connected
	if _, exists := m.peers[node.ID]; exists {
		return nil
	}

	peer := &Peer{
		Node:   node,
		stopCh: make(chan struct{}),
	}

	// Create tunnel client
	if node.TunnelURL != "" {
		tunnel, err := NewTunnelClient(node.TunnelURL, m.localNode.PrivateKey, node.PublicKey)
		if err != nil {
			return fmt.Errorf("failed to create tunnel client: %w", err)
		}
		peer.tunnel = tunnel
	}

	m.peers[node.ID] = peer

	// Build AllowedIPs: mesh IP + direct exit routes
	allowedIPs := m.buildAllowedIPs(node)

	// If we have a tunnel, the endpoint is the local tunnel port
	endpoint := ""
	if peer.tunnel != nil {
		endpoint = peer.tunnel.GetLocalEndpoint()
	}

	if err := m.wgManager.AddMeshPeer(node.PublicKey, node.MeshIP, allowedIPs, endpoint); err != nil {
		return fmt.Errorf("failed to add WireGuard peer: %w", err)
	}

	// Start tunnel if available
	if peer.tunnel != nil {
		if err := peer.tunnel.Start(); err != nil {
			log.Printf("[mesh] Failed to start tunnel to %s: %v", node.Name, err)
		}
	}

	// Update route table
	m.updateRouteTableLocked()

	log.Printf("[mesh] Connected to peer: %s (%s)", node.Name, node.MeshIP)
	return nil
}

// buildAllowedIPs constructs the AllowedIPs list for a peer
func (m *Manager) buildAllowedIPs(node *database.MeshNode) []string {
	allowedIPs := []string{node.MeshIP + "/32"}

	// Add direct exit routes
	for _, route := range node.ExitRoutes {
		if route.Enabled {
			allowedIPs = append(allowedIPs, route.CIDR)
		}
	}

	// Add learned routes from this peer
	m.mu.RLock()
	peer := m.peers[node.ID]
	m.mu.RUnlock()

	if peer != nil {
		for _, lr := range peer.learnedRoutes {
			// Avoid duplicates
			found := false
			for _, ip := range allowedIPs {
				if ip == lr.CIDR {
					found = true
					break
				}
			}
			if !found {
				allowedIPs = append(allowedIPs, lr.CIDR)
			}
		}
	}

	return allowedIPs
}

// syncLoop periodically syncs with peers
func (m *Manager) syncLoop() {
	defer m.wg.Done()

	// Initial sync after a short delay
	time.Sleep(5 * time.Second)
	m.syncWithPeers()

	ticker := time.NewTicker(m.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.syncWithPeers()
		case <-m.stopCh:
			return
		}
	}
}

// syncWithPeers syncs route information with all peers
func (m *Manager) syncWithPeers() {
	m.mu.RLock()
	peers := make([]*Peer, 0, len(m.peers))
	for _, peer := range m.peers {
		peers = append(peers, peer)
	}
	m.mu.RUnlock()

	routesChanged := false
	for _, peer := range peers {
		changed, err := m.syncWithPeer(peer)
		if err != nil {
			log.Printf("[mesh] Sync with %s failed: %v", peer.Node.Name, err)
			peer.IsOnline = false
		} else {
			peer.IsOnline = true
			peer.LastSeen = time.Now()
			if changed {
				routesChanged = true
			}
		}
	}

	// Update route table and WireGuard peers if routes changed
	if routesChanged {
		m.mu.Lock()
		m.updateRouteTableLocked()
		m.updateWireGuardPeers()
		m.mu.Unlock()
	}

	// Save peer status to database
	for _, peer := range peers {
		now := time.Now()
		peer.Node.LastSeen = &now
		peer.Node.IsOnline = peer.IsOnline
		m.db.Save(peer.Node)
	}
}

// syncWithPeer syncs with a single peer via HTTP API
func (m *Manager) syncWithPeer(peer *Peer) (bool, error) {
	if peer.Node.APIEndpoint == "" {
		// No API endpoint, just mark as online
		return false, nil
	}

	// Fetch sync data from peer
	syncData, err := m.fetchSyncData(peer.Node.APIEndpoint)
	if err != nil {
		return false, err
	}

	// Measure RTT
	start := time.Now()
	peer.RTT = time.Since(start)

	// Process learned routes
	oldRoutes := peer.learnedRoutes
	peer.learnedRoutes = m.processLearnedRoutes(peer, syncData)

	// Check if routes changed
	routesChanged := !routesEqual(oldRoutes, peer.learnedRoutes)

	// Update peer's exit routes in database if they changed
	m.updatePeerExitRoutes(peer, syncData)

	return routesChanged, nil
}

// fetchSyncData fetches sync data from a peer's API
func (m *Manager) fetchSyncData(apiEndpoint string) (*SyncData, error) {
	url := apiEndpoint + "/api/mesh/sync-data"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Mesh-Token", m.config.Token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sync data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sync API returned %d: %s", resp.StatusCode, string(body))
	}

	var syncData SyncData
	if err := json.NewDecoder(resp.Body).Decode(&syncData); err != nil {
		return nil, fmt.Errorf("failed to decode sync data: %w", err)
	}

	return &syncData, nil
}

// processLearnedRoutes processes routes learned from a peer
func (m *Manager) processLearnedRoutes(peer *Peer, syncData *SyncData) []LearnedRoute {
	var routes []LearnedRoute

	// Add peer's direct exit routes
	for _, er := range syncData.ExitRoutes {
		routes = append(routes, LearnedRoute{
			CIDR:     er.CIDR,
			Priority: er.Priority,
			Origin:   peer.Node.Name,
			HopCount: 1,
		})
	}

	// Add transitive routes (routes the peer learned from others)
	for _, lr := range syncData.LearnedRoutes {
		// Skip routes that originate from us (avoid loops)
		if lr.Origin == m.config.Name {
			continue
		}

		routes = append(routes, LearnedRoute{
			CIDR:     lr.CIDR,
			Priority: lr.Priority + 10, // Lower priority for transitive routes
			Origin:   lr.Origin,
			HopCount: lr.HopCount + 1,
		})
	}

	return routes
}

// updatePeerExitRoutes updates a peer's exit routes in the database
func (m *Manager) updatePeerExitRoutes(peer *Peer, syncData *SyncData) {
	// Get current routes from database
	var existingRoutes []database.ExitRoute
	m.db.Where("node_id = ?", peer.Node.ID).Find(&existingRoutes)

	// Build map of existing routes
	existingMap := make(map[string]*database.ExitRoute)
	for i := range existingRoutes {
		existingMap[existingRoutes[i].CIDR] = &existingRoutes[i]
	}

	// Add/update routes from sync data
	for _, er := range syncData.ExitRoutes {
		if existing, ok := existingMap[er.CIDR]; ok {
			// Update priority if changed
			if existing.Priority != er.Priority {
				existing.Priority = er.Priority
				m.db.Save(existing)
			}
			delete(existingMap, er.CIDR)
		} else {
			// Add new route
			newRoute := &database.ExitRoute{
				NodeID:   peer.Node.ID,
				CIDR:     er.CIDR,
				Priority: er.Priority,
				Enabled:  true,
			}
			m.db.Create(newRoute)
		}
	}

	// Routes remaining in existingMap are no longer advertised - disable them
	for _, route := range existingMap {
		route.Enabled = false
		m.db.Save(route)
	}
}

// updateRouteTableLocked rebuilds the route table (must be called with m.mu held)
func (m *Manager) updateRouteTableLocked() {
	m.routeTable.mu.Lock()
	defer m.routeTable.mu.Unlock()

	// Clear existing routes
	m.routeTable.routes = make(map[string]*RouteEntry)

	// Add routes from all online peers
	for _, peer := range m.peers {
		if !peer.IsOnline {
			continue
		}

		// Add direct exit routes
		for _, route := range peer.Node.ExitRoutes {
			if !route.Enabled {
				continue
			}

			m.addRouteIfBetter(route.CIDR, peer.Node.Name, peer.Node.MeshIP,
				route.Priority, peer.Node.ID, peer.Node.Name, 1)
		}

		// Add learned routes (transitive)
		for _, lr := range peer.learnedRoutes {
			m.addRouteIfBetter(lr.CIDR, peer.Node.Name, peer.Node.MeshIP,
				lr.Priority, peer.Node.ID, lr.Origin, lr.HopCount)
		}
	}
}

// addRouteIfBetter adds a route if it's better than existing
func (m *Manager) addRouteIfBetter(cidr, viaNode, viaIP string, priority int, nodeID uint, origin string, hopCount int) {
	existing, exists := m.routeTable.routes[cidr]

	// Compare routes: lower priority number = better, fewer hops = better
	if exists {
		// Priority takes precedence, then hop count
		if existing.Priority < priority {
			return
		}
		if existing.Priority == priority && existing.HopCount <= hopCount {
			return
		}
	}

	m.routeTable.routes[cidr] = &RouteEntry{
		CIDR:     cidr,
		ViaNode:  viaNode,
		ViaIP:    viaIP,
		Priority: priority,
		NodeID:   nodeID,
		Origin:   origin,
		HopCount: hopCount,
	}
}

// updateWireGuardPeers updates WireGuard peer AllowedIPs based on route table
func (m *Manager) updateWireGuardPeers() {
	for _, peer := range m.peers {
		allowedIPs := m.buildAllowedIPs(peer.Node)
		if err := m.wgManager.UpdateMeshPeerAllowedIPs(peer.Node.PublicKey, allowedIPs); err != nil {
			log.Printf("[mesh] Failed to update AllowedIPs for %s: %v", peer.Node.Name, err)
		}
	}
}

// updateRouteTable is the public version that acquires lock
func (m *Manager) updateRouteTable() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateRouteTableLocked()
}

// GetRouteTable returns a copy of the current route table
func (m *Manager) GetRouteTable() []RouteEntry {
	if m == nil {
		return nil
	}

	m.routeTable.mu.RLock()
	defer m.routeTable.mu.RUnlock()

	routes := make([]RouteEntry, 0, len(m.routeTable.routes))
	for _, route := range m.routeTable.routes {
		routes = append(routes, *route)
	}
	return routes
}

// GetLearnedRoutes returns routes to advertise to other peers
func (m *Manager) GetLearnedRoutes() []LearnedRoute {
	if m == nil {
		return nil
	}

	m.routeTable.mu.RLock()
	defer m.routeTable.mu.RUnlock()

	var routes []LearnedRoute
	for _, entry := range m.routeTable.routes {
		routes = append(routes, LearnedRoute{
			CIDR:     entry.CIDR,
			Priority: entry.Priority,
			Origin:   entry.Origin,
			HopCount: entry.HopCount,
		})
	}
	return routes
}

// GetLocalNode returns the local node info
func (m *Manager) GetLocalNode() *database.MeshNode {
	if m == nil {
		return nil
	}
	return m.localNode
}

// GetPeers returns all peer nodes
func (m *Manager) GetPeers() []database.MeshNode {
	if m == nil {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	peers := make([]database.MeshNode, 0, len(m.peers))
	for _, peer := range m.peers {
		peers = append(peers, *peer.Node)
	}
	return peers
}

// AddPeer adds a new peer to the Mesh network
func (m *Manager) AddPeer(tunnelURL, apiEndpoint string) (*database.MeshNode, error) {
	// Perform handshake with the peer
	peerInfo, err := m.handshake(apiEndpoint)
	if err != nil {
		return nil, fmt.Errorf("handshake failed: %w", err)
	}

	// Check if peer already exists
	var existing database.MeshNode
	if err := m.db.Where("public_key = ?", peerInfo.PublicKey).First(&existing).Error; err == nil {
		// Update existing node
		existing.TunnelURL = tunnelURL
		existing.APIEndpoint = apiEndpoint
		existing.IsOnline = true
		now := time.Now()
		existing.LastSeen = &now
		if err := m.db.Save(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, m.connectToPeer(&existing)
	}

	// Create new node
	node := &database.MeshNode{
		Name:        peerInfo.Name,
		PublicKey:   peerInfo.PublicKey,
		MeshIP:      peerInfo.MeshIP,
		TunnelURL:   tunnelURL,
		APIEndpoint: apiEndpoint,
		IsLocal:     false,
		IsOnline:    true,
	}

	now := time.Now()
	node.LastSeen = &now

	if err := m.db.Create(node).Error; err != nil {
		return nil, err
	}

	// Add exit routes from peer
	for _, route := range peerInfo.ExitRoutes {
		exitRoute := &database.ExitRoute{
			NodeID:   node.ID,
			CIDR:     route.CIDR,
			Comment:  route.Comment,
			Priority: route.Priority,
			Enabled:  true,
		}
		m.db.Create(exitRoute)
	}

	// Reload with exit routes
	m.db.Preload("ExitRoutes").First(node, node.ID)

	return node, m.connectToPeer(node)
}

// RemovePeer removes a peer from the Mesh network
func (m *Manager) RemovePeer(nodeID uint) error {
	m.mu.Lock()
	peer, exists := m.peers[nodeID]
	if exists {
		// Stop tunnel
		if peer.tunnel != nil {
			peer.tunnel.Close()
		}
		// Remove from WireGuard
		m.wgManager.RemovePeer(peer.Node.PublicKey)
		delete(m.peers, nodeID)
	}
	m.mu.Unlock()

	// Delete from database
	m.db.Where("node_id = ?", nodeID).Delete(&database.ExitRoute{})
	return m.db.Delete(&database.MeshNode{}, nodeID).Error
}

// AddExitRoute adds an exit route for the local node
func (m *Manager) AddExitRoute(cidr, comment string, priority int) (*database.ExitRoute, error) {
	if m.localNode == nil {
		return nil, fmt.Errorf("local node not initialized")
	}

	route := &database.ExitRoute{
		NodeID:   m.localNode.ID,
		CIDR:     cidr,
		Comment:  comment,
		Priority: priority,
		Enabled:  true,
	}

	if err := m.db.Create(route).Error; err != nil {
		return nil, err
	}

	return route, nil
}

// GetExitRoutes returns exit routes for the local node
func (m *Manager) GetExitRoutes() ([]database.ExitRoute, error) {
	if m.localNode == nil {
		return nil, fmt.Errorf("local node not initialized")
	}

	var routes []database.ExitRoute
	err := m.db.Where("node_id = ?", m.localNode.ID).Find(&routes).Error
	return routes, err
}

// DeleteExitRoute deletes an exit route
func (m *Manager) DeleteExitRoute(routeID uint) error {
	return m.db.Delete(&database.ExitRoute{}, routeID).Error
}

// IsGateway returns true if this node accepts client connections
func (m *Manager) IsGateway() bool {
	if m == nil {
		return true // Default behavior when Mesh is disabled
	}
	return m.config.Role == database.MeshRoleGateway || m.config.Role == database.MeshRoleBoth
}

// GetConfig returns the Mesh configuration
func (m *Manager) GetConfig() Config {
	if m == nil {
		return Config{}
	}
	return m.config
}

// TriggerSync manually triggers a sync with all peers
func (m *Manager) TriggerSync() {
	go m.syncWithPeers()
}

// HandshakeInfo contains information exchanged during handshake
type HandshakeInfo struct {
	Name       string
	PublicKey  string
	MeshIP     string
	TunnelURL  string
	ExitRoutes []struct {
		CIDR     string
		Comment  string
		Priority int
	}
}

// handshake performs handshake with a peer via HTTP API
func (m *Manager) handshake(apiEndpoint string) (*HandshakeInfo, error) {
	url := apiEndpoint + "/api/mesh/handshake"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Mesh-Token", m.config.Token)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform handshake: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("handshake returned %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Name       string `json:"name"`
		PublicKey  string `json:"public_key"`
		MeshIP     string `json:"mesh_ip"`
		TunnelURL  string `json:"tunnel_url"`
		ExitRoutes []struct {
			CIDR     string `json:"cidr"`
			Comment  string `json:"comment"`
			Priority int    `json:"priority"`
		} `json:"exit_routes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode handshake response: %w", err)
	}

	info := &HandshakeInfo{
		Name:      response.Name,
		PublicKey: response.PublicKey,
		MeshIP:    response.MeshIP,
		TunnelURL: response.TunnelURL,
	}

	for _, er := range response.ExitRoutes {
		info.ExitRoutes = append(info.ExitRoutes, struct {
			CIDR     string
			Comment  string
			Priority int
		}{
			CIDR:     er.CIDR,
			Comment:  er.Comment,
			Priority: er.Priority,
		})
	}

	return info, nil
}

// routesEqual compares two route slices for equality
func routesEqual(a, b []LearnedRoute) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]LearnedRoute)
	for _, r := range a {
		aMap[r.CIDR] = r
	}

	for _, r := range b {
		ar, ok := aMap[r.CIDR]
		if !ok {
			return false
		}
		if ar.Priority != r.Priority || ar.Origin != r.Origin || ar.HopCount != r.HopCount {
			return false
		}
	}

	return true
}
