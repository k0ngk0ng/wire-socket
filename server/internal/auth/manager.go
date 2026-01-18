package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"time"
	"wire-socket-server/internal/database"

	"github.com/golang-jwt/jwt/v5"
)

// Manager manages multiple authentication providers
type Manager struct {
	db              *database.DB
	jwtSecret       []byte
	callbackBaseURL string

	providers map[string]Provider
	mu        sync.RWMutex

	// State management for OAuth flows
	states   map[string]*stateInfo
	statesMu sync.RWMutex
}

// stateInfo holds information about an OAuth state
type stateInfo struct {
	ProviderID  string
	RedirectURI string
	CreatedAt   time.Time
}

// NewManager creates a new authentication manager
func NewManager(db *database.DB, jwtSecret string, ssoConfig *SSOConfig) *Manager {
	m := &Manager{
		db:        db,
		jwtSecret: []byte(jwtSecret),
		providers: make(map[string]Provider),
		states:    make(map[string]*stateInfo),
	}

	// Always add local provider
	localProvider := NewLocalProvider(db)
	m.providers["local"] = localProvider

	// Add SSO providers if configured
	if ssoConfig != nil && ssoConfig.Enabled {
		m.callbackBaseURL = ssoConfig.CallbackBaseURL
		for _, cfg := range ssoConfig.Providers {
			if !cfg.Enabled {
				continue
			}
			provider, err := m.createProvider(cfg)
			if err != nil {
				// Log warning but continue
				fmt.Printf("Warning: failed to create provider %s: %v\n", cfg.ID, err)
				continue
			}
			m.providers[cfg.ID] = provider
		}
	}

	// Start state cleanup goroutine
	go m.cleanupStates()

	return m
}

// createProvider creates a provider based on configuration
func (m *Manager) createProvider(cfg ProviderConfig) (Provider, error) {
	switch cfg.Type {
	case ProviderTypeOIDC:
		return NewOIDCProvider(m.db, cfg, m.callbackBaseURL)
	case ProviderTypeOAuth2:
		return NewOAuth2Provider(m.db, cfg, m.callbackBaseURL)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", cfg.Type)
	}
}

// GetProviders returns information about all available providers
func (m *Manager) GetProviders() []ProviderInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	providers := make([]ProviderInfo, 0, len(m.providers))
	for _, p := range m.providers {
		providers = append(providers, p.GetInfo())
	}
	return providers
}

// GetProvider returns a specific provider by ID
func (m *Manager) GetProvider(id string) (Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, ok := m.providers[id]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", id)
	}
	return provider, nil
}

// GetSSOProvider returns a specific SSO provider by ID
func (m *Manager) GetSSOProvider(id string) (SSOProvider, error) {
	provider, err := m.GetProvider(id)
	if err != nil {
		return nil, err
	}

	ssoProvider, ok := provider.(SSOProvider)
	if !ok {
		return nil, fmt.Errorf("provider %s is not an SSO provider", id)
	}
	return ssoProvider, nil
}

// Authenticate authenticates a user with the specified provider
func (m *Manager) Authenticate(ctx context.Context, providerID string, credentials map[string]string) (*database.User, error) {
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	return provider.Authenticate(ctx, credentials)
}

// GenerateState generates a random state for OAuth flows
func (m *Manager) GenerateState(providerID, redirectURI string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}

	state := base64.URLEncoding.EncodeToString(b)

	m.statesMu.Lock()
	m.states[state] = &stateInfo{
		ProviderID:  providerID,
		RedirectURI: redirectURI,
		CreatedAt:   time.Now(),
	}
	m.statesMu.Unlock()

	return state, nil
}

// ValidateState validates and consumes a state
func (m *Manager) ValidateState(state string) (*stateInfo, error) {
	m.statesMu.Lock()
	defer m.statesMu.Unlock()

	info, ok := m.states[state]
	if !ok {
		return nil, fmt.Errorf("invalid state")
	}

	// Check if state is expired (10 minutes)
	if time.Since(info.CreatedAt) > 10*time.Minute {
		delete(m.states, state)
		return nil, fmt.Errorf("state expired")
	}

	// Consume state (one-time use)
	delete(m.states, state)
	return info, nil
}

