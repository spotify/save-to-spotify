package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v1, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier: %v", err)
	}
	v2, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier: %v", err)
	}

	if v1 == v2 {
		t.Error("two verifiers were identical — expected random values")
	}

	// RFC 7636: 43..128 chars
	if len(v1) < 43 || len(v1) > 128 {
		t.Errorf("verifier length = %d, want 43..128", len(v1))
	}

	// Base64url unpadded — must decode cleanly
	if _, err := base64.RawURLEncoding.DecodeString(v1); err != nil {
		t.Errorf("verifier is not valid base64url: %v", err)
	}
}

func TestGenerateCodeChallenge_MatchesSHA256(t *testing.T) {
	verifier := "test-verifier-value"
	challenge := GenerateCodeChallenge(verifier)

	hash := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(hash[:])

	if challenge != want {
		t.Errorf("challenge = %q, want %q", challenge, want)
	}
}

func TestGenerateCodeChallenge_Deterministic(t *testing.T) {
	verifier := "a-fixed-verifier"
	if GenerateCodeChallenge(verifier) != GenerateCodeChallenge(verifier) {
		t.Error("same verifier produced different challenges")
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}
	s2, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState: %v", err)
	}

	if s1 == s2 {
		t.Error("two states were identical — expected random values")
	}
	if s1 == "" {
		t.Error("state is empty")
	}
	if _, err := base64.RawURLEncoding.DecodeString(s1); err != nil {
		t.Errorf("state is not valid base64url: %v", err)
	}
}
