package api

import (
	"fmt"
	"net/http"
	"wire-socket-server/internal/auth"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/wireguard"

	"github.com/gin-gonic/gin"
)

// Router sets up the API routes
type Router struct {
	authHandler *auth.Handler
	db          *database.DB
	configGen   *wireguard.ConfigGenerator
	tunnelURL   string
	routes      []string // Additional routes to push to clients
	subnet      string   // VPN subnet (automatically included in routes)
}

// NewRouter creates a new API router
func NewRouter(authHandler *auth.Handler, db *database.DB, configGen *wireguard.ConfigGenerator, tunnelURL string, routes []string, subnet string) *Router {
	return &Router{
		authHandler: authHandler,
		db:          db,
		configGen:   configGen,
		tunnelURL:   tunnelURL,
		routes:      routes,
		subnet:      subnet,
	}
}

// SetupRoutes configures all API routes
func (r *Router) SetupRoutes(engine *gin.Engine) {
	// Health check
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// API v1 routes
	v1 := engine.Group("/api")
	{
		// Public routes (no authentication required)
		auth := v1.Group("/auth")
		{
			auth.POST("/login", r.authHandler.Login)
			auth.POST("/register", r.authHandler.Register)
		}

		// Protected routes (authentication required)
		protected := v1.Group("")
		protected.Use(r.authHandler.AuthMiddleware())
		{
			protected.POST("/auth/refresh", r.authHandler.RefreshToken)
			protected.GET("/config", r.GetConfig)
			protected.GET("/servers", r.ListServers)
			protected.GET("/status", r.GetStatus)
		}

		// Admin routes (would need admin middleware in production)
		admin := v1.Group("/admin")
		admin.Use(r.authHandler.AuthMiddleware())
		{
			admin.POST("/users", r.CreateUser)
			admin.GET("/users", r.ListUsers)
			admin.DELETE("/users/:id", r.DeleteUser)
		}
	}
}

// GetConfig returns WireGuard configuration for the authenticated user
func (r *Router) GetConfig(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get server ID from query params (default to first server)
	serverID := uint(1)
	if sid, ok := c.GetQuery("server_id"); ok {
		var id uint
		if _, err := fmt.Sscanf(sid, "%d", &id); err == nil {
			serverID = id
		}
	}

	// Generate WireGuard config
	config, err := r.configGen.GenerateForUser(userID.(uint), serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build routes: subnet + additional routes
	allRoutes := []string{r.subnet}
	allRoutes = append(allRoutes, r.routes...)

	c.JSON(http.StatusOK, gin.H{
		"config":     config,
		"ini_format": config.ToINIFormat(),
		"tunnel_url": r.tunnelURL,
		"routes":     allRoutes,
	})
}

// ListServers returns available VPN servers
func (r *Router) ListServers(c *gin.Context) {
	var servers []database.Server
	if err := r.db.Find(&servers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch servers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"servers": servers})
}

// GetStatus returns connection status and statistics
func (r *Router) GetStatus(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Get user's allocated IPs
	var allocations []database.AllocatedIP
	if err := r.db.Where("user_id = ?", userID).Preload("Server").Find(&allocations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch allocations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"allocations": allocations,
	})
}

// CreateUser creates a new user (admin only)
func (r *Router) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required,min=8"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Use auth handler to register user
	r.authHandler.Register(c)
}

// ListUsers returns all users (admin only)
func (r *Router) ListUsers(c *gin.Context) {
	var users []database.User
	if err := r.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch users"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"users": users})
}

// DeleteUser deletes a user (admin only)
func (r *Router) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	var user database.User
	if err := r.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Delete user's IP allocations
	r.db.Where("user_id = ?", user.ID).Delete(&database.AllocatedIP{})

	// Delete user
	if err := r.db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted successfully"})
}
