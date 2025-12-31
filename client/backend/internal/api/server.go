package api

import (
	"context"
	"fmt"
	"net/http"
	"wire-socket-client/internal/connection"

	"github.com/gin-gonic/gin"
)

// Server is the local HTTP API server
type Server struct {
	connMgr      *connection.Manager
	multiMgr     *connection.MultiManager
	httpServer   *http.Server
	engine       *gin.Engine
}

// NewServer creates a new API server
func NewServer(connMgr *connection.Manager, addr string) *Server {
	return NewServerWithMulti(connMgr, nil, addr)
}

// NewServerWithMulti creates a new API server with multi-tunnel support
func NewServerWithMulti(connMgr *connection.Manager, multiMgr *connection.MultiManager, addr string) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.Default()

	// Enable CORS for local Electron app
	engine.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	s := &Server{
		connMgr:  connMgr,
		multiMgr: multiMgr,
		engine:   engine,
		httpServer: &http.Server{
			Addr:    addr,
			Handler: engine,
		},
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures the API routes
func (s *Server) setupRoutes() {
	s.engine.GET("/health", s.healthCheck)

	api := s.engine.Group("/api")
	{
		api.POST("/connect", s.connect)
		api.POST("/disconnect", s.disconnect)
		api.GET("/status", s.getStatus)
		api.GET("/servers", s.getServers)
		api.POST("/servers", s.addServer)

		// Route management
		api.GET("/routes/settings", s.getRouteSettings)
		api.PUT("/routes/settings", s.updateRouteSettings)

		// Password management
		api.POST("/change-password", s.changePassword)

		// Auth (for multi-tunnel mode)
		api.POST("/auth/login", s.authLogin)
		api.POST("/auth/logout", s.authLogout)

		// Multi-tunnel management
		tunnels := api.Group("/tunnels")
		{
			tunnels.GET("", s.getTunnelsStatus)
			tunnels.POST("/connect", s.connectTunnel)
			tunnels.POST("/disconnect", s.disconnectTunnel)
			tunnels.POST("/disconnect-all", s.disconnectAllTunnels)
		}
	}
}

// Start starts the API server
func (s *Server) Start() error {
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("API server error: %v\n", err)
		}
	}()

	fmt.Printf("API server started on %s\n", s.httpServer.Addr)
	return nil
}

// Stop stops the API server
func (s *Server) Stop() error {
	return s.httpServer.Shutdown(context.Background())
}

// Handler functions

func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"version": "1.0.0",
	})
}

func (s *Server) connect(c *gin.Context) {
	var req connection.ConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := s.connMgr.Connect(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "connecting",
		"message": "VPN connection initiated",
	})
}

func (s *Server) disconnect(c *gin.Context) {
	if err := s.connMgr.Disconnect(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "disconnected",
		"message": "VPN disconnected successfully",
	})
}

func (s *Server) getStatus(c *gin.Context) {
	status := s.connMgr.GetStatus()
	c.JSON(http.StatusOK, status)
}

func (s *Server) getServers(c *gin.Context) {
	servers := s.connMgr.GetServers()
	c.JSON(http.StatusOK, gin.H{
		"servers": servers,
	})
}

func (s *Server) addServer(c *gin.Context) {
	var server connection.ServerConfig
	if err := c.ShouldBindJSON(&server); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// In a real implementation, you'd save this to a database or file
	c.JSON(http.StatusOK, gin.H{
		"message": "server added successfully",
		"server":  server,
	})
}

func (s *Server) getRouteSettings(c *gin.Context) {
	settings := s.connMgr.GetRouteSettings()
	availableRoutes := s.connMgr.GetAvailableRoutes()
	activeRoutes := s.connMgr.GetActiveRoutes()

	c.JSON(http.StatusOK, gin.H{
		"excluded_routes":  settings.ExcludedRoutes,
		"available_routes": availableRoutes,
		"active_routes":    activeRoutes,
	})
}

func (s *Server) updateRouteSettings(c *gin.Context) {
	var req struct {
		ExcludedRoutes []string `json:"excluded_routes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := s.connMgr.SetExcludedRoutes(req.ExcludedRoutes); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "route settings updated",
		"excluded_routes": req.ExcludedRoutes,
	})
}

func (s *Server) changePassword(c *gin.Context) {
	var req connection.ChangePasswordRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.ServerAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server_address is required"})
		return
	}

	if req.Token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current_password and new_password are required"})
		return
	}

	if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new password must be at least 8 characters"})
		return
	}

	if err := s.connMgr.ChangePassword(req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "password changed successfully",
	})
}

// Multi-tunnel handlers

func (s *Server) getTunnelsStatus(c *gin.Context) {
	if s.multiMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "multi-tunnel not enabled"})
		return
	}

	status := s.multiMgr.GetStatus()
	c.JSON(http.StatusOK, status)
}

func (s *Server) authLogin(c *gin.Context) {
	if s.multiMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "multi-tunnel not enabled"})
		return
	}

	var req connection.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.AuthURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth_url is required"})
		return
	}

	session, err := s.multiMgr.Login(req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"username":      session.Username,
		"tunnels":       session.Tunnels,
	})
}

func (s *Server) authLogout(c *gin.Context) {
	if s.multiMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "multi-tunnel not enabled"})
		return
	}

	s.multiMgr.Logout()

	c.JSON(http.StatusOK, gin.H{
		"status":  "logged_out",
		"message": "Logged out and disconnected all tunnels",
	})
}

func (s *Server) connectTunnel(c *gin.Context) {
	if s.multiMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "multi-tunnel not enabled"})
		return
	}

	var req connection.TunnelConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.TunnelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel_id is required"})
		return
	}

	if err := s.multiMgr.Connect(req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "connecting",
		"tunnel_id": req.TunnelID,
		"message":   "Tunnel connection initiated",
	})
}

func (s *Server) disconnectTunnel(c *gin.Context) {
	if s.multiMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "multi-tunnel not enabled"})
		return
	}

	var req struct {
		TunnelID string `json:"tunnel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.TunnelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tunnel_id is required"})
		return
	}

	if err := s.multiMgr.Disconnect(req.TunnelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "disconnected",
		"tunnel_id": req.TunnelID,
		"message":   "Tunnel disconnected successfully",
	})
}

func (s *Server) disconnectAllTunnels(c *gin.Context) {
	if s.multiMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "multi-tunnel not enabled"})
		return
	}

	s.multiMgr.DisconnectAll()

	c.JSON(http.StatusOK, gin.H{
		"status":  "disconnected",
		"message": "All tunnels disconnected",
	})
}
