package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"wire-socket-tunnel/internal/database"
	"wire-socket-tunnel/internal/wireguard"

	"github.com/gin-gonic/gin"
)

// AuthHandler handles authentication by proxying to auth service
type AuthHandler struct {
	db           *database.DB
	wgManager    *wireguard.Manager
	authURL      string // URL of auth service
	tunnelID     string
	tunnelToken  string
	tunnelURL    string // Public tunnel URL for clients
	subnet       string
	serverPubKey string
	endpoint     string
	dns          []string
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(db *database.DB, wgManager *wireguard.Manager, config AuthConfig) *AuthHandler {
	return &AuthHandler{
		db:           db,
		wgManager:    wgManager,
		authURL:      config.AuthURL,
		tunnelID:     config.TunnelID,
		tunnelToken:  config.TunnelToken,
		tunnelURL:    config.TunnelURL,
		subnet:       config.Subnet,
		serverPubKey: config.ServerPublicKey,
		endpoint:     config.Endpoint,
		dns:          config.DNS,
	}
}

// AuthConfig holds auth-related configuration
type AuthConfig struct {
	AuthURL         string
	TunnelID        string
	TunnelToken     string
	TunnelURL       string
	Subnet          string
	ServerPublicKey string
	Endpoint        string
	DNS             []string
}

// LoginRequest from client
type LoginRequest struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
}

// VerifyRequest to auth service
type VerifyRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TunnelID string `json:"tunnel_id"`
}

// VerifyResponse from auth service
type VerifyResponse struct {
	Valid          bool     `json:"valid"`
	UserID         uint     `json:"user_id"`
	Username       string   `json:"username"`
	AllowedTunnels []string `json:"allowed_tunnels"`
	Error          string   `json:"error"`
}

// Login handles POST /api/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Verify with auth service
	verifyResp, err := h.verifyWithAuth(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "auth service unavailable"})
		return
	}

	if !verifyResp.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": verifyResp.Error})
		return
	}

	// Allocate IP for user
	allocated, err := h.db.GetOrCreateIP(verifyResp.UserID, verifyResp.Username, h.subnet)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to allocate IP"})
		return
	}

	// Update public key
	if err := h.db.UpdatePublicKey(verifyResp.UserID, req.PublicKey); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update public key"})
		return
	}

	// Add WireGuard peer
	if err := h.wgManager.AddPeer(req.PublicKey, allocated.IP+"/32"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add peer"})
		return
	}

	// Get routes
	routes, _ := h.db.GetEnabledRoutes()
	if routes == nil {
		routes = []string{"0.0.0.0/0"}
	}

	// Return config
	c.JSON(http.StatusOK, gin.H{
		"interface": gin.H{
			"address": allocated.IP + "/32",
			"dns":     h.dns,
		},
		"peer": gin.H{
			"public_key":  h.serverPubKey,
			"endpoint":    h.endpoint,
			"allowed_ips": routes,
		},
		"tunnel_url": h.tunnelURL,
	})
}

// verifyWithAuth calls auth service to verify user
func (h *AuthHandler) verifyWithAuth(username, password string) (*VerifyResponse, error) {
	reqBody := VerifyRequest{
		Username: username,
		Password: password,
		TunnelID: h.tunnelID,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", h.authURL+"/api/tunnel/verify", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Tunnel-Token", h.tunnelToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var verifyResp VerifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&verifyResp); err != nil {
		return nil, err
	}

	return &verifyResp, nil
}

// RegisterWithAuth registers this tunnel with auth service
func (h *AuthHandler) RegisterWithAuth(name, url, internalURL, region, masterToken string) error {
	reqBody := map[string]string{
		"id":           h.tunnelID,
		"name":         name,
		"url":          url,
		"internal_url": internalURL,
		"region":       region,
		"token":        h.tunnelToken,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", h.authURL+"/api/tunnel/register", bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Master-Token", masterToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("registration failed: %d", resp.StatusCode)
	}

	return nil
}

// GetConfig handles GET /api/config - returns WireGuard config for authenticated user
func (h *AuthHandler) GetConfig(c *gin.Context) {
	// This endpoint could be used for config refresh
	c.JSON(http.StatusOK, gin.H{
		"peer": gin.H{
			"public_key": h.serverPubKey,
			"endpoint":   h.endpoint,
		},
		"tunnel_url": h.tunnelURL,
	})
}

// ChangePassword handles POST /api/auth/change-password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	// Proxy to auth service - not implemented yet
	c.JSON(http.StatusNotImplemented, gin.H{"error": "password change should be done via auth service"})
}
