package tunnelservice

import (
	"net/http"
	"strconv"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/nat"
	"wire-socket-server/internal/route"

	"github.com/gin-gonic/gin"
)

// AdminHandler handles local admin API endpoints
type AdminHandler struct {
	db            *database.TunnelDB
	natManager    *nat.Manager
	routeManager  *route.Manager
	defaultDevice string
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(db *database.TunnelDB, natManager *nat.Manager, defaultDevice string) *AdminHandler {
	return &AdminHandler{
		db:            db,
		natManager:    natManager,
		defaultDevice: defaultDevice,
	}
}

// ============ Route Management ============

// ListRoutes returns all routes
func (h *AdminHandler) ListRoutes(c *gin.Context) {
	var routes []database.TunnelRoute
	if err := h.db.Find(&routes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch routes"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"routes": routes})
}

// CreateRouteRequest for creating a route
type CreateRouteRequest struct {
	CIDR          string `json:"cidr" binding:"required"`
	Gateway       string `json:"gateway"`
	Device        string `json:"device"`
	Metric        int    `json:"metric"`
	Comment       string `json:"comment"`
	Enabled       *bool  `json:"enabled"`
	PushToClient  *bool  `json:"push_to_client"`
	ApplyOnServer *bool  `json:"apply_on_server"`
}

// CreateRoute creates a new route
func (h *AdminHandler) CreateRoute(c *gin.Context) {
	var req CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	enabled := true
	pushToClient := true
	applyOnServer := false

	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.PushToClient != nil {
		pushToClient = *req.PushToClient
	}
	if req.ApplyOnServer != nil {
		applyOnServer = *req.ApplyOnServer
	}

	dbRoute := database.TunnelRoute{
		CIDR:          req.CIDR,
		Gateway:       req.Gateway,
		Device:        req.Device,
		Metric:        req.Metric,
		Comment:       req.Comment,
		Enabled:       enabled,
		PushToClient:  pushToClient,
		ApplyOnServer: applyOnServer,
	}

	if err := h.db.Create(&dbRoute).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create route"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"route": dbRoute})
}

// UpdateRoute updates a route
func (h *AdminHandler) UpdateRoute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid route id"})
		return
	}

	var dbRoute database.TunnelRoute
	if err := h.db.First(&dbRoute, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	var req CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.CIDR != "" {
		dbRoute.CIDR = req.CIDR
	}
	dbRoute.Gateway = req.Gateway
	dbRoute.Device = req.Device
	dbRoute.Metric = req.Metric
	dbRoute.Comment = req.Comment

	if req.Enabled != nil {
		dbRoute.Enabled = *req.Enabled
	}
	if req.PushToClient != nil {
		dbRoute.PushToClient = *req.PushToClient
	}
	if req.ApplyOnServer != nil {
		dbRoute.ApplyOnServer = *req.ApplyOnServer
	}

	if err := h.db.Save(&dbRoute).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update route"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"route": dbRoute})
}

// DeleteRoute deletes a route
func (h *AdminHandler) DeleteRoute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid route id"})
		return
	}

	if err := h.db.Delete(&database.TunnelRoute{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete route"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "route deleted"})
}

// ApplyRoutes applies server-side routes
func (h *AdminHandler) ApplyRoutes(c *gin.Context) {
	var dbRoutes []database.TunnelRoute
	if err := h.db.Where("enabled = ? AND apply_on_server = ?", true, true).Find(&dbRoutes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch routes"})
		return
	}

	if len(dbRoutes) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "No routes to apply", "routes_count": 0})
		return
	}

	var routes []route.Route
	for _, r := range dbRoutes {
		routes = append(routes, route.Route{
			CIDR:    r.CIDR,
			Gateway: r.Gateway,
			Device:  r.Device,
			Metric:  r.Metric,
		})
	}

	routeConfig := route.Config{
		DefaultDevice: h.defaultDevice,
		Routes:        routes,
	}

	if h.routeManager != nil {
		h.routeManager.Cleanup()
	}

	newManager := route.NewManager(routeConfig)
	if err := newManager.Apply(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply routes: " + err.Error()})
		return
	}

	h.routeManager = newManager

	c.JSON(http.StatusOK, gin.H{"message": "Routes applied", "routes_count": len(routes)})
}

// ============ NAT Management ============

// ListNATRules returns all NAT rules
func (h *AdminHandler) ListNATRules(c *gin.Context) {
	var rules []database.TunnelNATRule
	if err := h.db.Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch NAT rules"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"nat_rules": rules})
}

// CreateNATRule creates a NAT rule
func (h *AdminHandler) CreateNATRule(c *gin.Context) {
	var rule database.TunnelNATRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	rule.Enabled = true
	if err := h.db.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create NAT rule"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"nat_rule": rule})
}

// UpdateNATRule updates a NAT rule
func (h *AdminHandler) UpdateNATRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule id"})
		return
	}

	var rule database.TunnelNATRule
	if err := h.db.First(&rule, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}

	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.Save(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"nat_rule": rule})
}

// DeleteNATRule deletes a NAT rule
func (h *AdminHandler) DeleteNATRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule id"})
		return
	}

	if err := h.db.Delete(&database.TunnelNATRule{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "rule deleted"})
}

// ApplyNATRules applies NAT rules
func (h *AdminHandler) ApplyNATRules(c *gin.Context) {
	if h.natManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "NAT manager not initialized"})
		return
	}

	var rules []database.TunnelNATRule
	if err := h.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch NAT rules"})
		return
	}

	config := nat.Config{Enabled: true}

	for _, rule := range rules {
		switch rule.Type {
		case database.TunnelNATTypeMasquerade:
			config.Masquerade = append(config.Masquerade, nat.MasqueradeRule{Interface: rule.Interface})
		case database.TunnelNATTypeSNAT:
			config.SNAT = append(config.SNAT, nat.SNATRule{
				Source:      rule.Source,
				Destination: rule.Destination,
				Interface:   rule.Interface,
				ToSource:    rule.ToSource,
			})
		case database.TunnelNATTypeDNAT:
			config.DNAT = append(config.DNAT, nat.DNATRule{
				Interface:     rule.Interface,
				Protocol:      rule.Protocol,
				Port:          rule.Port,
				ToDestination: rule.ToDestination,
			})
		case database.TunnelNATTypeTCPMSS:
			config.TCPMSS = append(config.TCPMSS, nat.TCPMSSRule{
				Interface: rule.Interface,
				Source:    rule.Source,
				MSS:       rule.MSS,
			})
		}
	}

	h.natManager.Cleanup()
	newManager := nat.NewManager(config)
	if err := newManager.Apply(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply NAT rules: " + err.Error()})
		return
	}

	*h.natManager = *newManager

	c.JSON(http.StatusOK, gin.H{
		"message":    "NAT rules applied",
		"masquerade": len(config.Masquerade),
		"snat":       len(config.SNAT),
		"dnat":       len(config.DNAT),
		"tcpmss":     len(config.TCPMSS),
	})
}
