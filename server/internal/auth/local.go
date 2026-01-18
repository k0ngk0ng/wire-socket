package auth

import (
	"context"
	"fmt"
	"time"
	"wire-socket-server/internal/database"

	"golang.org/x/crypto/bcrypt"
)

// LocalProvider implements the Provider interface for local username/password authentication
type LocalProvider struct {
	db *database.DB
}

// NewLocalProvider creates a new local authentication provider
func NewLocalProvider(db *database.DB) *LocalProvider {
	return &LocalProvider{db: db}
}

// GetInfo returns information about the local provider
func (p *LocalProvider) GetInfo() ProviderInfo {
	return ProviderInfo{
		ID:   "local",
		Type: ProviderTypeLocal,
		Name: "Local Account",
	}
}

// Authenticate validates username and password
func (p *LocalProvider) Authenticate(ctx context.Context, credentials map[string]string) (*database.User, error) {
	username := credentials["username"]
	password := credentials["password"]

	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	// Find user by username
	var user database.User
	if err := p.db.Where("username = ?", username).First(&user).Error; err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user is a local user (has password)
	if user.AuthProvider != "local" && user.AuthProvider != "" {
		return nil, fmt.Errorf("user must login with %s", user.AuthProvider)
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	// Check if user is active
	if !user.IsActive {
		return nil, fmt.Errorf("account is inactive")
	}

	// Update last login
	now := time.Now()
	p.db.Model(&user).Update("last_login", now)
	user.LastLogin = &now

	return &user, nil
}
