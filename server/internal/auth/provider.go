package auth

import (
	"context"
	"wire-socket-server/internal/database"
)

// ProviderType represents the type of authentication provider
type ProviderType string

const (
	ProviderTypeLocal  ProviderType = "local"
	ProviderTypeOIDC   ProviderType = "oidc"
	ProviderTypeOAuth2 ProviderType = "oauth2"
)

// ProviderInfo contains information about an authentication provider
type ProviderInfo struct {
	ID   string       `json:"id"`
	Type ProviderType `json:"type"`
	Name string       `json:"name"`
}

// SSOUserInfo contains user information from SSO provider
type SSOUserInfo struct {
	ExternalID string   // User ID from IdP
	Username   string   // Mapped username
	Email      string   // Email address
	Groups     []string // Group memberships (for admin check)
}

// Provider defines the interface for authentication providers
type Provider interface {
	// GetInfo returns information about the provider
	GetInfo() ProviderInfo

	// Authenticate validates credentials and returns user info
	// For local provider: uses username/password
	// For SSO providers: validates the authorization code
	Authenticate(ctx context.Context, credentials map[string]string) (*database.User, error)
}

// SSOProvider extends Provider with SSO-specific methods
type SSOProvider interface {
	Provider

	// GetAuthURL returns the URL to redirect the user for authentication
	GetAuthURL(state string, redirectURI string) string

	// ExchangeCode exchanges an authorization code for user info
	ExchangeCode(ctx context.Context, code string, redirectURI string) (*SSOUserInfo, error)
}
