package authservice

import (
	"net/http"
	"time"
	"wire-socket-server/internal/database"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// TunnelHandler handles tunnel-related API endpoints
type TunnelHandler struct {
	db *database.AuthDB
}

// NewTunnelHandler creates a new TunnelHandler
func NewTunnelHandler(db *database.AuthDB) *TunnelHandler {
	return &TunnelHandler{db: db}
}

// VerifyRequest from tunnel node
type VerifyRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	TunnelID string `json:"tunnel_id" binding:"required"`
}

// VerifyResponse to tunnel node
type VerifyResponse struct {
	Valid          bool     `json:"valid"`
	UserID         uint     `json:"user_id,omitempty"`
	Username       string   `json:"username,omitempty"`
	AllowedTunnels []string `json:"allowed_tunnels,omitempty"`
	Error          string   `json:"error,omitempty"`
}

// Verify handles POST /api/tunnel/verify - called by tunnel nodes
func (h *TunnelHandler) Verify(c *gin.Context) {
	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, VerifyResponse{
			Valid: false,
			Error: "invalid request",
		})
		return
	}

	// Verify tunnel node identity from header
	tunnelToken := c.GetHeader("X-Tunnel-Token")
	if !h.verifyTunnelToken(req.TunnelID, tunnelToken) {
		c.JSON(http.StatusForbidden, VerifyResponse{
			Valid: false,
			Error: "invalid tunnel credentials",
		})
		return
	}

	// Find user
	var user database.AuthUser
	if err := h.db.Where("username = ? AND is_active = ?", req.Username, true).First(&user).Error; err != nil {
		c.JSON(http.StatusOK, VerifyResponse{
			Valid: false,
			Error: "user not found or inactive",
		})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusOK, VerifyResponse{
			Valid: false,
			Error: "invalid password",
		})
		return
	}

	// Get allowed tunnels
	allowedTunnels, err := h.db.GetUserAllowedTunnels(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, VerifyResponse{
			Valid: false,
			Error: "failed to get tunnel access",
		})
		return
	}

	// Check if user can access this tunnel
	canAccess := false
	for _, t := range allowedTunnels {
		if t == req.TunnelID {
			canAccess = true
			break
		}
	}

	if !canAccess {
		c.JSON(http.StatusOK, VerifyResponse{
			Valid: false,
			Error: "no access to this tunnel",
		})
		return
	}

	c.JSON(http.StatusOK, VerifyResponse{
		Valid:          true,
		UserID:         user.ID,
		Username:       user.Username,
		AllowedTunnels: allowedTunnels,
	})
}

// verifyTunnelToken checks if the tunnel token is valid
func (h *TunnelHandler) verifyTunnelToken(tunnelID, token string) bool {
	var tunnel database.Tunnel
	if err := h.db.Where("id = ? AND is_active = ?", tunnelID, true).First(&tunnel).Error; err != nil {
		return false
	}

	// Compare token hash
	if err := bcrypt.CompareHashAndPassword([]byte(tunnel.TokenHash), []byte(token)); err != nil {
		return false
	}

	// Update last seen
	h.db.Model(&tunnel).Update("last_seen", time.Now())
	return true
}

// RegisterRequest from tunnel node
type RegisterRequest struct {
	ID          string `json:"id" binding:"required"`
	Name        string `json:"name" binding:"required"`
	URL         string `json:"url" binding:"required"`
	InternalURL string `json:"internal_url"`
	Region      string `json:"region"`
	Token       string `json:"token" binding:"required"` // Pre-shared secret
}

// Register handles POST /api/tunnel/register - called by tunnel nodes on startup
func (h *TunnelHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Check if tunnel exists
	var tunnel database.Tunnel
	err := h.db.Where("id = ?", req.ID).First(&tunnel).Error

	if err != nil {
		// New tunnel - verify master token from header
		masterToken := c.GetHeader("X-Master-Token")
		if !h.verifyMasterToken(masterToken) {
			c.JSON(http.StatusForbidden, gin.H{"error": "invalid master token"})
			return
		}

		// Hash the token
		tokenHash, err := bcrypt.GenerateFromPassword([]byte(req.Token), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash token"})
			return
		}

		tunnel = database.Tunnel{
			ID:          req.ID,
			Name:        req.Name,
			URL:         req.URL,
			InternalURL: req.InternalURL,
			Region:      req.Region,
			TokenHash:   string(tokenHash),
			IsActive:    true,
			LastSeen:    time.Now(),
		}

		if err := h.db.Create(&tunnel).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register tunnel"})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "tunnel registered",
			"id":      tunnel.ID,
		})
		return
	}

	// Existing tunnel - verify its token
	if err := bcrypt.CompareHashAndPassword([]byte(tunnel.TokenHash), []byte(req.Token)); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid tunnel token"})
		return
	}

	// Update tunnel info
	tunnel.Name = req.Name
	tunnel.URL = req.URL
	tunnel.InternalURL = req.InternalURL
	tunnel.Region = req.Region
	tunnel.LastSeen = time.Now()

	if err := h.db.Save(&tunnel).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update tunnel"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "tunnel updated",
		"id":      tunnel.ID,
	})
}

// Heartbeat handles POST /api/tunnel/heartbeat
func (h *TunnelHandler) Heartbeat(c *gin.Context) {
	tunnelID := c.GetHeader("X-Tunnel-ID")
	tunnelToken := c.GetHeader("X-Tunnel-Token")

	if !h.verifyTunnelToken(tunnelID, tunnelToken) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid tunnel credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ListTunnels handles GET /api/tunnels - returns available tunnels for clients
func (h *TunnelHandler) ListTunnels(c *gin.Context) {
	var tunnels []database.Tunnel
	if err := h.db.Where("is_active = ?", true).Find(&tunnels).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tunnels"})
		return
	}

	// Return public info only
	type PublicTunnel struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		URL    string `json:"url"`
		Region string `json:"region"`
	}

	result := make([]PublicTunnel, len(tunnels))
	for i, t := range tunnels {
		result[i] = PublicTunnel{
			ID:     t.ID,
			Name:   t.Name,
			URL:    t.URL,
			Region: t.Region,
		}
	}

	c.JSON(http.StatusOK, gin.H{"tunnels": result})
}

// verifyMasterToken checks the master token for new tunnel registration
func (h *TunnelHandler) verifyMasterToken(token string) bool {
	// Use the configured master token
	return token != "" && token == masterToken
}

// masterToken holds the master token for tunnel registration
var masterToken string

// SetMasterToken sets the master token (called from main)
func SetMasterToken(token string) {
	masterToken = token
}
