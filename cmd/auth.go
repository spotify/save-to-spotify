package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spotify/save-to-spotify/auth"
	"github.com/spotify/save-to-spotify/config"
)

const callbackTimeout = 5 * time.Minute

func printAuthUsage() {
	fmt.Printf(`Usage: %s auth <command>

Commands:
  login    Authenticate with Spotify
  status   Show current authentication status
  logout   Remove stored credentials
`, binName)
}

func handleAuth(args []string) error {
	if len(args) == 0 {
		printAuthUsage()
		return nil
	}

	switch args[0] {
	case "login":
		return handleLogin(args[1:])
	case "status":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s auth status\n\nShow current authentication status.\n", binName)
			return nil
		}
		return handleStatus()
	case "logout":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s auth logout\n\nRemove stored credentials.\n", binName)
			return nil
		}
		return handleLogout()
	case "-h", "--help", "help":
		printAuthUsage()
		return nil
	default:
		return fmt.Errorf("unknown auth subcommand: %s", args[0])
	}
}

func handleLogin(args []string) error {
	noBrowser := false
	port := config.RedirectPort

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Printf(`Usage: %s auth login [flags]

Authenticate with Spotify via OAuth (PKCE).

Flags:
  --no-browser     Don't open a browser (for headless/remote servers)
  --port <port>    Local callback port (default: 8085)
`, binName)
			return nil
		case "--no-browser":
			noBrowser = true
		case "--port":
			if i+1 >= len(args) {
				return fmt.Errorf("--port requires a value")
			}
			i++
			p, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("invalid port: %s", args[i])
			}
			port = p
		}
	}

	verifier, err := auth.GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("failed to generate code verifier: %w", err)
	}
	challenge := auth.GenerateCodeChallenge(verifier)

	state, err := auth.GenerateState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	dpKey, err := auth.GenerateDPoPKey()
	if err != nil {
		return fmt.Errorf("failed to generate DPoP key: %w", err)
	}

	authURL := auth.BuildAuthURL(port, state, challenge, dpKey.Thumbprint())

	if noBrowser {
		return loginHeadless(authURL, verifier, port, state, dpKey)
	}
	return loginWithBrowser(authURL, verifier, port, state, dpKey)
}

func loginWithBrowser(authURL, verifier string, port int, state string, dpKey *auth.DPoPKey) error {
	srv, resultCh, err := auth.StartCallbackServer(port, state)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not start local server: %v\n", err)
		fmt.Println("Falling back to manual mode...")
		return loginHeadless(authURL, verifier, port, state, dpKey)
	}

	fmt.Println("Opening browser for Spotify authentication...")
	fmt.Println()

	if err := auth.OpenBrowser(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser: %v\n", err)
		fmt.Println("Falling back to manual mode...")
		srv.Close()
		return loginHeadless(authURL, verifier, port, state, dpKey)
	}

	fmt.Println("Waiting for authentication in the browser...")
	fmt.Printf("If the browser didn't open, visit:\n  %s\n\n", authURL)

	result, err := auth.WaitForCallback(srv, resultCh, callbackTimeout)
	if err != nil {
		return err
	}

	return exchangeAndSave(result.Code, verifier, port, dpKey)
}

