package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// GenerateCodeVerifier creates a cryptographically random PKCE code verifier.
// Length is between 43-128 characters per RFC 7636.
func GenerateCodeVerifier() (string, error) {
	buf := make([]byte, 64)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64URLEncode(buf), nil
}

// GenerateCodeChallenge derives the S256 code challenge from a verifier.
func GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64URLEncode(hash[:])
}

// GenerateState creates a random state parameter for CSRF protection.
func GenerateState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64URLEncode(buf), nil
}

// base64URLEncode encodes bytes using unpadded base64url encoding (RFC 4648 §5).
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
