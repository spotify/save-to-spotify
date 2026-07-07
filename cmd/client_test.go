package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spotify/save-to-spotify/config"
)

// startBackendTestServer starts a test server that records request headers.
// If asBackend is true, config.BackendBaseURL is pointed at it so the
// additional-headers transport treats it as the backend host.
func startBackendTestServer(t *testing.T, asBackend bool) (*httptest.Server, *http.Header) {
	t.Helper()

	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)

	if asBackend {
		origURL := config.BackendBaseURL
		config.BackendBaseURL = srv.URL
		t.Cleanup(func() { config.BackendBaseURL = origURL })
	}

	return srv, &gotHeaders
}

func setAdditionalHeaders(t *testing.T, headers map[string]string) {
	t.Helper()
	orig := config.AdditionalHeaders()
	config.SetAdditionalHeaders(headers)
	t.Cleanup(func() { config.SetAdditionalHeaders(orig) })
}

func doTestAPIRequest(t *testing.T, srv *httptest.Server) {
	t.Helper()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/api/v1/shows", nil)
	resp, err := doAPIRequest(req, &config.TokenData{AccessToken: "test-token"})
	if err != nil {
		t.Fatalf("doAPIRequest: %v", err)
	}
	resp.Body.Close()
}

func TestDoAPIRequest_NoAdditionalHeaders(t *testing.T) {
	setAdditionalHeaders(t, nil)
	srv, gotHeaders := startBackendTestServer(t, true)
	doTestAPIRequest(t, srv)

	if got := gotHeaders.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := gotHeaders.Get("X-STS-Test"); got != "" {
		t.Errorf("X-STS-Test should be absent, got %q", got)
	}
}

func TestDoAPIRequest_WithAdditionalHeaders(t *testing.T) {
	setAdditionalHeaders(t, map[string]string{"X-STS-Test": "1"})
	srv, gotHeaders := startBackendTestServer(t, true)
	doTestAPIRequest(t, srv)

	if got := gotHeaders.Get("Authorization"); got != "Bearer test-token" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
	}
	if got := gotHeaders.Get("X-STS-Test"); got != "1" {
		t.Errorf("X-STS-Test = %q, want %q", got, "1")
	}
}

func TestDoAPIRequest_HeadersScopedToBackendHost(t *testing.T) {
	setAdditionalHeaders(t, map[string]string{"X-STS-Test": "1"})
	srv, gotHeaders := startBackendTestServer(t, false)
	doTestAPIRequest(t, srv)

	if got := gotHeaders.Get("X-STS-Test"); got != "" {
		t.Errorf("X-STS-Test should not be sent to non-backend host, got %q", got)
	}
}

func TestDoAPIRequest_InvalidHeaderValueDoesNotBreakRequests(t *testing.T) {
	setAdditionalHeaders(t, map[string]string{"X-STS-Bad": "a\nb", "X-STS-Ok": "1"})
	srv, gotHeaders := startBackendTestServer(t, true)
	doTestAPIRequest(t, srv)

	if got := gotHeaders.Get("X-STS-Bad"); got != "" {
		t.Errorf("X-STS-Bad should have been dropped, got %q", got)
	}
	if got := gotHeaders.Get("X-STS-Ok"); got != "1" {
		t.Errorf("X-STS-Ok = %q, want %q", got, "1")
	}
}
