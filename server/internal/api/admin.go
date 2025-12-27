package api

import (
	"net/http"
	"strconv"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/nat"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// AdminHandler handles admin API endpoints
type AdminHandler struct {
	db         *database.DB
	natManager *nat.Manager
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(db *database.DB, natManager *nat.Manager) *AdminHandler {
	return &AdminHandler{
		db:         db,
		natManager: natManager,
	}
}

// SetNATManager sets the NAT manager (for dynamic updates)
func (h *AdminHandler) SetNATManager(natManager *nat.Manager) {
	h.natManager = natManager
}

// ============ User Management ============

// ListUsers returns all users
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var users []database.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// GetUser returns a specific user
func (h *AdminHandler) GetUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var user database.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// UpdateUser updates a user
func (h *AdminHandler) UpdateUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var user database.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		IsActive *bool  `json:"is_active"`
		IsAdmin  *bool  `json:"is_admin"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Update fields if provided
	if req.Username != "" {
		user.Username = req.Username
	}
	if req.Email != "" {
		user.Email = req.Email
	}
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
			return
		}
		user.PasswordHash = string(hashedPassword)
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if req.IsAdmin != nil {
		user.IsAdmin = *req.IsAdmin
	}

	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": user})
}

// DeleteUser deletes a user
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var user database.User
	if err := h.db.First(&user, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Delete user's IP allocations
	h.db.Where("user_id = ?", user.ID).Delete(&database.AllocatedIP{})

	// Delete user's sessions
	h.db.Where("user_id = ?", user.ID).Delete(&database.Session{})

	// Delete user
	if err := h.db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted successfully"})
}

// ============ Route Management ============

// ListRoutes returns all routes
func (h *AdminHandler) ListRoutes(c *gin.Context) {
	var routes []database.Route
	if err := h.db.Find(&routes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch routes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"routes": routes})
}

// CreateRoute creates a new route
func (h *AdminHandler) CreateRoute(c *gin.Context) {
	var req struct {
		CIDR    string `json:"cidr" binding:"required"`
		Comment string `json:"comment"`
		Enabled *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	route := database.Route{
		CIDR:    req.CIDR,
		Comment: req.Comment,
		Enabled: enabled,
	}

	if err := h.db.Create(&route).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "route already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"route": route})
}

// UpdateRoute updates a route
func (h *AdminHandler) UpdateRoute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid route id"})
		return
	}

	var route database.Route
	if err := h.db.First(&route, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	var req struct {
		CIDR    string `json:"cidr"`
		Comment string `json:"comment"`
		Enabled *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.CIDR != "" {
		route.CIDR = req.CIDR
	}
	if req.Comment != "" {
		route.Comment = req.Comment
	}
	if req.Enabled != nil {
		route.Enabled = *req.Enabled
	}

	if err := h.db.Save(&route).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update route"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"route": route})
}

// DeleteRoute deletes a route
func (h *AdminHandler) DeleteRoute(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid route id"})
		return
	}

	var route database.Route
	if err := h.db.First(&route, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	if err := h.db.Delete(&route).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete route"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "route deleted successfully"})
}

// GetEnabledRoutes returns all enabled routes (for internal use)
func (h *AdminHandler) GetEnabledRoutes() ([]string, error) {
	var routes []database.Route
	if err := h.db.Where("enabled = ?", true).Find(&routes).Error; err != nil {
		return nil, err
	}

	cidrs := make([]string, len(routes))
	for i, route := range routes {
		cidrs[i] = route.CIDR
	}
	return cidrs, nil
}

// ============ NAT Rule Management ============

// ListNATRules returns all NAT rules
func (h *AdminHandler) ListNATRules(c *gin.Context) {
	var rules []database.NATRule
	if err := h.db.Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch NAT rules"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"nat_rules": rules})
}

// CreateNATRule creates a new NAT rule
func (h *AdminHandler) CreateNATRule(c *gin.Context) {
	var req struct {
		Type          database.NATRuleType `json:"type" binding:"required"`
		Comment       string               `json:"comment"`
		Enabled       *bool                `json:"enabled"`
		Interface     string               `json:"interface"`
		Source        string               `json:"source"`
		Destination   string               `json:"destination"`
		ToSource      string               `json:"to_source"`
		Protocol      string               `json:"protocol"`
		Port          int                  `json:"port"`
		ToDestination string               `json:"to_destination"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Validate rule type
	switch req.Type {
	case database.NATTypeMasquerade:
		if req.Interface == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "interface is required for masquerade rule"})
			return
		}
	case database.NATTypeSNAT:
		if req.Source == "" || req.Destination == "" || req.Interface == "" || req.ToSource == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "source, destination, interface, and to_source are required for SNAT rule"})
			return
		}
	case database.NATTypeDNAT:
		if req.Interface == "" || req.Protocol == "" || req.Port == 0 || req.ToDestination == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "interface, protocol, port, and to_destination are required for DNAT rule"})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid rule type"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	rule := database.NATRule{
		Type:          req.Type,
		Comment:       req.Comment,
		Enabled:       enabled,
		Interface:     req.Interface,
		Source:        req.Source,
		Destination:   req.Destination,
		ToSource:      req.ToSource,
		Protocol:      req.Protocol,
		Port:          req.Port,
		ToDestination: req.ToDestination,
	}

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid NAT rule id"})
		return
	}

	var rule database.NATRule
	if err := h.db.First(&rule, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "NAT rule not found"})
		return
	}

	var req struct {
		Comment       string `json:"comment"`
		Enabled       *bool  `json:"enabled"`
		Interface     string `json:"interface"`
		Source        string `json:"source"`
		Destination   string `json:"destination"`
		ToSource      string `json:"to_source"`
		Protocol      string `json:"protocol"`
		Port          int    `json:"port"`
		ToDestination string `json:"to_destination"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Comment != "" {
		rule.Comment = req.Comment
	}
	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}
	if req.Interface != "" {
		rule.Interface = req.Interface
	}
	if req.Source != "" {
		rule.Source = req.Source
	}
	if req.Destination != "" {
		rule.Destination = req.Destination
	}
	if req.ToSource != "" {
		rule.ToSource = req.ToSource
	}
	if req.Protocol != "" {
		rule.Protocol = req.Protocol
	}
	if req.Port != 0 {
		rule.Port = req.Port
	}
	if req.ToDestination != "" {
		rule.ToDestination = req.ToDestination
	}

	if err := h.db.Save(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update NAT rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"nat_rule": rule})
}

// DeleteNATRule deletes a NAT rule
func (h *AdminHandler) DeleteNATRule(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid NAT rule id"})
		return
	}

	var rule database.NATRule
	if err := h.db.First(&rule, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "NAT rule not found"})
		return
	}

	if err := h.db.Delete(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete NAT rule"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "NAT rule deleted successfully"})
}

// ApplyNATRules reloads and applies all NAT rules from the database
func (h *AdminHandler) ApplyNATRules(c *gin.Context) {
	if h.natManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "NAT manager not initialized"})
		return
	}

	// Load all enabled NAT rules from database
	var rules []database.NATRule
	if err := h.db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch NAT rules"})
		return
	}

	// Build NAT config from database rules
	config := nat.Config{
		Enabled: true,
	}

	for _, rule := range rules {
		switch rule.Type {
		case database.NATTypeMasquerade:
			config.Masquerade = append(config.Masquerade, nat.MasqueradeRule{
				Interface: rule.Interface,
			})
		case database.NATTypeSNAT:
			config.SNAT = append(config.SNAT, nat.SNATRule{
				Source:      rule.Source,
				Destination: rule.Destination,
				Interface:   rule.Interface,
				ToSource:    rule.ToSource,
			})
		case database.NATTypeDNAT:
			config.DNAT = append(config.DNAT, nat.DNATRule{
				Interface:     rule.Interface,
				Protocol:      rule.Protocol,
				Port:          rule.Port,
				ToDestination: rule.ToDestination,
			})
		}
	}

	// Cleanup existing rules and apply new ones
	h.natManager.Cleanup()
	newManager := nat.NewManager(config)
	if err := newManager.Apply(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply NAT rules: " + err.Error()})
		return
	}

	// Update the manager reference
	*h.natManager = *newManager

	c.JSON(http.StatusOK, gin.H{
		"message":    "NAT rules applied successfully",
		"masquerade": len(config.Masquerade),
		"snat":       len(config.SNAT),
		"dnat":       len(config.DNAT),
	})
}
