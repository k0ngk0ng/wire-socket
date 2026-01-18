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
	"wire-socket-server/internal/database"
)

// OAuth2Provider implements the SSOProvider interface for OAuth 2.0 authentication
type OAuth2Provider struct {
	db              *database.DB
	config          ProviderConfig
	callbackBaseURL string

	// PKCE state storage
	pkceStates   map[string]*PKCE
	pkceStatesMu sync.RWMutex
}

// OAuth2TokenResponse represents the token endpoint response
type OAuth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// NewOAuth2Provider creates a new OAuth 2.0 authentication provider
func NewOAuth2Provider(db *database.DB, config ProviderConfig, callbackBaseURL string) (*OAuth2Provider, error) {
	// Validate required fields
	if config.AuthorizeURL == "" || config.TokenURL == "" || config.UserinfoURL == "" {
		return nil, fmt.Errorf("OAuth2 provider requires authorize_url, token_url, and userinfo_url")
	}

	return &OAuth2Provider{
		db:              db,
		config:          config,
		callbackBaseURL: callbackBaseURL,
		pkceStates:      make(map[string]*PKCE),
	}, nil
}

// GetInfo returns information about the provider
func (p *OAuth2Provider) GetInfo() ProviderInfo {
	return ProviderInfo{
		ID:   p.config.ID,
		Type: ProviderTypeOAuth2,
		Name: p.config.Name,
	}
}

// GetAuthURL returns the URL to redirect the user for authentication
func (p *OAuth2Provider) GetAuthURL(state string, redirectURI string) string {
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
	// Some OAuth2 providers support PKCE
	params.Set("code_challenge", pkce.CodeChallenge)
	params.Set("code_challenge_method", "S256")

	return p.config.AuthorizeURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for user info
func (p *OAuth2Provider) ExchangeCode(ctx context.Context, code string, redirectURI string) (*SSOUserInfo, error) {
	// Exchange code for tokens (without PKCE verifier)
	tokenResp, err := p.exchangeCodeForTokens(ctx, code, redirectURI, "")
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %w", err)
	}

	// Get user info
	userInfo, err := p.getUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Check organization membership if required
	if len(p.config.AllowedOrgs) > 0 && p.config.OrgsURL != "" {
		allowed, err := p.checkOrgMembership(ctx, tokenResp.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to check organization membership: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("user is not a member of allowed organizations")
		}
	}

	return userInfo, nil
}

// ExchangeCodeWithState exchanges an authorization code for user info with state for PKCE
func (p *OAuth2Provider) ExchangeCodeWithState(ctx context.Context, code string, redirectURI string, state string) (*SSOUserInfo, error) {
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
	tokenResp, err := p.exchangeCodeForTokens(ctx, code, redirectURI, codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %w", err)
	}

	// Get user info
	userInfo, err := p.getUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Check organization membership if required
	if len(p.config.AllowedOrgs) > 0 && p.config.OrgsURL != "" {
		allowed, err := p.checkOrgMembership(ctx, tokenResp.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to check organization membership: %w", err)
		}
		if !allowed {
			return nil, fmt.Errorf("user is not a member of allowed organizations")
		}
	}

	return userInfo, nil
}

func (p *OAuth2Provider) exchangeCodeForTokens(ctx context.Context, code, redirectURI, codeVerifier string) (*OAuth2TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("client_id", p.config.ClientID)
	data.Set("client_secret", p.config.ClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	if codeVerifier != "" {
		data.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.config.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json") // GitHub requires this

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	// Some providers return form-encoded response (like GitHub)
	var tokenResp OAuth2TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		// Try parsing as form-encoded
		values, parseErr := url.ParseQuery(string(body))
		if parseErr != nil {
			return nil, fmt.Errorf("failed to parse token response: %w", err)
		}
		tokenResp.AccessToken = values.Get("access_token")
		tokenResp.TokenType = values.Get("token_type")
		tokenResp.Scope = values.Get("scope")
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	return &tokenResp, nil
}

func (p *OAuth2Provider) getUserInfo(ctx context.Context, accessToken string) (*SSOUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.config.UserinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	// For GitHub, use token in different format
	if strings.Contains(p.config.UserinfoURL, "github.com") {
		req.Header.Set("Authorization", "token "+accessToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var claims map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil, err
	}

	return p.mapClaimsToUser(claims)
}

func (p *OAuth2Provider) mapClaimsToUser(claims map[string]interface{}) (*SSOUserInfo, error) {
	userInfo := &SSOUserInfo{}

	// Get external ID
	// Different providers use different fields
	if id, ok := claims["id"].(float64); ok {
		userInfo.ExternalID = fmt.Sprintf("%.0f", id)
	} else if id, ok := claims["id"].(string); ok {
		userInfo.ExternalID = id
	} else if sub, ok := claims["sub"].(string); ok {
		userInfo.ExternalID = sub
	} else {
		return nil, fmt.Errorf("missing user ID in response")
	}

	// Get username
	usernameField := p.config.Mapping.GetUsernameField()
	if username, ok := claims[usernameField].(string); ok {
		userInfo.Username = username
	} else if login, ok := claims["login"].(string); ok {
		// GitHub uses "login" for username
		userInfo.Username = login
	} else if name, ok := claims["name"].(string); ok {
		userInfo.Username = name
	}

	// Get email
	emailField := p.config.Mapping.GetEmailField()
	if email, ok := claims[emailField].(string); ok {
		userInfo.Email = email
	}

	// Check email domain restriction
	if len(p.config.AllowedDomains) > 0 && userInfo.Email != "" {
		parts := strings.Split(userInfo.Email, "@")
		if len(parts) == 2 {
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
	}

	return userInfo, nil
}

func (p *OAuth2Provider) checkOrgMembership(ctx context.Context, accessToken string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.config.OrgsURL, nil)
	if err != nil {
		return false, err
	}

	// For GitHub
	if strings.Contains(p.config.OrgsURL, "github.com") {
		req.Header.Set("Authorization", "token "+accessToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("organizations endpoint returned %d", resp.StatusCode)
	}

	var orgs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&orgs); err != nil {
		return false, err
	}

	for _, org := range orgs {
		var orgName string
		if login, ok := org["login"].(string); ok {
			orgName = login
		} else if name, ok := org["name"].(string); ok {
			orgName = name
		}

		for _, allowed := range p.config.AllowedOrgs {
			if strings.EqualFold(orgName, allowed) {
				return true, nil
			}
		}
	}

	return false, nil
}

// Authenticate is not used for OAuth2 (uses callback flow)
func (p *OAuth2Provider) Authenticate(ctx context.Context, credentials map[string]string) (*database.User, error) {
	return nil, fmt.Errorf("OAuth2 provider does not support direct authentication")
}

// GetAdminValues returns the admin values for this provider
func (p *OAuth2Provider) GetAdminValues() []string {
	return p.config.Mapping.AdminValues
}
