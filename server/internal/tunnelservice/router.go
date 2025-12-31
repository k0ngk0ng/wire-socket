package tunnelservice

import (
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/nat"
	"wire-socket-server/internal/wireguard"

	"github.com/gin-gonic/gin"
)

// Router sets up all API routes for the tunnel service
type Router struct {
	db           *database.TunnelDB
	authHandler  *AuthHandler
	adminHandler *AdminHandler
}

// NewRouter creates a new Router
func NewRouter(db *database.TunnelDB, wgManager *wireguard.Manager, natManager *nat.Manager, authConfig AuthConfig, defaultDevice string) *Router {
	return &Router{
		db:           db,
		authHandler:  NewAuthHandler(db, wgManager, authConfig),
		adminHandler: NewAdminHandler(db, natManager, defaultDevice),
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
		// Auth endpoints (for clients)
		auth := api.Group("/auth")
		{
			auth.POST("/login", r.authHandler.Login)
			auth.POST("/change-password", r.authHandler.ChangePassword)
		}

		// Config endpoint
		api.GET("/config", r.authHandler.GetConfig)

		// Admin endpoints (require JWT auth if configured)
		admin := api.Group("/admin")
		admin.Use(r.authHandler.AdminAuthMiddleware())
		{
			// Route management
			admin.GET("/routes", r.adminHandler.ListRoutes)
			admin.POST("/routes", r.adminHandler.CreateRoute)
			admin.PUT("/routes/:id", r.adminHandler.UpdateRoute)
			admin.DELETE("/routes/:id", r.adminHandler.DeleteRoute)
			admin.POST("/routes/apply", r.adminHandler.ApplyRoutes)

			// NAT management
			admin.GET("/nat", r.adminHandler.ListNATRules)
			admin.POST("/nat", r.adminHandler.CreateNATRule)
			admin.PUT("/nat/:id", r.adminHandler.UpdateNATRule)
			admin.DELETE("/nat/:id", r.adminHandler.DeleteNATRule)
			admin.POST("/nat/apply", r.adminHandler.ApplyNATRules)
		}
	}
}
