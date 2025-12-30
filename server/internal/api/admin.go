package api

import (
	"net/http"
	"strconv"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/nat"
	"wire-socket-server/internal/route"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// AdminHandler handles admin API endpoints
type AdminHandler struct {
	db            *database.DB
	natManager    *nat.Manager
	routeManager  *route.Manager
	defaultDevice string
}

// NewAdminHandler creates a new AdminHandler
func NewAdminHandler(db *database.DB, natManager *nat.Manager, defaultDevice string) *AdminHandler {
	return &AdminHandler{
		db:            db,
		natManager:    natManager,
		defaultDevice: defaultDevice,
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
		CIDR          string `json:"cidr" binding:"required"`
		Gateway       string `json:"gateway"`
		Device        string `json:"device"`
		Metric        int    `json:"metric"`
		Comment       string `json:"comment"`
		Enabled       *bool  `json:"enabled"`
		PushToClient  *bool  `json:"push_to_client"`
		ApplyOnServer *bool  `json:"apply_on_server"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	pushToClient := true
	if req.PushToClient != nil {
		pushToClient = *req.PushToClient
	}

	applyOnServer := false
	if req.ApplyOnServer != nil {
		applyOnServer = *req.ApplyOnServer
	}

	dbRoute := database.Route{
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
		c.JSON(http.StatusConflict, gin.H{"error": "route already exists"})
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

	var dbRoute database.Route
	if err := h.db.First(&dbRoute, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	var req struct {
		CIDR          string `json:"cidr"`
		Gateway       string `json:"gateway"`
		Device        string `json:"device"`
		Metric        *int   `json:"metric"`
		Comment       string `json:"comment"`
		Enabled       *bool  `json:"enabled"`
		PushToClient  *bool  `json:"push_to_client"`
		ApplyOnServer *bool  `json:"apply_on_server"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.CIDR != "" {
		dbRoute.CIDR = req.CIDR
	}
	if req.Gateway != "" {
		dbRoute.Gateway = req.Gateway
	}
	if req.Device != "" {
		dbRoute.Device = req.Device
	}
	if req.Metric != nil {
		dbRoute.Metric = *req.Metric
	}
	if req.Comment != "" {
		dbRoute.Comment = req.Comment
	}
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

// GetEnabledRoutes returns all enabled routes that should be pushed to clients
func (h *AdminHandler) GetEnabledRoutes() ([]string, error) {
	var routes []database.Route
	if err := h.db.Where("enabled = ? AND push_to_client = ?", true, true).Find(&routes).Error; err != nil {
		return nil, err
	}

	cidrs := make([]string, len(routes))
	for i, r := range routes {
		cidrs[i] = r.CIDR
	}
	return cidrs, nil
}

// ApplyRoutes applies server-side routes
func (h *AdminHandler) ApplyRoutes(c *gin.Context) {
	var dbRoutes []database.Route
	if err := h.db.Where("enabled = ? AND apply_on_server = ?", true, true).Find(&dbRoutes).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch routes"})
		return
	}

	if len(dbRoutes) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message":      "No routes to apply",
			"routes_count": 0,
		})
		return
	}

	// Build route config
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

	// Cleanup existing routes and apply new ones
	if h.routeManager != nil {
		h.routeManager.Cleanup()
	}

	newManager := route.NewManager(routeConfig)
	if err := newManager.Apply(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to apply routes: " + err.Error()})
		return
	}

	// Update the manager reference
	h.routeManager = newManager

	c.JSON(http.StatusOK, gin.H{
		"message":      "Routes applied successfully",
		"routes_count": len(routes),
	})
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
		MSS           int                  `json:"mss"`
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
	case database.NATTypeTCPMSS:
		if req.Interface == "" || req.Source == "" || req.MSS == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "interface, source, and mss are required for TCPMSS rule"})
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
		MSS:           req.MSS,
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
		MSS           int    `json:"mss"`
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
	if req.MSS != 0 {
		rule.MSS = req.MSS
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
		case database.NATTypeTCPMSS:
			config.TCPMSS = append(config.TCPMSS, nat.TCPMSSRule{
				Interface: rule.Interface,
				Source:    rule.Source,
				MSS:       rule.MSS,
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
		"tcpmss":     len(config.TCPMSS),
	})
}

// ============ Group Management ============

// ListGroups returns all groups
func (h *AdminHandler) ListGroups(c *gin.Context) {
	var groups []database.Group
	if err := h.db.Find(&groups).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch groups"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"groups": groups})
}

// CreateGroup creates a new group
func (h *AdminHandler) CreateGroup(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	group := database.Group{
		Name:        req.Name,
		Description: req.Description,
	}

	if err := h.db.Create(&group).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "group already exists"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"group": group})
}

// GetGroup returns a specific group with its users and routes
func (h *AdminHandler) GetGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}

	var group database.Group
	if err := h.db.First(&group, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	// Get users in this group
	var userGroups []database.UserGroup
	h.db.Where("group_id = ?", id).Preload("User").Find(&userGroups)
	users := make([]database.User, len(userGroups))
	for i, ug := range userGroups {
		users[i] = ug.User
	}

	// Get routes in this group
	var routeGroups []database.RouteGroup
	h.db.Where("group_id = ?", id).Preload("Route").Find(&routeGroups)
	routes := make([]database.Route, len(routeGroups))
	for i, rg := range routeGroups {
		routes[i] = rg.Route
	}

	c.JSON(http.StatusOK, gin.H{
		"group":  group,
		"users":  users,
		"routes": routes,
	})
}

