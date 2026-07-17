package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

// tokenResponse is the raw JSON from Spotify's /api/token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// CallbackResult is sent through the channel from the local HTTP callback handler.
type CallbackResult struct {
	Code             string
	State            string
	Error            string
	ErrorDescription string
}

// BuildAuthURL constructs the Spotify authorization URL with PKCE params.
// dpopJKT is the JWK SHA-256 thumbprint that binds the authorization code to the DPoP key.
func BuildAuthURL(port int, state, codeChallenge, dpopJKT string) string {
	redirectURI := fmt.Sprintf(config.RedirectURI, port)
	params := url.Values{
		"client_id":             {config.GetClientID()},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {config.Scopes},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {codeChallenge},
		"dpop_jkt":              {dpopJKT},
	}
	return config.AuthURL + "?" + params.Encode()
}

// StartCallbackServer starts a temporary HTTP server to receive the OAuth callback.
// It returns the server, a channel that receives the callback result, and any error.
func StartCallbackServer(port int, expectedState string) (*http.Server, <-chan CallbackResult, error) {
	resultCh := make(chan CallbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		result := CallbackResult{
			Code:             q.Get("code"),
			State:            q.Get("state"),
			Error:            q.Get("error"),
			ErrorDescription: q.Get("error_description"),
		}

		if result.Error != "" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, successPage("Authentication Failed",
				"Error: "+html.EscapeString(result.Error)+". You can close this tab.",
				false))
			resultCh <- result
			return
		}

		if result.State != expectedState {
			result.Error = "state mismatch — possible CSRF attack"
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, successPage("Authentication Failed",
				"State mismatch. You can close this tab.",
				false))
			resultCh <- result
			return
		}

		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, successPage("Authentication Successful",
			"You can close this tab and return to the terminal.",
			true))
		resultCh <- result
	})

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, fmt.Errorf("could not listen on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := srv.Serve(listener); err != http.ErrServerClosed {
			fmt.Printf("Callback server error: %v\n", err)
		}
	}()

	return srv, resultCh, nil
}

// WaitForCallback waits for the callback result with a timeout.
func WaitForCallback(srv *http.Server, resultCh <-chan CallbackResult, timeout time.Duration) (*CallbackResult, error) {
	select {
	case result := <-resultCh:
		// Give the browser a moment to render the success page
		time.Sleep(500 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)

		if result.Error != "" {
			return nil, fmt.Errorf("authorization failed: %s", formatAuthError(result.Error, result.ErrorDescription))
		}
		return &result, nil

	case <-time.After(timeout):
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		return nil, fmt.Errorf("timed out waiting for authorization (waited %s)", timeout)
	}
}

// ExchangeCode exchanges an authorization code for tokens using PKCE.
// If dpopKey is non-nil, a DPoP proof is included in the token request.
func ExchangeCode(client *http.Client, code, codeVerifier string, port int, dpopKey *DPoPKey) (*config.TokenData, error) {
	redirectURI := fmt.Sprintf(config.RedirectURI, port)

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {config.GetClientID()},
		"code_verifier": {codeVerifier},
	}

	return doTokenRequest(client, data, dpopKey)
}

// RefreshAccessToken uses a refresh token to get a new access token.
// If dpopKey is non-nil, a DPoP proof is included in the token request.
func RefreshAccessToken(client *http.Client, refreshToken string, dpopKey *DPoPKey) (*config.TokenData, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {config.GetClientID()},
	}

	return doTokenRequest(client, data, dpopKey)
}

const authMaxResponseBytes = 1 << 20 // 1 MB for token responses

// doTokenRequest performs the actual HTTP POST to Spotify's token endpoint.
// If dpopKey is non-nil, a DPoP proof header is included. On a use_dpop_nonce
// error the request is retried once with the server-provided nonce.
func doTokenRequest(client *http.Client, data url.Values, dpopKey *DPoPKey) (*config.TokenData, error) {
	var nonce string
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(context.Background(), "POST", config.TokenURL,
			strings.NewReader(data.Encode()))
		if err != nil {
			return nil, fmt.Errorf("failed to create token request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		if dpopKey != nil {
			proof, err := dpopKey.CreateProof("POST", config.TokenURL, nonce)
			if err != nil {
				return nil, fmt.Errorf("failed to create DPoP proof: %w", err)
			}
			// Use raw map to preserve "DPoP" casing — Header.Set would
			// canonicalize it to "Dpop".
			req.Header["DPoP"] = []string{proof}
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("token request failed: %w", err)
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, authMaxResponseBytes))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read token response: %w", err)
		}

		var tok tokenResponse
		if err := json.Unmarshal(body, &tok); err != nil {
			return nil, fmt.Errorf("failed to parse token response: %w", err)
		}

		// Retry with server-provided nonce if requested (RFC 9449 §8).
		// Header.Get canonicalizes both sides so "DPoP-Nonce" maps to the
		// same canonical key ("Dpop-Nonce") used when the response was parsed.
		if dpopKey != nil && attempt == 0 && tok.Error == "use_dpop_nonce" {
			if n := resp.Header.Get("Dpop-Nonce"); n != "" {
				nonce = n
				continue
			}
		}

		if tok.Error != "" {
			return nil, fmt.Errorf("token error: %s — %s", tok.Error, tok.ErrorDesc)
		}

		return &config.TokenData{
			AccessToken:  tok.AccessToken,
			RefreshToken: tok.RefreshToken,
			TokenType:    tok.TokenType,
			ExpiresAt:    time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second),
			Scopes:       tok.Scope,
		}, nil
	}

	return nil, fmt.Errorf("token request failed after DPoP nonce retry")
}

// ExtractCodeFromURL parses an authorization code from a pasted redirect URL
// and validates the state parameter for CSRF protection.
func ExtractCodeFromURL(rawURL, expectedState string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if errMsg := parsed.Query().Get("error"); errMsg != "" {
		return "", fmt.Errorf("authorization error: %s", formatAuthError(
			errMsg,
			parsed.Query().Get("error_description"),
		))
	}

	code := parsed.Query().Get("code")
	if code == "" {
		return "", fmt.Errorf("no authorization code found in URL")
	}

	if state := parsed.Query().Get("state"); state != expectedState {
		return "", fmt.Errorf("state mismatch — possible CSRF attack")
	}

	return code, nil
}

// formatAuthError builds a terminal-facing message from the OAuth error
// parameters returned by the authorization server (RFC 6749 §4.1.2.1).
func formatAuthError(code, description string) string {
	msg := code
	if description != "" {
		msg += " — " + description
	}
	return msg
}

func successPage(title, message string, success bool) string {
	color := "#e74c3c"
	icon := "✗"
	if success {
		color = "#1DB954"
		icon = "✓"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family:-apple-system,BlinkMacSystemFont,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#191414;color:white;">
  <div style="text-align:center;max-width:400px;padding:2rem;">
    <div style="font-size:4rem;color:%s;margin-bottom:1rem;">%s</div>
    <h1 style="margin:0 0 0.5rem 0;">%s</h1>
    <p style="color:#b3b3b3;">%s</p>
  </div>
</body>
</html>`, title, color, icon, title, message)
}
