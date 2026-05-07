package httpx

import "net/http"

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
