package api

import (
	"net/http"
	"wire-socket-server/internal/auth"
	"wire-socket-server/internal/database"
	"wire-socket-server/internal/wireguard"

	"github.com/gin-gonic/gin"
)

// SSOHandler handles SSO-related API requests
type SSOHandler struct {
	authManager *auth.Manager
	db          *database.DB
	configGen   *wireguard.ConfigGenerator
	tunnelURL   string
	subnet      string
}

// NewSSOHandler creates a new SSO handler
func NewSSOHandler(authManager *auth.Manager, db *database.DB, configGen *wireguard.ConfigGenerator, tunnelURL, subnet string) *SSOHandler {
	return &SSOHandler{
		authManager: authManager,
		db:          db,
		configGen:   configGen,
		tunnelURL:   tunnelURL,
		subnet:      subnet,
	}
}

// GetProviders returns available authentication providers
func (h *SSOHandler) GetProviders(c *gin.Context) {
	providers := h.authManager.GetProviders()
	c.JSON(http.StatusOK, gin.H{
		"providers": providers,
	})
}

// Login handles local username/password login
func (h *SSOHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Authenticate with local provider
	user, err := h.authManager.Authenticate(c.Request.Context(), "local", map[string]string{
		"username": req.Username,
		"password": req.Password,
	})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Generate JWT token
	token, expiresAt, err := h.authManager.GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      token,
		"expires_at": expiresAt,
		"user": gin.H{
			"id":            user.ID,
			"username":      user.Username,
			"email":         user.Email,
			"is_admin":      user.IsAdmin,
			"auth_provider": user.AuthProvider,
		},
	})
}

// InitiateSSO redirects to the SSO provider for authentication
func (h *SSOHandler) InitiateSSO(c *gin.Context) {
	providerID := c.Param("provider")

	// Get the SSO provider
	provider, err := h.authManager.GetSSOProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	// Get optional redirect URI from query
	redirectURI := c.Query("redirect_uri")

	// Generate state
	state, err := h.authManager.GenerateState(providerID, redirectURI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
		return
	}

	// Get callback URL
	callbackURL := h.authManager.GetCallbackURL(providerID)

	// Get authorization URL
	authURL := provider.GetAuthURL(state, callbackURL)
	if authURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate authorization URL"})
		return
	}

	// Redirect to IdP
	c.Redirect(http.StatusFound, authURL)
}

// HandleCallback handles the SSO callback from the IdP
func (h *SSOHandler) HandleCallback(c *gin.Context) {
	providerID := c.Param("provider")

	// Get code and state from query
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		// Check for error in query
		if errMsg := c.Query("error"); errMsg != "" {
			errDesc := c.Query("error_description")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":       errMsg,
				"description": errDesc,
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
		return
	}

	// Validate state
	stateInfo, err := h.authManager.ValidateState(state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify provider matches
	if stateInfo.ProviderID != providerID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider mismatch"})
		return
	}

	// Get the SSO provider
	provider, err := h.authManager.GetSSOProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "provider not found"})
		return
	}

	// Get callback URL
	callbackURL := h.authManager.GetCallbackURL(providerID)

	// Exchange code for user info (use the extended interface if available)
	var ssoUser *auth.SSOUserInfo
	type stateExchanger interface {
		ExchangeCodeWithState(ctx interface{}, code, redirectURI, state string) (*auth.SSOUserInfo, error)
	}
	if se, ok := provider.(stateExchanger); ok {
		ssoUser, err = se.ExchangeCodeWithState(c.Request.Context(), code, callbackURL, state)
	} else {
		ssoUser, err = provider.ExchangeCode(c.Request.Context(), code, callbackURL)
	}
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Get admin values for this provider
	var adminValues []string
	type adminProvider interface {
		GetAdminValues() []string
	}
	if ap, ok := provider.(adminProvider); ok {
		adminValues = ap.GetAdminValues()
	}

	// Find or create user
	user, err := h.authManager.FindOrCreateUser(providerID, ssoUser, adminValues)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check if user is active
	if !user.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is inactive"})
		return
	}

	// Generate JWT token
	token, expiresAt, err := h.authManager.GenerateToken(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Check Accept header for response format
	if c.GetHeader("Accept") == "application/json" || c.Query("format") == "json" {
		// Return JSON response
		c.JSON(http.StatusOK, gin.H{
			"token":      token,
			"expires_at": expiresAt,
			"user": gin.H{
				"id":            user.ID,
				"username":      user.Username,
				"email":         user.Email,
				"is_admin":      user.IsAdmin,
				"auth_provider": user.AuthProvider,
			},
		})
		return
	}

	// Check if there's a custom redirect URI
	if stateInfo.RedirectURI != "" {
		// Redirect to custom URI with token
		redirectURL := stateInfo.RedirectURI
		if redirectURL[len(redirectURL)-1] != '?' && redirectURL[len(redirectURL)-1] != '&' {
			if len(redirectURL) > 0 && redirectURL[len(redirectURL)-1] != '/' {
				redirectURL += "?"
			}
		}
		redirectURL += "token=" + token
		c.Redirect(http.StatusFound, redirectURL)
		return
	}

	// Default: redirect to success page
	c.Redirect(http.StatusFound, "/login-success?token="+token)
}

// GetCurrentUser returns information about the currently authenticated user
func (h *SSOHandler) GetCurrentUser(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var user database.User
	if err := h.db.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            user.ID,
		"username":      user.Username,
		"email":         user.Email,
		"is_admin":      user.IsAdmin,
		"auth_provider": user.AuthProvider,
		"last_login":    user.LastLogin,
		"created_at":    user.CreatedAt,
	})
}

// AuthMiddleware returns the auth middleware using the manager
func (h *SSOHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>" format
		tokenString := authHeader
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		}

		userID, err := h.authManager.ValidateToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		// Set user ID in context
		c.Set("user_id", userID)
		c.Next()
	}
}

// AdminMiddleware checks if the user is an admin
func (h *SSOHandler) AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		var user database.User
		if err := h.db.First(&user, userID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			c.Abort()
			return
		}

		if !user.IsAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		c.Set("is_admin", true)
		c.Next()
	}
}
