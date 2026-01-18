// Package sdk provides SSO (Single Sign-On) authentication support.
package sdk

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AuthProvider represents an authentication provider
type AuthProvider struct {
	// ID is the unique identifier for the provider
	ID string `json:"id"`

	// Type is the provider type: "local", "oidc", or "oauth2"
	Type string `json:"type"`

	// Name is the display name for the provider
	Name string `json:"name"`
}

// SSOConfig contains SSO-specific configuration
type SSOConfig struct {
	// Server is the VPN server address
	Server string

	// ProviderID is the SSO provider to use
	ProviderID string

	// RedirectURI is the callback URI after SSO login
	// For desktop apps: "wiresocket://auth/callback"
	// For mobile apps: use app-specific scheme
	RedirectURI string

	// State is an optional state parameter for CSRF protection
	// If empty, a random state will be generated
	State string
}

// SSOResult contains the result of SSO authentication
type SSOResult struct {
	// Token is the JWT token from the server
	Token string

	// Error contains error message if authentication failed
	Error string

	// State is the state parameter returned from the IdP
	State string
}

// GetAuthProviders fetches available authentication providers from the server.
// This allows clients to display SSO options to users.
func (c *Client) GetAuthProviders(server string) ([]AuthProvider, error) {
	apiBase := normalizeServerURL(server)
	providersURL := apiBase + "/api/auth/providers"

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(providersURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch providers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var response struct {
		Providers []AuthProvider `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Providers, nil
}

// GetSSOAuthURL generates the SSO authorization URL.
// The user should be redirected to this URL in a browser to complete SSO login.
// After successful login, the IdP will redirect to the RedirectURI with a token or code.
func (c *Client) GetSSOAuthURL(config SSOConfig) (string, error) {
	if config.Server == "" {
		return "", fmt.Errorf("server address is required")
	}
	if config.ProviderID == "" {
		return "", fmt.Errorf("provider ID is required")
	}
	if config.RedirectURI == "" {
		return "", fmt.Errorf("redirect URI is required")
	}

	apiBase := normalizeServerURL(config.Server)

	// Generate state if not provided
	state := config.State
	if state == "" {
		state = generateRandomState()
	}

	// Build SSO URL
	ssoURL := fmt.Sprintf("%s/api/auth/sso/%s", apiBase, config.ProviderID)
	params := url.Values{}
	params.Set("redirect_uri", config.RedirectURI)
	params.Set("state", state)

	return ssoURL + "?" + params.Encode(), nil
}

// ParseSSOCallback parses the SSO callback URL and extracts the token or error.
// This should be called when the app receives the callback from the IdP.
func (c *Client) ParseSSOCallback(callbackURL string) (*SSOResult, error) {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse callback URL: %w", err)
	}

	result := &SSOResult{
		Token: parsed.Query().Get("token"),
		Error: parsed.Query().Get("error"),
		State: parsed.Query().Get("state"),
	}

	if result.Error != "" {
		return result, fmt.Errorf("SSO error: %s", result.Error)
	}

	if result.Token == "" {
		return result, fmt.Errorf("no token in callback")
	}

	return result, nil
}

// ConnectWithSSO performs a complete SSO connection flow.
// This is a convenience method that combines token-based authentication with VPN connection.
// The token should be obtained from the SSO callback.
func (c *Client) ConnectWithSSO(server, token string, options ...ConnectConfig) error {
	// Build config with token
	config := DefaultConnectConfig()
	if len(options) > 0 {
		config = options[0]
	}
	config.Server = server
	config.Token = token

	return c.Connect(config)
}

// ======== Helper functions ========

// generateRandomState generates a random state string for CSRF protection
func generateRandomState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based state
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// generatePKCE generates PKCE code verifier and challenge
// This can be used by clients that want to implement PKCE flow manually
func GeneratePKCE() (verifier, challenge string, err error) {
	// Generate 32 random bytes for verifier
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	verifier = base64.RawURLEncoding.EncodeToString(b)

	// Generate challenge using SHA256
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])

	return verifier, challenge, nil
}

// GetSSOProviders is a standalone function to get providers without creating a client
func GetSSOProviders(server string) ([]AuthProvider, error) {
	apiBase := normalizeServerURL(server)
	providersURL := apiBase + "/api/auth/providers"

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(providersURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch providers: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var response struct {
		Providers []AuthProvider `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Providers, nil
}

// BuildSSOAuthURL is a standalone function to build SSO URL without creating a client
func BuildSSOAuthURL(server, providerID, redirectURI string) (string, error) {
	if server == "" {
		return "", fmt.Errorf("server address is required")
	}
	if providerID == "" {
		return "", fmt.Errorf("provider ID is required")
	}
	if redirectURI == "" {
		return "", fmt.Errorf("redirect URI is required")
	}

	apiBase := normalizeServerURL(server)
	state := generateRandomState()

	ssoURL := fmt.Sprintf("%s/api/auth/sso/%s", apiBase, providerID)
	params := url.Values{}
	params.Set("redirect_uri", redirectURI)
	params.Set("state", state)

	return ssoURL + "?" + params.Encode(), nil
}

// ParseSSOCallbackURL is a standalone function to parse callback URL
func ParseSSOCallbackURL(callbackURL string) (token, state, errMsg string, err error) {
	parsed, err := url.Parse(callbackURL)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to parse callback URL: %w", err)
	}

	token = parsed.Query().Get("token")
	state = parsed.Query().Get("state")
	errMsg = parsed.Query().Get("error")

	if errMsg != "" {
		return "", state, errMsg, fmt.Errorf("SSO error: %s", errMsg)
	}

	if token == "" {
		return "", state, "", fmt.Errorf("no token in callback")
	}

	return token, state, "", nil
}

// FilterSSOProviders filters providers to only include SSO providers (not local)
func FilterSSOProviders(providers []AuthProvider) []AuthProvider {
	var sso []AuthProvider
	for _, p := range providers {
		if !strings.EqualFold(p.Type, "local") {
			sso = append(sso, p)
		}
	}
	return sso
}

// HasSSOProviders checks if the server has any SSO providers configured
func HasSSOProviders(providers []AuthProvider) bool {
	for _, p := range providers {
		if !strings.EqualFold(p.Type, "local") {
			return true
		}
	}
	return false
}