// UpdateGroup updates a group
func (h *AdminHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}

	var group database.Group
	if err := h.db.First(&group, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Name != "" {
		group.Name = req.Name
	}
	if req.Description != "" {
		group.Description = req.Description
	}

	if err := h.db.Save(&group).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"group": group})
}

// DeleteGroup deletes a group
func (h *AdminHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}

	var group database.Group
	if err := h.db.First(&group, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	// Delete all user-group associations
	h.db.Where("group_id = ?", id).Delete(&database.UserGroup{})

	// Delete all route-group associations
	h.db.Where("group_id = ?", id).Delete(&database.RouteGroup{})

	// Delete the group
	if err := h.db.Delete(&group).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "group deleted successfully"})
}

// AddUserToGroup adds a user to a group
func (h *AdminHandler) AddUserToGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}

	var req struct {
		UserID uint `json:"user_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Verify group exists
	var group database.Group
	if err := h.db.First(&group, groupID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	// Verify user exists
	var user database.User
	if err := h.db.First(&user, req.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	userGroup := database.UserGroup{
		UserID:  req.UserID,
		GroupID: uint(groupID),
	}

	if err := h.db.Create(&userGroup).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "user already in group"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "user added to group"})
}

// RemoveUserFromGroup removes a user from a group
func (h *AdminHandler) RemoveUserFromGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}

	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	result := h.db.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&database.UserGroup{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not in group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user removed from group"})
}

// AddRouteToGroup adds a route to a group
func (h *AdminHandler) AddRouteToGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}

	var req struct {
		RouteID uint `json:"route_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Verify group exists
	var group database.Group
	if err := h.db.First(&group, groupID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}

	// Verify route exists
	var route database.Route
	if err := h.db.First(&route, req.RouteID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}

	routeGroup := database.RouteGroup{
		RouteID: req.RouteID,
		GroupID: uint(groupID),
	}

	if err := h.db.Create(&routeGroup).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "route already in group"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "route added to group"})
}

// RemoveRouteFromGroup removes a route from a group
func (h *AdminHandler) RemoveRouteFromGroup(c *gin.Context) {
	groupID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group id"})
		return
	}

	routeID, err := strconv.ParseUint(c.Param("route_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid route id"})
		return
	}

	result := h.db.Where("group_id = ? AND route_id = ?", groupID, routeID).Delete(&database.RouteGroup{})
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not in group"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "route removed from group"})
}

// GetRoutesForUser returns routes that should be pushed to a specific user based on their groups
func (h *AdminHandler) GetRoutesForUser(userID uint) ([]string, error) {
	// Get user's groups
	var userGroups []database.UserGroup
	if err := h.db.Where("user_id = ?", userID).Find(&userGroups).Error; err != nil {
		return nil, err
	}

	// If user has no groups, return empty (or you could return global routes)
	if len(userGroups) == 0 {
		// Return routes that are not assigned to any group (global routes)
		return h.getGlobalRoutes()
	}

	// Get group IDs
	groupIDs := make([]uint, len(userGroups))
	for i, ug := range userGroups {
		groupIDs[i] = ug.GroupID
	}

	// Get route IDs for these groups
	var routeGroups []database.RouteGroup
	if err := h.db.Where("group_id IN ?", groupIDs).Find(&routeGroups).Error; err != nil {
		return nil, err
	}

	// Get unique route IDs
	routeIDSet := make(map[uint]bool)
	for _, rg := range routeGroups {
		routeIDSet[rg.RouteID] = true
	}

	if len(routeIDSet) == 0 {
		// User is in groups but no routes assigned, return global routes
		return h.getGlobalRoutes()
	}

	routeIDs := make([]uint, 0, len(routeIDSet))
	for id := range routeIDSet {
		routeIDs = append(routeIDs, id)
	}

	// Get routes
	var routes []database.Route
	if err := h.db.Where("id IN ? AND enabled = ? AND push_to_client = ?", routeIDs, true, true).Find(&routes).Error; err != nil {
		return nil, err
	}

	cidrs := make([]string, len(routes))
	for i, r := range routes {
		cidrs[i] = r.CIDR
	}

	return cidrs, nil
}

// getGlobalRoutes returns routes not assigned to any group
func (h *AdminHandler) getGlobalRoutes() ([]string, error) {
	// Get all route IDs that are assigned to groups
	var routeGroups []database.RouteGroup
	h.db.Find(&routeGroups)

	assignedRouteIDs := make([]uint, len(routeGroups))
	for i, rg := range routeGroups {
		assignedRouteIDs[i] = rg.RouteID
	}

	var routes []database.Route
	query := h.db.Where("enabled = ? AND push_to_client = ?", true, true)
	if len(assignedRouteIDs) > 0 {
		query = query.Where("id NOT IN ?", assignedRouteIDs)
	}
	if err := query.Find(&routes).Error; err != nil {
		return nil, err
	}

	cidrs := make([]string, len(routes))
	for i, r := range routes {
		cidrs[i] = r.CIDR
	}

	return cidrs, nil
}
