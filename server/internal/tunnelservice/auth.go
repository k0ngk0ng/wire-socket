package tunnelservice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/wireguard"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthHandler handles authentication by proxying to auth service
type AuthHandler struct {
	db           *database.TunnelDB
	wgManager    *wireguard.Manager
	authURL      string // URL of auth service
	tunnelID     string
	tunnelToken  string
	tunnelURL    string // Public tunnel URL for clients
	subnet       string
	serverPubKey string
	endpoint     string
	dns          []string
	jwtSecret    string
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(db *database.TunnelDB, wgManager *wireguard.Manager, config AuthConfig) *AuthHandler {
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
		jwtSecret:    config.JWTSecret,
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
	JWTSecret       string // For admin authentication
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
		routes = []string{}
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

// AdminAuthMiddleware validates JWT token for admin access
func (h *AuthHandler) AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip if no JWT secret configured (backwards compatibility)
		if h.jwtSecret == "" {
			c.Next()
			return
		}

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			c.Abort()
			return
		}

		// Parse and validate token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(h.jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		// Check if user is admin
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		isAdmin, _ := claims["is_admin"].(bool)
		if !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}
