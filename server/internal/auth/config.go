package auth

// SSOConfig represents the SSO configuration
type SSOConfig struct {
	Enabled         bool             `yaml:"enabled"`
	CallbackBaseURL string           `yaml:"callback_base_url"`
	Providers       []ProviderConfig `yaml:"providers"`
}

// ProviderConfig represents a single SSO provider configuration
type ProviderConfig struct {
	ID       string       `yaml:"id"`
	Type     ProviderType `yaml:"type"`
	Name     string       `yaml:"name"`
	Enabled  bool         `yaml:"enabled"`

	// OIDC specific
	Issuer string `yaml:"issuer,omitempty"`

	// OAuth2 specific (also used if OIDC discovery fails)
	AuthorizeURL string `yaml:"authorize_url,omitempty"`
	TokenURL     string `yaml:"token_url,omitempty"`
	UserinfoURL  string `yaml:"userinfo_url,omitempty"`

	// Common OAuth settings
	ClientID     string   `yaml:"client_id"`
	ClientSecret string   `yaml:"client_secret"`
	Scopes       []string `yaml:"scopes,omitempty"`

	// User mapping
	Mapping UserMapping `yaml:"mapping,omitempty"`

	// Access control
	AllowedDomains []string `yaml:"allowed_domains,omitempty"` // Restrict by email domain
	AllowedOrgs    []string `yaml:"allowed_orgs,omitempty"`    // Restrict by organization (GitHub/GitLab)
	OrgsURL        string   `yaml:"orgs_url,omitempty"`        // URL to fetch user organizations
}

// UserMapping defines how to map IdP claims to user fields
type UserMapping struct {
	Username    string   `yaml:"username,omitempty"`     // Claim to use as username (default: email)
	Email       string   `yaml:"email,omitempty"`        // Claim to use as email (default: email)
	AdminClaim  string   `yaml:"admin_claim,omitempty"`  // Claim to check for admin status
	AdminValues []string `yaml:"admin_values,omitempty"` // Values that indicate admin status
}

// GetDefaultScopes returns default scopes for the provider type
func (c *ProviderConfig) GetDefaultScopes() []string {
	if len(c.Scopes) > 0 {
		return c.Scopes
	}

	switch c.Type {
	case ProviderTypeOIDC:
		return []string{"openid", "profile", "email"}
	case ProviderTypeOAuth2:
		return []string{"user:email"}
	default:
		return nil
	}
}

// GetUsernameField returns the field to use for username mapping
func (m *UserMapping) GetUsernameField() string {
	if m.Username != "" {
		return m.Username
	}
	return "email"
}

// GetEmailField returns the field to use for email mapping
func (m *UserMapping) GetEmailField() string {
	if m.Email != "" {
		return m.Email
	}
	return "email"
}
