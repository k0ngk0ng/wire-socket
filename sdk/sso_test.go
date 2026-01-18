package sdk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthProvider(t *testing.T) {
	provider := AuthProvider{
		ID:   "azure-ad",
		Type: "oidc",
		Name: "Microsoft Azure AD",
	}

	if provider.ID != "azure-ad" {
		t.Errorf("expected ID 'azure-ad', got '%s'", provider.ID)
	}
	if provider.Type != "oidc" {
		t.Errorf("expected Type 'oidc', got '%s'", provider.Type)
	}
	if provider.Name != "Microsoft Azure AD" {
		t.Errorf("expected Name 'Microsoft Azure AD', got '%s'", provider.Name)
	}
}

func TestFilterSSOProviders(t *testing.T) {
	providers := []AuthProvider{
		{ID: "local", Type: "local", Name: "Local Account"},
		{ID: "azure-ad", Type: "oidc", Name: "Microsoft"},
		{ID: "google", Type: "oauth2", Name: "Google"},
	}

	ssoProviders := FilterSSOProviders(providers)

	if len(ssoProviders) != 2 {
		t.Errorf("expected 2 SSO providers, got %d", len(ssoProviders))
	}

	for _, p := range ssoProviders {
		if p.Type == "local" {
			t.Error("FilterSSOProviders should not include local provider")
		}
	}
}

func TestFilterSSOProviders_CaseInsensitive(t *testing.T) {
	providers := []AuthProvider{
		{ID: "local", Type: "LOCAL", Name: "Local Account"},
		{ID: "local2", Type: "Local", Name: "Local Account 2"},
		{ID: "azure", Type: "OIDC", Name: "Azure"},
	}

	ssoProviders := FilterSSOProviders(providers)

	if len(ssoProviders) != 1 {
		t.Errorf("expected 1 SSO provider, got %d", len(ssoProviders))
	}
	if ssoProviders[0].ID != "azure" {
		t.Errorf("expected azure provider, got %s", ssoProviders[0].ID)
	}
}

func TestFilterSSOProviders_Empty(t *testing.T) {
	providers := []AuthProvider{}
	ssoProviders := FilterSSOProviders(providers)

	if len(ssoProviders) != 0 {
		t.Errorf("expected 0 SSO providers, got %d", len(ssoProviders))
	}
}

