package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// PKCE represents a Proof Key for Code Exchange
type PKCE struct {
	CodeVerifier  string
	CodeChallenge string
}

// GeneratePKCE generates a new PKCE code verifier and challenge
func GeneratePKCE() (*PKCE, error) {
	// Generate 32 random bytes for the verifier
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}

	// Base64 URL encode without padding
	codeVerifier := base64.RawURLEncoding.EncodeToString(verifierBytes)

	// SHA256 hash of the verifier
	hash := sha256.Sum256([]byte(codeVerifier))

	// Base64 URL encode without padding
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])

	return &PKCE{
		CodeVerifier:  codeVerifier,
		CodeChallenge: codeChallenge,
	}, nil
}
