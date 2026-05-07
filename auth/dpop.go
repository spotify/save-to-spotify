package auth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"
)

const (
	jwkTypeEC    = "EC"
	jwkCurveP256 = "P-256"
	dpopJWTType  = "dpop+jwt"
	dpopSignAlg  = "ES256"
)

// DPoPKey holds an ECDSA P-256 key pair for DPoP proof-of-possession (RFC 9449).
type DPoPKey struct {
	PrivateKey *ecdsa.PrivateKey
}

// GenerateDPoPKey creates a new ECDSA P-256 key pair for DPoP.
func GenerateDPoPKey() (*DPoPKey, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate DPoP key: %w", err)
	}
	return &DPoPKey{PrivateKey: key}, nil
}

// dpopJWK is the JWK representation of the EC key (public or private).
type dpopJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	D   string `json:"d,omitempty"`
}

// publicJWK returns the public key in JWK format.
func (k *DPoPKey) publicJWK() dpopJWK {
	pub := k.PrivateKey.PublicKey
	return dpopJWK{
		Kty: jwkTypeEC,
		Crv: jwkCurveP256,
		X:   base64URLEncode(padTo32(pub.X.Bytes())),
		Y:   base64URLEncode(padTo32(pub.Y.Bytes())),
	}
}

// MarshalJSON serializes the full key pair (including private key) for persistence.
func (k *DPoPKey) MarshalJSON() ([]byte, error) {
	jwk := k.publicJWK()
	jwk.D = base64URLEncode(padTo32(k.PrivateKey.D.Bytes()))
	return json.Marshal(jwk)
}

// UnmarshalDPoPKey deserializes a DPoP key pair from its JSON representation.
func UnmarshalDPoPKey(data []byte) (*DPoPKey, error) {
	var jwk dpopJWK
	if err := json.Unmarshal(data, &jwk); err != nil {
		return nil, fmt.Errorf("failed to parse DPoP key: %w", err)
	}
	if jwk.Kty != jwkTypeEC || jwk.Crv != jwkCurveP256 {
		return nil, fmt.Errorf("unsupported DPoP key type: %s/%s", jwk.Kty, jwk.Crv)
	}

	xBytes, err := decodeJWKField("x", jwk.X)
	if err != nil {
		return nil, err
	}
	yBytes, err := decodeJWKField("y", jwk.Y)
	if err != nil {
		return nil, err
	}
	dBytes, err := decodeJWKField("d", jwk.D)
	if err != nil {
		return nil, err
	}

	key := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(xBytes),
			Y:     new(big.Int).SetBytes(yBytes),
		},
		D: new(big.Int).SetBytes(dBytes),
	}

	return &DPoPKey{PrivateKey: key}, nil
}

// CreateProof generates a DPoP proof JWT for the given HTTP method and URL.
// nonce is included when non-empty (for server-provided DPoP-Nonce).
func (k *DPoPKey) CreateProof(method, rawURL, nonce string) (string, error) {
	htu, err := canonicalHTU(rawURL)
	if err != nil {
		return "", err
	}

	header := map[string]any{
		"typ": dpopJWTType,
		"alg": dpopSignAlg,
		"jwk": k.publicJWK(),
	}

	jtiBytes := make([]byte, 16)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", fmt.Errorf("failed to generate jti: %w", err)
	}

	payload := map[string]any{
		"jti": base64URLEncode(jtiBytes),
		"htm": method,
		"htu": htu,
		"iat": time.Now().Unix(),
	}

	if nonce != "" {
		payload["nonce"] = nonce
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DPoP header: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal DPoP payload: %w", err)
	}

	headerB64 := base64URLEncode(headerJSON)
	payloadB64 := base64URLEncode(payloadJSON)

	signingInput := headerB64 + "." + payloadB64
	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, k.PrivateKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign DPoP proof: %w", err)
	}

	// ES256 signature: r || s, each zero-padded to 32 bytes
	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)

	return headerB64 + "." + payloadB64 + "." + base64URLEncode(sig), nil
}

// Thumbprint returns the base64url-encoded SHA-256 JWK Thumbprint (RFC 7638).
func (k *DPoPKey) Thumbprint() string {
	jwk := k.publicJWK()
	// RFC 7638: required members in lexicographic order for EC keys
	canonical := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s","y":"%s"}`,
		jwk.Crv, jwk.Kty, jwk.X, jwk.Y)
	hash := sha256.Sum256([]byte(canonical))
	return base64URLEncode(hash[:])
}

// canonicalHTU returns the URL without query and fragment per RFC 9449.
func canonicalHTU(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL for DPoP proof: %w", err)
	}
	var b strings.Builder
	b.WriteString(parsed.Scheme)
	b.WriteString("://")
	b.WriteString(parsed.Host)
	b.WriteString(parsed.Path)
	return b.String(), nil
}

func decodeJWKField(name, value string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid DPoP key %s: %w", name, err)
	}
	return b, nil
}

// padTo32 left-pads b to 32 bytes (P-256 coordinate size).
func padTo32(b []byte) []byte {
	if len(b) >= 32 {
		return b
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}
