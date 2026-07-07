package httpx

import (
	"net/http"
	"net/url"
)

// UserAgentTransport applies a default User-Agent header to outbound requests.
type UserAgentTransport struct {
	Base      http.RoundTripper
	UserAgent string
}

func (t UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if clone.Header == nil {
		clone.Header = make(http.Header)
	}
	if clone.Header.Get("User-Agent") == "" && t.UserAgent != "" {
		clone.Header.Set("User-Agent", t.UserAgent)
	}

	rt := t.Base
	if rt == nil {
		rt = http.DefaultTransport
	}

	return rt.RoundTrip(clone)
}

// BackendHeadersTransport applies extra headers to outbound requests that
// target the backend host, leaving requests to other hosts (token endpoints,
// signed storage URLs) untouched.
type BackendHeadersTransport struct {
	Base http.RoundTripper
	// Headers returns the headers to apply; called per request so runtime
	// changes (e.g. in tests) are honored.
	Headers func() map[string]string
	// BackendURL returns the backend base URL used to scope injection.
	BackendURL func() string
}

func (t BackendHeadersTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if headers := t.headersFor(req); len(headers) > 0 {
		req = req.Clone(req.Context())
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	rt := t.Base
	if rt == nil {
		rt = http.DefaultTransport
	}

	return rt.RoundTrip(req)
}

func (t BackendHeadersTransport) headersFor(req *http.Request) map[string]string {
	if t.Headers == nil || t.BackendURL == nil || req.URL == nil {
		return nil
	}
	headers := t.Headers()
	if len(headers) == 0 {
		return nil
	}
	base, err := url.Parse(t.BackendURL())
	if err != nil || base.Scheme != req.URL.Scheme || base.Host != req.URL.Host {
		return nil
	}
	return headers
}
