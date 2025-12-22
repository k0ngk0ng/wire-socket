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
	connMgr    *connection.Manager
	httpServer *http.Server
	engine     *gin.Engine
}

// NewServer creates a new API server
func NewServer(connMgr *connection.Manager, addr string) *Server {
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
		connMgr: connMgr,
		engine:  engine,
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
