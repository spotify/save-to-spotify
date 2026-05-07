package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestUserAgentTransport_SetsDefaultUserAgent(t *testing.T) {
	var gotUserAgent string

	rt := UserAgentTransport{
		UserAgent: "save-to-spotify/test",
		Base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotUserAgent = req.Header.Get("User-Agent")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(http.NoBody),
				Header:     make(http.Header),
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()

	if gotUserAgent != "save-to-spotify/test" {
		t.Fatalf("User-Agent = %q, want %q", gotUserAgent, "save-to-spotify/test")
	}
}

func TestUserAgentTransport_PreservesExistingUserAgent(t *testing.T) {
	var gotUserAgent string

	rt := UserAgentTransport{
		UserAgent: "save-to-spotify/test",
		Base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotUserAgent = req.Header.Get("User-Agent")
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(http.NoBody),
				Header:     make(http.Header),
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("User-Agent", "existing-agent")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()

	if gotUserAgent != "existing-agent" {
		t.Fatalf("User-Agent = %q, want %q", gotUserAgent, "existing-agent")
	}
}

func TestUserAgentTransport_DoesNotMutateOriginalRequest(t *testing.T) {
	rt := UserAgentTransport{
		UserAgent: "save-to-spotify/test",
		Base: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(http.NoBody),
				Header:     make(http.Header),
			}, nil
		}),
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()

	if got := req.Header.Get("User-Agent"); got != "" {
		t.Fatalf("original request User-Agent = %q, want empty", got)
	}
}

func TestUserAgentTransport_UsesDefaultTransportWhenBaseNil(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "save-to-spotify/test" {
			t.Fatalf("User-Agent = %q, want %q", got, "save-to-spotify/test")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	rt := UserAgentTransport{UserAgent: "save-to-spotify/test"}

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}
