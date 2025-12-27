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
	authHandler  *auth.Handler
	adminHandler *AdminHandler
	db           *database.DB
	configGen    *wireguard.ConfigGenerator
	tunnelURL    string
	subnet       string // VPN subnet (automatically included in routes)
}

// NewRouter creates a new API router
func NewRouter(authHandler *auth.Handler, adminHandler *AdminHandler, db *database.DB, configGen *wireguard.ConfigGenerator, tunnelURL string, subnet string) *Router {
	return &Router{
		authHandler:  authHandler,
		adminHandler: adminHandler,
		db:           db,
		configGen:    configGen,
		tunnelURL:    tunnelURL,
		subnet:       subnet,
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

		// Admin routes (requires authentication + admin privileges)
		admin := v1.Group("/admin")
		admin.Use(r.authHandler.AuthMiddleware())
		admin.Use(r.authHandler.AdminMiddleware())
		{
			// User management
			admin.GET("/users", r.adminHandler.ListUsers)
			admin.POST("/users", r.authHandler.CreateUserByAdmin)
			admin.GET("/users/:id", r.adminHandler.GetUser)
			admin.PUT("/users/:id", r.adminHandler.UpdateUser)
			admin.DELETE("/users/:id", r.adminHandler.DeleteUser)

			// Route management
			admin.GET("/routes", r.adminHandler.ListRoutes)
			admin.POST("/routes", r.adminHandler.CreateRoute)
			admin.PUT("/routes/:id", r.adminHandler.UpdateRoute)
			admin.DELETE("/routes/:id", r.adminHandler.DeleteRoute)

			// NAT rule management
			admin.GET("/nat", r.adminHandler.ListNATRules)
			admin.POST("/nat", r.adminHandler.CreateNATRule)
			admin.PUT("/nat/:id", r.adminHandler.UpdateNATRule)
			admin.DELETE("/nat/:id", r.adminHandler.DeleteNATRule)
			admin.POST("/nat/apply", r.adminHandler.ApplyNATRules)
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

	// Build routes: subnet + additional routes from database
	allRoutes := []string{r.subnet}
	dbRoutes, err := r.adminHandler.GetEnabledRoutes()
	if err == nil {
		allRoutes = append(allRoutes, dbRoutes...)
	}

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
