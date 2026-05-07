package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spotify/save-to-spotify/config"
	"github.com/spotify/save-to-spotify/internal/httpx"
)

const testUserAgent = "save-to-spotify/test-version"

func testTokenClient() *http.Client {
	return &http.Client{
		Timeout:   config.APITimeout(),
		Transport: httpx.UserAgentTransport{UserAgent: testUserAgent},
	}
}

func TestExchangeCode_DPoPProofSent(t *testing.T) {
	key, _ := GenerateDPoPKey()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dpopHeader := r.Header.Get("DPoP")
		if dpopHeader == "" {
			t.Error("DPoP header missing from token request")
		}
		if got := r.Header.Get("User-Agent"); got != testUserAgent {
			t.Fatalf("User-Agent = %q, want %q", got, testUserAgent)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "dpop-access-token",
			"token_type":    "DPoP",
			"expires_in":    3600,
			"refresh_token": "dpop-refresh-token",
			"scope":         "user-read-private",
		})
	}))
	defer server.Close()

	origURL := config.TokenURL
	config.TokenURL = server.URL + "/api/token"
	t.Cleanup(func() { config.TokenURL = origURL })

	token, err := ExchangeCode(testTokenClient(), "test-code", "test-verifier", 8085, key)
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if token.AccessToken != "dpop-access-token" {
		t.Errorf("AccessToken = %q, want dpop-access-token", token.AccessToken)
	}
}

func TestExchangeCode_NilDPoPKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("DPoP") != "" {
			t.Error("DPoP header should not be present when key is nil")
		}
		if got := r.Header.Get("User-Agent"); got != testUserAgent {
			t.Fatalf("User-Agent = %q, want %q", got, testUserAgent)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "bearer-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "refresh",
			"scope":         "user-read-private",
		})
	}))
	defer server.Close()

	origURL := config.TokenURL
	config.TokenURL = server.URL + "/api/token"
	t.Cleanup(func() { config.TokenURL = origURL })

	token, err := ExchangeCode(testTokenClient(), "test-code", "test-verifier", 8085, nil)
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if token.AccessToken != "bearer-token" {
		t.Errorf("AccessToken = %q, want bearer-token", token.AccessToken)
	}
}

func TestDoTokenRequest_NonceRetry(t *testing.T) {
	key, _ := GenerateDPoPKey()
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if got := r.Header.Get("User-Agent"); got != testUserAgent {
			t.Fatalf("User-Agent = %q, want %q", got, testUserAgent)
		}

		if attempts == 1 {
			// First attempt: require nonce
			w.Header().Set("DPoP-Nonce", "server-nonce-xyz")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{
				"error":             "use_dpop_nonce",
				"error_description": "Authorization server requires nonce in DPoP proof",
			})
			return
		}

		// Second attempt: verify nonce was included in proof
		// (We trust CreateProof includes it — the test for that is in dpop_test.go)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "nonce-token",
			"token_type":    "DPoP",
			"expires_in":    3600,
			"refresh_token": "nonce-refresh",
			"scope":         "user-read-private",
		})
	}))
	defer server.Close()

	origURL := config.TokenURL
	config.TokenURL = server.URL + "/api/token"
	t.Cleanup(func() { config.TokenURL = origURL })

	token, err := ExchangeCode(testTokenClient(), "test-code", "test-verifier", 8085, key)
	if err != nil {
		t.Fatalf("ExchangeCode with nonce retry: %v", err)
	}
	if token.AccessToken != "nonce-token" {
		t.Errorf("AccessToken = %q, want nonce-token", token.AccessToken)
	}
	if attempts != 2 {
		t.Errorf("server saw %d attempts, want 2", attempts)
	}
}

func TestRefreshAccessToken_DPoPProofSent(t *testing.T) {
	key, _ := GenerateDPoPKey()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("DPoP") == "" {
			t.Error("DPoP header missing from refresh request")
		}
		if got := r.Header.Get("User-Agent"); got != testUserAgent {
			t.Fatalf("User-Agent = %q, want %q", got, testUserAgent)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": "refreshed-token",
			"token_type":   "DPoP",
			"expires_in":   3600,
			"scope":        "user-read-private",
		})
	}))
	defer server.Close()

	origURL := config.TokenURL
	config.TokenURL = server.URL + "/api/token"
	t.Cleanup(func() { config.TokenURL = origURL })

	token, err := RefreshAccessToken(testTokenClient(), "old-refresh", key)
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	if token.AccessToken != "refreshed-token" {
		t.Errorf("AccessToken = %q, want refreshed-token", token.AccessToken)
	}
}

func TestBuildAuthURL_IncludesDPoPJKT(t *testing.T) {
	key, _ := GenerateDPoPKey()
	thumbprint := key.Thumbprint()

	url := BuildAuthURL(8085, "test-state", "test-challenge", thumbprint)

	if url == "" {
		t.Fatal("BuildAuthURL returned empty")
	}
	if !containsParam(url, "dpop_jkt", thumbprint) {
		t.Errorf("URL missing dpop_jkt parameter, got: %s", url)
	}
	if !containsParam(url, "code_challenge", "test-challenge") {
		t.Errorf("URL missing code_challenge parameter")
	}
}

func containsParam(rawURL, key, value string) bool {
	return strings.Contains(rawURL, key+"="+value)
}