// cleanupStates periodically removes expired states
func (m *Manager) cleanupStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		m.statesMu.Lock()
		now := time.Now()
		for state, info := range m.states {
			if now.Sub(info.CreatedAt) > 10*time.Minute {
				delete(m.states, state)
			}
		}
		m.statesMu.Unlock()
	}
}

// FindOrCreateUser finds an existing user or creates a new one for SSO login
func (m *Manager) FindOrCreateUser(providerID string, ssoUser *SSOUserInfo, adminValues []string) (*database.User, error) {
	// Try to find existing user by provider and external ID
	var user database.User
	err := m.db.Where("auth_provider = ? AND external_id = ?", providerID, ssoUser.ExternalID).First(&user).Error
	if err == nil {
		// Update last login
		now := time.Now()
		m.db.Model(&user).Updates(map[string]interface{}{
			"last_login": now,
			"email":      ssoUser.Email,    // Update email in case it changed
			"username":   ssoUser.Username, // Update username in case it changed
		})
		user.LastLogin = &now
		return &user, nil
	}

	// Check if a user with this email already exists (from different provider)
	err = m.db.Where("email = ?", ssoUser.Email).First(&user).Error
	if err == nil {
		// User exists with different provider - don't allow
		if user.AuthProvider != providerID {
			return nil, fmt.Errorf("email already associated with a %s account", user.AuthProvider)
		}
		// Update external ID if missing (migration case)
		now := time.Now()
		m.db.Model(&user).Updates(map[string]interface{}{
			"external_id": ssoUser.ExternalID,
			"last_login":  now,
		})
		user.LastLogin = &now
		return &user, nil
	}

	// Create new user (JIT Provisioning)
	isAdmin := m.checkAdminStatus(ssoUser.Groups, adminValues)
	now := time.Now()
	user = database.User{
		Username:     ssoUser.Username,
		Email:        ssoUser.Email,
		AuthProvider: providerID,
		ExternalID:   ssoUser.ExternalID,
		IsActive:     true,
		IsAdmin:      isAdmin,
		LastLogin:    &now,
	}

	if err := m.db.Create(&user).Error; err != nil {
		// Handle username conflict - append random suffix
		if strings.Contains(err.Error(), "username") {
			user.Username = fmt.Sprintf("%s_%d", ssoUser.Username, time.Now().UnixNano()%10000)
			if err := m.db.Create(&user).Error; err != nil {
				return nil, fmt.Errorf("failed to create user: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
	}

	return &user, nil
}

// checkAdminStatus checks if user is admin based on group membership
func (m *Manager) checkAdminStatus(groups []string, adminValues []string) bool {
	if len(adminValues) == 0 {
		return false
	}

	for _, group := range groups {
		for _, admin := range adminValues {
			if strings.EqualFold(group, admin) {
				return true
			}
		}
	}
	return false
}

// GenerateToken generates a JWT token for a user
func (m *Manager) GenerateToken(userID uint) (string, time.Time, error) {
	expiresAt := time.Now().Add(24 * time.Hour)

	claims := jwt.MapClaims{
		"user_id": userID,
		"exp":     expiresAt.Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(m.jwtSecret)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// ValidateToken validates a JWT token and returns the user ID
func (m *Manager) ValidateToken(tokenString string) (uint, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return m.jwtSecret, nil
	})

	if err != nil {
		return 0, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID := uint(claims["user_id"].(float64))
		return userID, nil
	}

	return 0, jwt.ErrTokenInvalidClaims
}

// GetCallbackURL returns the callback URL for a provider
func (m *Manager) GetCallbackURL(providerID string) string {
	return fmt.Sprintf("%s/api/auth/callback/%s", m.callbackBaseURL, providerID)
}
