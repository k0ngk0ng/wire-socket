package authservice

import (
	"wire-socket-server/internal/database"

	"github.com/gin-gonic/gin"
)

// Router sets up all API routes for the auth service
type Router struct {
	db            *database.AuthDB
	authHandler   *AuthHandler
	adminHandler  *AdminHandler
	tunnelHandler *TunnelHandler
}

// NewRouter creates a new Router
func NewRouter(db *database.AuthDB, jwtSecret string) *Router {
	return &Router{
		db:            db,
		authHandler:   NewAuthHandler(db, jwtSecret),
		adminHandler:  NewAdminHandler(db),
		tunnelHandler: NewTunnelHandler(db),
	}
}

// SetupRoutes configures all routes
func (r *Router) SetupRoutes(engine *gin.Engine) {
	// Health check
	engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	api := engine.Group("/api")
	{
		// Auth endpoints (for admin login)
		auth := api.Group("/auth")
		{
			auth.POST("/login", r.authHandler.Login)
		}

		// Tunnel endpoints (for tunnel nodes)
		tunnel := api.Group("/tunnel")
		{
			tunnel.POST("/verify", r.tunnelHandler.Verify)
			tunnel.POST("/register", r.tunnelHandler.Register)
			tunnel.POST("/heartbeat", r.tunnelHandler.Heartbeat)
		}

		// Public endpoints
		api.GET("/tunnels", r.tunnelHandler.ListTunnels)

		// Admin endpoints (require auth)
		admin := api.Group("/admin")
		admin.Use(r.authHandler.AuthMiddleware(), r.authHandler.AdminMiddleware())
		{
			// User management
			admin.GET("/users", r.adminHandler.ListUsers)
			admin.POST("/users", r.adminHandler.CreateUser)
			admin.GET("/users/:id", r.adminHandler.GetUser)
			admin.PUT("/users/:id", r.adminHandler.UpdateUser)
			admin.DELETE("/users/:id", r.adminHandler.DeleteUser)
			admin.GET("/users/:id/tunnels", r.adminHandler.GetUserTunnelAccess)
			admin.PUT("/users/:id/tunnels", r.adminHandler.SetUserTunnelAccess)

			// Tunnel management
			admin.GET("/tunnels", r.adminHandler.ListTunnels)
			admin.GET("/tunnels/:id", r.adminHandler.GetTunnel)
			admin.PUT("/tunnels/:id", r.adminHandler.UpdateTunnel)
			admin.DELETE("/tunnels/:id", r.adminHandler.DeleteTunnel)
		}
	}
}
