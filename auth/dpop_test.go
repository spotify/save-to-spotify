package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
)

func TestGenerateDPoPKey(t *testing.T) {
	key, err := GenerateDPoPKey()
	if err != nil {
		t.Fatalf("GenerateDPoPKey: %v", err)
	}
	if key.PrivateKey == nil {
		t.Fatal("PrivateKey is nil")
	}
	if key.PrivateKey.Curve != elliptic.P256() {
		t.Error("curve is not P-256")
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	key, err := GenerateDPoPKey()
	if err != nil {
		t.Fatalf("GenerateDPoPKey: %v", err)
	}

	data, err := key.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	restored, err := UnmarshalDPoPKey(data)
	if err != nil {
		t.Fatalf("UnmarshalDPoPKey: %v", err)
	}

	if key.PrivateKey.D.Cmp(restored.PrivateKey.D) != 0 {
		t.Error("private key D component differs after round-trip")
	}
	if key.PrivateKey.PublicKey.X.Cmp(restored.PrivateKey.PublicKey.X) != 0 {
		t.Error("public key X component differs after round-trip")
	}
	if key.PrivateKey.PublicKey.Y.Cmp(restored.PrivateKey.PublicKey.Y) != 0 {
		t.Error("public key Y component differs after round-trip")
	}
}

func TestMarshalJSON_IncludesPrivateKey(t *testing.T) {
	key, err := GenerateDPoPKey()
	if err != nil {
		t.Fatal(err)
	}

	data, err := key.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}

	var jwk map[string]string
	if err := json.Unmarshal(data, &jwk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if jwk["kty"] != "EC" {
		t.Errorf("kty = %q, want EC", jwk["kty"])
	}
	if jwk["crv"] != "P-256" {
		t.Errorf("crv = %q, want P-256", jwk["crv"])
	}
	if jwk["d"] == "" {
		t.Error("d (private key) is empty")
	}
	if jwk["x"] == "" {
		t.Error("x is empty")
	}
	if jwk["y"] == "" {
		t.Error("y is empty")
	}
}

func TestUnmarshalDPoPKey_InvalidType(t *testing.T) {
	data := []byte(`{"kty":"RSA","crv":"P-256","x":"AAAA","y":"BBBB","d":"CCCC"}`)
	_, err := UnmarshalDPoPKey(data)
	if err == nil {
		t.Fatal("expected error for unsupported key type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error = %q, should mention unsupported", err)
	}
}

func TestUnmarshalDPoPKey_InvalidJSON(t *testing.T) {
	_, err := UnmarshalDPoPKey([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestCreateProof_JWTStructure(t *testing.T) {
	key, _ := GenerateDPoPKey()

	proof, err := key.CreateProof("POST", "https://accounts.spotify.com/api/token", "")
	if err != nil {
		t.Fatalf("CreateProof: %v", err)
	}

	parts := strings.Split(proof, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d parts, want 3", len(parts))
	}

	// Decode header
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}

	if header["typ"] != "dpop+jwt" {
		t.Errorf("typ = %v, want dpop+jwt", header["typ"])
	}
	if header["alg"] != "ES256" {
		t.Errorf("alg = %v, want ES256", header["alg"])
	}
	jwk, ok := header["jwk"].(map[string]any)
	if !ok {
		t.Fatal("jwk not present in header")
	}
	if jwk["kty"] != "EC" {
		t.Errorf("jwk.kty = %v, want EC", jwk["kty"])
	}
	if jwk["crv"] != "P-256" {
		t.Errorf("jwk.crv = %v, want P-256", jwk["crv"])
	}
	// jwk should NOT contain "d" (private key)
	if _, hasD := jwk["d"]; hasD {
		t.Error("jwk in header should not contain private key d")
	}
}

func TestCreateProof_Claims(t *testing.T) {
	key, _ := GenerateDPoPKey()

	proof, err := key.CreateProof("GET", "https://api.spotify.com/v1/me", "")
	if err != nil {
		t.Fatalf("CreateProof: %v", err)
	}

	parts := strings.Split(proof, ".")
	payloadJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var payload map[string]any
	json.Unmarshal(payloadJSON, &payload)

	if payload["htm"] != "GET" {
		t.Errorf("htm = %v, want GET", payload["htm"])
	}
	if payload["htu"] != "https://api.spotify.com/v1/me" {
		t.Errorf("htu = %v, want https://api.spotify.com/v1/me", payload["htu"])
	}
	if payload["jti"] == nil || payload["jti"] == "" {
		t.Error("jti is missing or empty")
	}
	if payload["iat"] == nil {
		t.Error("iat is missing")
	}
	// ath should never be present (Spotify does not use it)
	if _, hasAth := payload["ath"]; hasAth {
		t.Error("ath should not be present")
	}
	// No nonce when nonce is empty
	if _, hasNonce := payload["nonce"]; hasNonce {
		t.Error("nonce should not be present when nonce is empty")
	}
}

func TestCreateProof_WithNonce(t *testing.T) {
	key, _ := GenerateDPoPKey()

	proof, err := key.CreateProof("POST", "https://accounts.spotify.com/api/token", "server-nonce-42")
	if err != nil {
		t.Fatalf("CreateProof: %v", err)
	}

	parts := strings.Split(proof, ".")
	payloadJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var payload map[string]any
	json.Unmarshal(payloadJSON, &payload)

	if payload["nonce"] != "server-nonce-42" {
		t.Errorf("nonce = %v, want server-nonce-42", payload["nonce"])
	}
}

func TestCreateProof_URLStripping(t *testing.T) {
	key, _ := GenerateDPoPKey()

	proof, err := key.CreateProof("GET", "https://api.spotify.com/v1/shows?limit=10&offset=0#frag", "")
	if err != nil {
		t.Fatalf("CreateProof: %v", err)
	}

	parts := strings.Split(proof, ".")
	payloadJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var payload map[string]any
	json.Unmarshal(payloadJSON, &payload)

	if payload["htu"] != "https://api.spotify.com/v1/shows" {
		t.Errorf("htu = %v, want URL without query/fragment", payload["htu"])
	}
}

func TestCreateProof_SignatureVerifies(t *testing.T) {
	key, _ := GenerateDPoPKey()

	proof, err := key.CreateProof("POST", "https://accounts.spotify.com/api/token", "")
	if err != nil {
		t.Fatalf("CreateProof: %v", err)
	}

	parts := strings.Split(proof, ".")
	signingInput := parts[0] + "." + parts[1]
	hash := sha256.Sum256([]byte(signingInput))

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	if len(sigBytes) != 64 {
		t.Fatalf("signature length = %d, want 64", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	if !ecdsa.Verify(&key.PrivateKey.PublicKey, hash[:], r, s) {
		t.Error("signature does not verify with the key's public key")
	}
}

func TestCreateProof_UniqueJTI(t *testing.T) {
	key, _ := GenerateDPoPKey()

	proof1, _ := key.CreateProof("POST", "https://accounts.spotify.com/api/token", "")
	proof2, _ := key.CreateProof("POST", "https://accounts.spotify.com/api/token", "")

	parts1 := strings.Split(proof1, ".")
	parts2 := strings.Split(proof2, ".")

	payload1JSON, _ := base64.RawURLEncoding.DecodeString(parts1[1])
	payload2JSON, _ := base64.RawURLEncoding.DecodeString(parts2[1])

	var p1, p2 map[string]any
	json.Unmarshal(payload1JSON, &p1)
	json.Unmarshal(payload2JSON, &p2)

	if p1["jti"] == p2["jti"] {
		t.Error("two proofs have the same jti — should be unique")
	}
}

func TestThumbprint_Deterministic(t *testing.T) {
	key, _ := GenerateDPoPKey()

	tp1 := key.Thumbprint()
	tp2 := key.Thumbprint()

	if tp1 != tp2 {
		t.Error("same key produced different thumbprints")
	}
	if tp1 == "" {
		t.Error("thumbprint is empty")
	}
}

func TestThumbprint_DifferentKeys(t *testing.T) {
	key1, _ := GenerateDPoPKey()
	key2, _ := GenerateDPoPKey()

	if key1.Thumbprint() == key2.Thumbprint() {
		t.Error("different keys produced the same thumbprint")
	}
}

func TestCanonicalHTU(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://accounts.spotify.com/api/token", "https://accounts.spotify.com/api/token"},
		{"https://api.spotify.com/v1/me?fields=id", "https://api.spotify.com/v1/me"},
		{"https://example.com/path#fragment", "https://example.com/path"},
		{"https://example.com/path?q=1#frag", "https://example.com/path"},
	}

	for _, tt := range tests {
		got, err := canonicalHTU(tt.input)
		if err != nil {
			t.Errorf("canonicalHTU(%q): %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("canonicalHTU(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPadTo32(t *testing.T) {
	// Already 32 bytes — no change
	full := make([]byte, 32)
	full[0] = 0xff
	result := padTo32(full)
	if len(result) != 32 || result[0] != 0xff {
		t.Error("padTo32 should return input unchanged when already 32 bytes")
	}

	// Shorter — left-pad with zeros
	short := []byte{0x01, 0x02}
	result = padTo32(short)
	if len(result) != 32 {
		t.Fatalf("padTo32 length = %d, want 32", len(result))
	}
	if result[30] != 0x01 || result[31] != 0x02 {
		t.Error("padTo32 should left-pad: data at the end")
	}
	for i := 0; i < 30; i++ {
		if result[i] != 0 {
			t.Errorf("padTo32 byte[%d] = %d, want 0", i, result[i])
		}
	}

	// Longer than 32 — returned as-is
	long := make([]byte, 33)
	result = padTo32(long)
	if len(result) != 33 {
		t.Errorf("padTo32 should not truncate: len = %d", len(result))
	}
}
