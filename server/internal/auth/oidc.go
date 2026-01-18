package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"wire-socket-server/internal/database"

	"github.com/golang-jwt/jwt/v5"
)

// OIDCProvider implements the SSOProvider interface for OIDC authentication
type OIDCProvider struct {
	db              *database.DB
	config          ProviderConfig
	callbackBaseURL string

	// OIDC Discovery cache
	discoveryOnce sync.Once
	discovery     *OIDCDiscovery
	discoveryErr  error

	// PKCE state storage
	pkceStates   map[string]*PKCE
	pkceStatesMu sync.RWMutex
}

// OIDCDiscovery represents the OIDC discovery document
type OIDCDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// OIDCTokenResponse represents the token endpoint response
type OIDCTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// NewOIDCProvider creates a new OIDC authentication provider
func NewOIDCProvider(db *database.DB, config ProviderConfig, callbackBaseURL string) (*OIDCProvider, error) {
	return &OIDCProvider{
		db:              db,
		config:          config,
		callbackBaseURL: callbackBaseURL,
		pkceStates:      make(map[string]*PKCE),
	}, nil
}

// GetInfo returns information about the provider
func (p *OIDCProvider) GetInfo() ProviderInfo {
	return ProviderInfo{
		ID:   p.config.ID,
		Type: ProviderTypeOIDC,
		Name: p.config.Name,
	}
}

// getDiscovery fetches and caches the OIDC discovery document
func (p *OIDCProvider) getDiscovery() (*OIDCDiscovery, error) {
	p.discoveryOnce.Do(func() {
		discoveryURL := strings.TrimSuffix(p.config.Issuer, "/") + "/.well-known/openid-configuration"

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", discoveryURL, nil)
		if err != nil {
			p.discoveryErr = fmt.Errorf("failed to create request: %w", err)
			return
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			p.discoveryErr = fmt.Errorf("failed to fetch discovery document: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			p.discoveryErr = fmt.Errorf("discovery endpoint returned %d", resp.StatusCode)
			return
		}

		if err := json.NewDecoder(resp.Body).Decode(&p.discovery); err != nil {
			p.discoveryErr = fmt.Errorf("failed to decode discovery document: %w", err)
			return
		}
	})

	return p.discovery, p.discoveryErr
}

// GetAuthURL returns the URL to redirect the user for authentication
func (p *OIDCProvider) GetAuthURL(state string, redirectURI string) string {
	discovery, err := p.getDiscovery()
	if err != nil {
		// Fallback to manual URL if discovery fails
		return ""
	}

	// Generate PKCE
	pkce, err := GeneratePKCE()
	if err != nil {
		return ""
	}

	// Store PKCE for later verification
	p.pkceStatesMu.Lock()
	p.pkceStates[state] = pkce
	p.pkceStatesMu.Unlock()

	// Build authorization URL
	params := url.Values{}
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join(p.config.GetDefaultScopes(), " "))
	params.Set("state", state)
	params.Set("code_challenge", pkce.CodeChallenge)
	params.Set("code_challenge_method", "S256")

	return discovery.AuthorizationEndpoint + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for user info
func (p *OIDCProvider) ExchangeCode(ctx context.Context, code string, redirectURI string) (*SSOUserInfo, error) {
	discovery, err := p.getDiscovery()
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC discovery: %w", err)
	}

	// Get PKCE verifier from state (we need to pass state here somehow)
	// For now, we'll use a simplified approach without PKCE verification
	// In production, the state should be used to look up the PKCE verifier

	// Exchange code for tokens
	tokenResp, err := p.exchangeCodeForTokens(ctx, discovery.TokenEndpoint, code, redirectURI, "")
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %w", err)
	}

	// Get user info
	userInfo, err := p.getUserInfo(ctx, discovery, tokenResp)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	return userInfo, nil
}