func loginHeadless(authURL, verifier string, port int, state string, dpKey *auth.DPoPKey) error {
	fmt.Println("Open this URL in any browser to authenticate:")
	fmt.Println()
	fmt.Printf("  %s\n", authURL)
	fmt.Println()
	fmt.Println("After authorizing, you'll be redirected to a localhost URL.")
	fmt.Println("(It may show a connection error — that's expected on remote servers.)")
	fmt.Println()
	fmt.Print("Paste the full redirect URL here: ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	code, err := auth.ExtractCodeFromURL(input, state)
	if err != nil {
		return err
	}

	return exchangeAndSave(code, verifier, port, dpKey)
}

func exchangeAndSave(code, verifier string, port int, dpKey *auth.DPoPKey) error {
	fmt.Print("Exchanging authorization code for tokens...")

	token, err := auth.ExchangeCode(httpClient, code, verifier, port, dpKey)
	if err != nil {
		fmt.Println(" ✗")
		return err
	}
	fmt.Println(" ✓")

	if err := config.SaveToken(token); err != nil {
		return fmt.Errorf("authenticated but failed to save token: %w", err)
	}

	keyData, err := dpKey.MarshalJSON()
	if err != nil {
		return fmt.Errorf("authenticated but failed to marshal DPoP key: %w", err)
	}
	if err := config.SaveDPoPKey(keyData); err != nil {
		return fmt.Errorf("authenticated but failed to save DPoP key: %w", err)
	}

	path, _ := config.TokenPath()
	fmt.Println()
	fmt.Println("✓ Successfully authenticated with Spotify!")
	fmt.Printf("  Token saved to %s\n", path)
	fmt.Printf("  Expires at %s\n", token.ExpiresAt.Local().Format(time.RFC822))

	return nil
}

type authStatusOutput struct {
	Authenticated    bool   `json:"authenticated"`
	TokenValid       bool   `json:"token_valid,omitempty"`
	ExpiresInSeconds int    `json:"expires_in_seconds,omitempty"`
	Scopes           string `json:"scopes,omitempty"`
}

func handleStatus() error {
	if t := os.Getenv(config.EnvVarAuthToken); t != "" {
		if config.JSONMode() {
			return printJSON(authStatusOutput{
				Authenticated: true,
				TokenValid:    true,
			})
		}
		fmt.Printf("Status: Authenticated (via %s)\n", config.EnvVarAuthToken)
		fmt.Println("  Source:  environment variable")
		fmt.Println("  Token:   Valid (no expiry tracking for env tokens)")
		return nil
	}

	token, err := config.LoadToken()
	if err != nil {
		if config.JSONMode() {
			return printJSON(authStatusOutput{Authenticated: false})
		}
		fmt.Println("Status: Not authenticated")
		fmt.Printf("  Run `%s auth login` to get started.\n", binName)
		return nil
	}

	valid := !token.IsExpired()

	if config.JSONMode() {
		out := authStatusOutput{
			Authenticated:    true,
			TokenValid:       valid,
			ExpiresInSeconds: int(time.Until(token.ExpiresAt).Seconds()),
			Scopes:           token.Scopes,
		}
		if err := printJSON(out); err != nil {
			return err
		}
		if !valid {
			return &SilentError{Code: 1}
		}
		return nil
	}

	path, _ := config.TokenPath()
	if valid {
		fmt.Println("Status: Authenticated")
	} else {
		fmt.Println("Status: Token expired")
	}
	fmt.Printf("  Token file:  %s\n", path)
	fmt.Printf("  Scopes:      %s\n", token.Scopes)
	if valid {
		remaining := time.Until(token.ExpiresAt).Round(time.Second)
		fmt.Printf("  Token:       Valid (expires in %s)\n", remaining)
	} else {
		fmt.Println("  Token:       Expired (will auto-refresh on next use)")
		return &SilentError{Code: 1}
	}

	return nil
}

func handleLogout() error {
	if err := config.DeleteToken(); err != nil {
		return err
	}
	if err := config.DeleteDPoPKey(); err != nil {
		return err
	}
	if config.JSONMode() {
		return printJSON(map[string]bool{"ok": true})
	}
	fmt.Println("✓ Logged out. Stored credentials removed.")
	return nil
}

func handleToken() error {
	token, err := getValidToken()
	if err != nil {
		return err
	}
	if config.JSONMode() {
		return printJSON(map[string]any{"access_token": token.AccessToken})
	}
	fmt.Print(token.AccessToken)
	return nil
}

// getValidToken loads the token and refreshes if expired.
// It also loads the DPoP key into the package-level dpopKey variable.
func getValidToken() (*config.TokenData, error) {
	if t := os.Getenv(config.EnvVarAuthToken); t != "" {
		dpopKey = nil
		return &config.TokenData{
			AccessToken: t,
			TokenType:   "Bearer",
		}, nil
	}

	token, err := config.LoadToken()
	if err != nil {
		return nil, err
	}

	// Load DPoP key if available. Error means the key file exists but is
	// corrupt/unreadable — fail loudly rather than silently falling back to
	// Bearer (which would cause 401s for DPoP-bound tokens).
	if err := loadDPoPKey(); err != nil {
		return nil, fmt.Errorf("DPoP key is corrupt — run `%s auth login` again: %w", binName, err)
	}

	if !token.IsExpired() {
		return token, nil
	}

	info("Token expired, refreshing...\n")
	newToken, err := auth.RefreshAccessToken(httpClient, token.RefreshToken, dpopKey)
	if err != nil {
		return nil, fmt.Errorf("token refresh failed — run `%s auth login` again: %w", binName, err)
	}

	if newToken.RefreshToken == "" {
		newToken.RefreshToken = token.RefreshToken
	}

	if err := config.SaveToken(newToken); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save refreshed token: %v\n", err)
	}

	return newToken, nil
}

// loadDPoPKey loads the persisted DPoP key into the package-level dpopKey variable.
// Returns nil when no key file exists (legitimate Bearer fallback).
// Returns an error when the file exists but can't be read or parsed — the caller
// should fail rather than silently downgrade to Bearer.
func loadDPoPKey() error {
	keyData, err := config.LoadDPoPKey()
	if err != nil {
		dpopKey = nil
		return fmt.Errorf("failed to read DPoP key: %w", err)
	}
	if keyData == nil {
		dpopKey = nil
		return nil
	}
	key, err := auth.UnmarshalDPoPKey(keyData)
	if err != nil {
		dpopKey = nil
		return fmt.Errorf("failed to parse DPoP key: %w", err)
	}
	dpopKey = key
	return nil
}
