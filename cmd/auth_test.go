package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spotify/save-to-spotify/auth"
	"github.com/spotify/save-to-spotify/config"
)

func TestGetValidToken_EnvVarClearsDPoPKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "env-token")

	oldKey := dpopKey
	dpopKey, _ = auth.GenerateDPoPKey()
	t.Cleanup(func() { dpopKey = oldKey })

	_, err := getValidToken()
	if err != nil {
		t.Fatalf("getValidToken: %v", err)
	}

	if dpopKey != nil {
		t.Errorf("dpopKey should be nil when using %s", config.EnvVarAuthToken)
	}
}

func TestGetValidToken_LoadsDPoPKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "")

	token := &config.TokenData{
		AccessToken:  "file-token",
		RefreshToken: "file-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	key, _ := auth.GenerateDPoPKey()
	keyData, _ := key.MarshalJSON()
	if err := config.SaveDPoPKey(keyData); err != nil {
		t.Fatal(err)
	}

	oldKey := dpopKey
	dpopKey = nil
	t.Cleanup(func() { dpopKey = oldKey })

	_, err := getValidToken()
	if err != nil {
		t.Fatalf("getValidToken: %v", err)
	}

	if dpopKey == nil {
		t.Error("dpopKey should be loaded from disk")
	}
}

func TestGetValidToken_NoDPoPKeyFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "")

	token := &config.TokenData{
		AccessToken:  "file-token",
		RefreshToken: "file-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	oldKey := dpopKey
	dpopKey, _ = auth.GenerateDPoPKey() // set to non-nil
	t.Cleanup(func() { dpopKey = oldKey })

	_, err := getValidToken()
	if err != nil {
		t.Fatalf("getValidToken: %v", err)
	}

	if dpopKey != nil {
		t.Error("dpopKey should be nil when no key file exists")
	}
}

func TestGetValidToken_CorruptDPoPKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "")

	token := &config.TokenData{
		AccessToken:  "file-token",
		RefreshToken: "file-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	// Write a corrupt DPoP key file
	if err := config.SaveDPoPKey([]byte("not valid json")); err != nil {
		t.Fatal(err)
	}

	oldKey := dpopKey
	t.Cleanup(func() { dpopKey = oldKey })

	_, err := getValidToken()
	if err == nil {
		t.Fatal("expected error for corrupt DPoP key")
	}
	if !strings.Contains(err.Error(), "DPoP key") {
		t.Errorf("error = %q, should mention DPoP key", err)
	}
}

func TestAPIRequests_UseBearerWithDPoP(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "")

	// Save DPoP key to disk
	key, _ := auth.GenerateDPoPKey()
	keyData, _ := key.MarshalJSON()
	if err := config.SaveDPoPKey(keyData); err != nil {
		t.Fatal(err)
	}
	oldKey := dpopKey
	t.Cleanup(func() { dpopKey = oldKey })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shows" {
			http.Error(w, "not found", 404)
			return
		}

		// API requests should use Bearer, not DPoP
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer dpop-test-token" {
			t.Errorf("Authorization = %q, want Bearer dpop-test-token", authHeader)
		}
		if r.Header.Get("DPoP") != "" {
			t.Error("DPoP proof should not be sent to API endpoints")
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(showCreateResponse{
			ShowURI: "spotify:show:dpop123",
		})
	}))
	defer server.Close()

	token := &config.TokenData{
		AccessToken:  "dpop-test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	err := handleShowsCreate([]string{"--title", "DPoP Test Show"})
	if err != nil {
		t.Fatalf("handleShowsCreate: %v", err)
	}
}