func TestHasSSOProviders(t *testing.T) {
	tests := []struct {
		name      string
		providers []AuthProvider
		expected  bool
	}{
		{
			name:      "empty list",
			providers: []AuthProvider{},
			expected:  false,
		},
		{
			name: "only local",
			providers: []AuthProvider{
				{ID: "local", Type: "local", Name: "Local"},
			},
			expected: false,
		},
		{
			name: "has oidc",
			providers: []AuthProvider{
				{ID: "local", Type: "local", Name: "Local"},
				{ID: "azure", Type: "oidc", Name: "Azure"},
			},
			expected: true,
		},
		{
			name: "has oauth2",
			providers: []AuthProvider{
				{ID: "google", Type: "oauth2", Name: "Google"},
			},
			expected: true,
		},
		{
			name: "case insensitive local",
			providers: []AuthProvider{
				{ID: "local", Type: "LOCAL", Name: "Local"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSSOProviders(tt.providers)
			if result != tt.expected {
				t.Errorf("HasSSOProviders() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()

	if err != nil {
		t.Fatalf("GeneratePKCE failed: %v", err)
	}

	if verifier == "" {
		t.Error("verifier should not be empty")
	}

	if challenge == "" {
		t.Error("challenge should not be empty")
	}

	if verifier == challenge {
		t.Error("verifier and challenge should be different")
	}

	// Verify lengths are reasonable (base64 encoded 32 bytes)
	if len(verifier) < 40 {
		t.Errorf("verifier seems too short: %d", len(verifier))
	}
	if len(challenge) < 40 {
		t.Errorf("challenge seems too short: %d", len(challenge))
	}
}

func TestGeneratePKCE_Uniqueness(t *testing.T) {
	v1, c1, err1 := GeneratePKCE()
	v2, c2, err2 := GeneratePKCE()

	if err1 != nil || err2 != nil {
		t.Fatalf("GeneratePKCE failed: %v, %v", err1, err2)
	}

	if v1 == v2 {
		t.Error("two calls should produce different verifiers")
	}
	if c1 == c2 {
		t.Error("two calls should produce different challenges")
	}
}

func TestBuildSSOAuthURL(t *testing.T) {
	tests := []struct {
		name        string
		server      string
		providerID  string
		redirectURI string
		wantErr     bool
		wantContain string
	}{
		{
			name:        "valid parameters",
			server:      "https://vpn.example.com",
			providerID:  "azure-ad",
			redirectURI: "wiresocket://auth/callback",
			wantErr:     false,
			wantContain: "/api/auth/sso/azure-ad",
		},
		{
			name:        "server without https",
			server:      "vpn.example.com",
			providerID:  "google",
			redirectURI: "myapp://callback",
			wantErr:     false,
			wantContain: "https://vpn.example.com/api/auth/sso/google",
		},
		{
			name:        "empty server",
			server:      "",
			providerID:  "azure-ad",
			redirectURI: "wiresocket://auth/callback",
			wantErr:     true,
		},
		{
			name:        "empty provider",
			server:      "https://vpn.example.com",
			providerID:  "",
			redirectURI: "wiresocket://auth/callback",
			wantErr:     true,
		},
		{
			name:        "empty redirect URI",
			server:      "https://vpn.example.com",
			providerID:  "azure-ad",
			redirectURI: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := BuildSSOAuthURL(tt.server, tt.providerID, tt.redirectURI)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.wantContain != "" && !containsString(url, tt.wantContain) {
				t.Errorf("URL should contain '%s', got '%s'", tt.wantContain, url)
			}

			// Verify URL contains required parameters
			if !containsString(url, "redirect_uri=") {
				t.Error("URL should contain redirect_uri parameter")
			}
			if !containsString(url, "state=") {
				t.Error("URL should contain state parameter")
			}
		})
	}
}

func TestParseSSOCallbackURL(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		wantToken  string
		wantState  string
		wantErrMsg string
		wantErr    bool
	}{
		{
			name:      "valid callback with token",
			url:       "wiresocket://auth/callback?token=abc123&state=xyz789",
			wantToken: "abc123",
			wantState: "xyz789",
			wantErr:   false,
		},
		{
			name:      "valid callback without state",
			url:       "wiresocket://auth/callback?token=abc123",
			wantToken: "abc123",
			wantState: "",
			wantErr:   false,
		},
		{
			name:       "error in callback",
			url:        "wiresocket://auth/callback?error=access_denied&state=xyz789",
			wantErrMsg: "access_denied",
			wantState:  "xyz789",
			wantErr:    true,
		},
		{
			name:    "no token in callback",
			url:     "wiresocket://auth/callback?state=xyz789",
			wantErr: true,
		},
		{
			name:    "invalid URL",
			url:     "://invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, state, errMsg, err := ParseSSOCallbackURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				if tt.wantErrMsg != "" && errMsg != tt.wantErrMsg {
					t.Errorf("expected error message '%s', got '%s'", tt.wantErrMsg, errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if token != tt.wantToken {
				t.Errorf("expected token '%s', got '%s'", tt.wantToken, token)
			}
			if state != tt.wantState {
				t.Errorf("expected state '%s', got '%s'", tt.wantState, state)
			}
		})
	}
}

func TestSSOConfig(t *testing.T) {
	config := SSOConfig{
		Server:      "https://vpn.example.com",
		ProviderID:  "azure-ad",
		RedirectURI: "wiresocket://auth/callback",
		State:       "custom-state-123",
	}

	if config.Server != "https://vpn.example.com" {
		t.Errorf("unexpected Server: %s", config.Server)
	}
	if config.ProviderID != "azure-ad" {
		t.Errorf("unexpected ProviderID: %s", config.ProviderID)
	}
	if config.RedirectURI != "wiresocket://auth/callback" {
		t.Errorf("unexpected RedirectURI: %s", config.RedirectURI)
	}
	if config.State != "custom-state-123" {
		t.Errorf("unexpected State: %s", config.State)
	}
}

func TestSSOResult(t *testing.T) {
	result := SSOResult{
		Token: "jwt-token-here",
		State: "state-123",
		Error: "",
	}

	if result.Token != "jwt-token-here" {
		t.Errorf("unexpected Token: %s", result.Token)
	}
	if result.State != "state-123" {
		t.Errorf("unexpected State: %s", result.State)
	}
	if result.Error != "" {
		t.Errorf("unexpected Error: %s", result.Error)
	}
}

func TestGetSSOProviders_MockServer(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/providers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		providers := struct {
			Providers []AuthProvider `json:"providers"`
		}{
			Providers: []AuthProvider{
				{ID: "local", Type: "local", Name: "Local Account"},
				{ID: "azure-ad", Type: "oidc", Name: "Microsoft Azure AD"},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(providers)
	}))
	defer server.Close()

	providers, err := GetSSOProviders(server.URL)
	if err != nil {
		t.Fatalf("GetSSOProviders failed: %v", err)
	}

	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

func TestGetSSOProviders_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := GetSSOProviders(server.URL)
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestGetSSOProviders_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	_, err := GetSSOProviders(server.URL)
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

// Helper function
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
