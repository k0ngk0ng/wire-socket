package authservice

import (
	"net/http"
	"time"
	"wire-socket-server/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	db        *database.AuthDB
	jwtSecret string
}

// NewAuthHandler creates a new AuthHandler
func NewAuthHandler(db *database.AuthDB, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		db:        db,
		jwtSecret: jwtSecret,
	}
}

// LoginRequest for user login
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// TunnelInfo contains tunnel connection info for clients
type TunnelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Region      string `json:"region"`
	InternalURL string `json:"internal_url"` // API endpoint for login
}

// LoginResponse for user login
type LoginResponse struct {
	Token    string       `json:"token"`
	Expires  int64        `json:"expires"`
	UserID   uint         `json:"user_id"`
	Username string       `json:"username"`
	IsAdmin  bool         `json:"is_admin"`
	Tunnels  []TunnelInfo `json:"tunnels"` // Accessible tunnels with connection info
}

// Login handles POST /api/auth/login - for both admin and regular users
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Find user (allow both admin and regular users)
	var user database.AuthUser
	if err := h.db.Where("username = ? AND is_active = ?", req.Username, true).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Generate JWT token
	expires := time.Now().Add(24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"is_admin": user.IsAdmin,
		"exp":      expires.Unix(),
	})

	tokenString, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Get user's accessible tunnels
	tunnels, err := h.getUserTunnels(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get tunnel access"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:    tokenString,
		Expires:  expires.Unix(),
		UserID:   user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		Tunnels:  tunnels,
	})
}

// getUserTunnels returns tunnels accessible by the user with connection info
func (h *AuthHandler) getUserTunnels(userID uint) ([]TunnelInfo, error) {
	// Get allowed tunnel IDs
	tunnelIDs, err := h.db.GetUserAllowedTunnels(userID)
	if err != nil {
		return nil, err
	}

	if len(tunnelIDs) == 0 {
		return []TunnelInfo{}, nil
	}

	// Get tunnel details
	var tunnels []database.Tunnel
	if err := h.db.Where("id IN ? AND is_active = ?", tunnelIDs, true).Find(&tunnels).Error; err != nil {
		return nil, err
	}

	result := make([]TunnelInfo, len(tunnels))
	for i, t := range tunnels {
		result[i] = TunnelInfo{
			ID:          t.ID,
			Name:        t.Name,
			URL:         t.URL,
			Region:      t.Region,
			InternalURL: t.InternalURL,
		}
	}

	return result, nil
}

// AuthMiddleware validates JWT tokens for admin API
func (h *AuthHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := c.GetHeader("Authorization")
		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// Remove "Bearer " prefix if present
		if len(tokenString) > 7 && tokenString[:7] == "Bearer " {
			tokenString = tokenString[7:]
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte(h.jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		c.Set("user_id", uint(claims["user_id"].(float64)))
		c.Set("username", claims["username"].(string))
		c.Set("is_admin", claims["is_admin"].(bool))
		c.Next()
	}
}

// AdminMiddleware ensures the user is an admin
func (h *AuthHandler) AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		isAdmin, exists := c.Get("is_admin")
		if !exists || !isAdmin.(bool) {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}
		c.Next()
	}
}