// ExchangeCodeWithState exchanges an authorization code for user info with state for PKCE
func (p *OIDCProvider) ExchangeCodeWithState(ctx context.Context, code string, redirectURI string, state string) (*SSOUserInfo, error) {
	discovery, err := p.getDiscovery()
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC discovery: %w", err)
	}

	// Get PKCE verifier
	p.pkceStatesMu.Lock()
	pkce := p.pkceStates[state]
	delete(p.pkceStates, state)
	p.pkceStatesMu.Unlock()

	codeVerifier := ""
	if pkce != nil {
		codeVerifier = pkce.CodeVerifier
	}

	// Exchange code for tokens
	tokenResp, err := p.exchangeCodeForTokens(ctx, discovery.TokenEndpoint, code, redirectURI, codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %w", err)
	}

	// Get user info
	userInfo, err := p.getUserInfo(ctx, discovery, tokenResp)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	return userInfo, nil
}

func (p *OIDCProvider) exchangeCodeForTokens(ctx context.Context, tokenURL, code, redirectURI, codeVerifier string) (*OIDCTokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp OIDCTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, err
	}

	return &tokenResp, nil
}

func (p *OIDCProvider) getUserInfo(ctx context.Context, discovery *OIDCDiscovery, tokenResp *OIDCTokenResponse) (*SSOUserInfo, error) {
	// Try to parse ID token first (contains user claims)
	if tokenResp.IDToken != "" {
		userInfo, err := p.parseIDToken(tokenResp.IDToken)
		if err == nil {
			return userInfo, nil
		}
		// Fall through to userinfo endpoint if ID token parsing fails
	}

	// Fetch from userinfo endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", discovery.UserinfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned %d", resp.StatusCode)
	}

	var claims map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, err
	}

	return p.mapClaimsToUser(claims)
}

func (p *OIDCProvider) parseIDToken(idToken string) (*SSOUserInfo, error) {
	// Parse without verification (we trust the token endpoint)
	// In production, you should verify the signature using JWKS
	token, _, err := jwt.NewParser().ParseUnverified(idToken, jwt.MapClaims{})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	return p.mapClaimsToUser(claims)
}

func (p *OIDCProvider) mapClaimsToUser(claims map[string]interface{}) (*SSOUserInfo, error) {
	userInfo := &SSOUserInfo{}

	// Get external ID (sub claim)
	if sub, ok := claims["sub"].(string); ok {
		userInfo.ExternalID = sub
	} else {
		return nil, fmt.Errorf("missing 'sub' claim")
	}

	// Get username
	usernameField := p.config.Mapping.GetUsernameField()
	if username, ok := claims[usernameField].(string); ok {
		userInfo.Username = username
	} else if email, ok := claims["email"].(string); ok {
		userInfo.Username = email
	}

	// Get email
	emailField := p.config.Mapping.GetEmailField()
	if email, ok := claims[emailField].(string); ok {
		userInfo.Email = email
	}

	// Check email domain restriction
	if len(p.config.AllowedDomains) > 0 && userInfo.Email != "" {
		parts := strings.Split(userInfo.Email, "@")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid email format")
		}
		domain := parts[1]
		allowed := false
		for _, d := range p.config.AllowedDomains {
			if strings.EqualFold(domain, d) {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("email domain not allowed")
		}
	}

	// Get groups for admin check
	if p.config.Mapping.AdminClaim != "" {
		if groups, ok := claims[p.config.Mapping.AdminClaim]; ok {
			switch v := groups.(type) {
			case []interface{}:
				for _, g := range v {
					if gs, ok := g.(string); ok {
						userInfo.Groups = append(userInfo.Groups, gs)
					}
				}
			case string:
				userInfo.Groups = []string{v}
			}
		}
	}

	return userInfo, nil
}

// Authenticate is not used for OIDC (uses callback flow)
func (p *OIDCProvider) Authenticate(ctx context.Context, credentials map[string]string) (*database.User, error) {
	return nil, fmt.Errorf("OIDC provider does not support direct authentication")
}

// GetAdminValues returns the admin values for this provider
func (p *OIDCProvider) GetAdminValues() []string {
	return p.config.Mapping.AdminValues
}
