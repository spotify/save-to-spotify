package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spotify/save-to-spotify/config"
)

func doTestAPIRequest(t *testing.T, headers map[string]string) http.Header {
	t.Helper()

	orig := config.AdditionalHeaders
	config.AdditionalHeaders = headers
	t.Cleanup(func() { config.AdditionalHeaders = orig })

	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	origURL := config.BackendBaseURL
	config.BackendBaseURL = srv.URL
	t.Cleanup(func() { config.BackendBaseURL = origURL })

	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/api/v1/shows", nil)
	resp, err := doAPIRequest(req, &config.TokenData{AccessToken: "test-token"})
	if err != nil {
		t.Fatalf("doAPIRequest: %v", err)
	}
	resp.Body.Close()
	return gotHeaders
}

func TestDoAPIRequest_NoAdditionalHeaders(t *testing.T) {
	h := doTestAPIRequest(t, map[string]string{})

	if got := h.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := h.Get("X-STS-Test"); got != "" {
		t.Errorf("X-STS-Test should be absent, got %q", got)
	}
}

func TestDoAPIRequest_WithAdditionalHeaders(t *testing.T) {
	h := doTestAPIRequest(t, map[string]string{"X-STS-Test": "1"})

	if got := h.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := h.Get("X-STS-Test"); got != "1" {
		t.Errorf("X-STS-Test = %q, want %q", got, "1")
	}
}
