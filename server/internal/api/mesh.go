package api

import (
	"net/http"
	"strconv"

	"wire-socket-server/internal/mesh"

	"github.com/gin-gonic/gin"
)

// MeshHandler handles Mesh-related API endpoints
type MeshHandler struct {
	meshManager *mesh.Manager
	meshToken   string
}

// NewMeshHandler creates a new Mesh API handler
func NewMeshHandler(meshManager *mesh.Manager, meshToken string) *MeshHandler {
	return &MeshHandler{
		meshManager: meshManager,
		meshToken:   meshToken,
	}
}

// MeshTokenMiddleware validates the X-Mesh-Token header for inter-node communication
func (h *MeshHandler) MeshTokenMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("X-Mesh-Token")
		if token == "" || token != h.meshToken {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid mesh token"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// ======== Admin API Endpoints ========

// GetStatus returns the Mesh network status
// GET /api/mesh/status
func (h *MeshHandler) GetStatus(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusOK, gin.H{
			"enabled": false,
		})
		return
	}

	localNode := h.meshManager.GetLocalNode()
	peers := h.meshManager.GetPeers()
	routes := h.meshManager.GetRouteTable()
	config := h.meshManager.GetConfig()

	c.JSON(http.StatusOK, gin.H{
		"enabled": true,
		"local_node": gin.H{
			"name":      localNode.Name,
			"mesh_ip":   localNode.MeshIP,
			"tunnel_url": localNode.TunnelURL,
			"role":      config.Role,
		},
		"peers":         peers,
		"routing_table": routes,
	})
}

// ListPeers returns all Mesh peers
// GET /api/mesh/peers
func (h *MeshHandler) ListPeers(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	peers := h.meshManager.GetPeers()
	c.JSON(http.StatusOK, gin.H{"peers": peers})
}

// AddPeerRequest represents the request body for adding a peer
type AddPeerRequest struct {
	TunnelURL   string `json:"tunnel_url" binding:"required"`
	APIEndpoint string `json:"api_endpoint" binding:"required"`
}

// AddPeer adds a new Mesh peer
// POST /api/mesh/peers
func (h *MeshHandler) AddPeer(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	var req AddPeerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	peer, err := h.meshManager.AddPeer(req.TunnelURL, req.APIEndpoint)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"peer": peer})
}

// RemovePeer removes a Mesh peer
// DELETE /api/mesh/peers/:id
func (h *MeshHandler) RemovePeer(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid peer id"})
		return
	}

	if err := h.meshManager.RemovePeer(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "peer removed"})
}

// ListExitRoutes returns exit routes for the local node
// GET /api/mesh/exit-routes
func (h *MeshHandler) ListExitRoutes(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	routes, err := h.meshManager.GetExitRoutes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"exit_routes": routes})
}

// AddExitRouteRequest represents the request body for adding an exit route
type AddExitRouteRequest struct {
	CIDR     string `json:"cidr" binding:"required"`
	Comment  string `json:"comment"`
	Priority int    `json:"priority"`
}

// AddExitRoute adds an exit route for the local node
// POST /api/mesh/exit-routes
func (h *MeshHandler) AddExitRoute(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	var req AddExitRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	priority := req.Priority
	if priority == 0 {
		priority = 100 // Default priority
	}

	route, err := h.meshManager.AddExitRoute(req.CIDR, req.Comment, priority)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"exit_route": route})
}

// DeleteExitRoute removes an exit route
// DELETE /api/mesh/exit-routes/:id
func (h *MeshHandler) DeleteExitRoute(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid route id"})
		return
	}

	if err := h.meshManager.DeleteExitRoute(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "exit route removed"})
}

// TriggerSync manually triggers a sync with all peers
// POST /api/mesh/sync
func (h *MeshHandler) TriggerSync(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	h.meshManager.TriggerSync()
	c.JSON(http.StatusOK, gin.H{"message": "sync triggered"})
}

// ======== Inter-Node API Endpoints ========

// Handshake handles handshake requests from other Mesh nodes
// GET /api/mesh/handshake
func (h *MeshHandler) Handshake(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	localNode := h.meshManager.GetLocalNode()
	exitRoutes, _ := h.meshManager.GetExitRoutes()

	// Build exit routes response
	routesResp := make([]gin.H, 0, len(exitRoutes))
	for _, route := range exitRoutes {
		if route.Enabled {
			routesResp = append(routesResp, gin.H{
				"cidr":     route.CIDR,
				"comment":  route.Comment,
				"priority": route.Priority,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"name":        localNode.Name,
		"public_key":  localNode.PublicKey,
		"mesh_ip":     localNode.MeshIP,
		"tunnel_url":  localNode.TunnelURL,
		"exit_routes": routesResp,
	})
}

// RegisterRequest represents a registration request from another node
type RegisterRequest struct {
	Name       string   `json:"name" binding:"required"`
	PublicKey  string   `json:"public_key" binding:"required"`
	MeshIP     string   `json:"mesh_ip" binding:"required"`
	TunnelURL  string   `json:"tunnel_url"`
	ExitRoutes []string `json:"exit_routes"`
}

// Register handles registration requests from other Mesh nodes
// POST /api/mesh/register
func (h *MeshHandler) Register(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Process registration and add peer
	c.JSON(http.StatusOK, gin.H{"message": "registered"})
}

// GetSyncData returns sync data for other Mesh nodes
// GET /api/mesh/sync-data
func (h *MeshHandler) GetSyncData(c *gin.Context) {
	if h.meshManager == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mesh not enabled"})
		return
	}

	localNode := h.meshManager.GetLocalNode()
	exitRoutes, _ := h.meshManager.GetExitRoutes()

	// Build direct exit routes response
	routesResp := make([]gin.H, 0, len(exitRoutes))
	for _, route := range exitRoutes {
		if route.Enabled {
			routesResp = append(routesResp, gin.H{
				"cidr":     route.CIDR,
				"priority": route.Priority,
			})
		}
	}

	// Build learned routes response (for transitive routing)
	learnedRoutes := h.meshManager.GetLearnedRoutes()
	learnedResp := make([]gin.H, 0, len(learnedRoutes))
	for _, lr := range learnedRoutes {
		learnedResp = append(learnedResp, gin.H{
			"cidr":      lr.CIDR,
			"priority":  lr.Priority,
			"origin":    lr.Origin,
			"hop_count": lr.HopCount,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"node": gin.H{
			"name":    localNode.Name,
			"mesh_ip": localNode.MeshIP,
		},
		"exit_routes":    routesResp,
		"learned_routes": learnedResp,
	})
}

// GetMeshRoutes returns all mesh routes as CIDR strings for client configuration
// This is used by the router to include mesh routes in the client's VPN config
func (h *MeshHandler) GetMeshRoutes() []string {
	if h.meshManager == nil {
		return nil
	}

	routeTable := h.meshManager.GetRouteTable()
	routes := make([]string, 0, len(routeTable))
	for _, entry := range routeTable {
		routes = append(routes, entry.CIDR)
	}
	return routes
}
