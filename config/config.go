package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ClientID             = "76764a523fed47e381243dc19dee5804"
	AuthURL              = "https://accounts.spotify.com/authorize"
	RedirectURI          = "http://127.0.0.1:%d/callback"
	RedirectPort         = 8085
	EnvVarAuthToken      = "SAVE_TO_SPOTIFY_AUTH_TOKEN"
	EnvVarBackendURL     = "SAVE_TO_SPOTIFY_BACKEND_URL"
	EnvVarTimeout        = "SAVE_TO_SPOTIFY_TIMEOUT"
	EnvVarClientID       = "SAVE_TO_SPOTIFY_CLIENT_ID"
	EnvVarNoUpdateCheck  = "SAVE_TO_SPOTIFY_NO_UPDATE_CHECK"
	EnvVarReleasesAPIURL = "SAVE_TO_SPOTIFY_RELEASES_API_URL"

	Scopes = "sts-content-management"

	MaxMediaFileSize = 1 << 30 // 1 GB in bytes
)

var TokenURL = "https://accounts.spotify.com/api/token"

var AllowedMediaExtensions = map[string]bool{
	".mp3": true, ".m4a": true, ".mp4": true,
	".mov": true, ".wav": true, ".ogg": true,
}

var BackendBaseURL = getBackendBaseURL()

// ReleasesAPIURL is the full URL for fetching the latest release version.
// Defaults to the backend service endpoint; override via SAVE_TO_SPOTIFY_RELEASES_API_URL.
var ReleasesAPIURL = getReleasesAPIURL()

func getBackendBaseURL() string {
	if u := os.Getenv(EnvVarBackendURL); u != "" {
		return u
	}
	return "https://saveto.spotify.com"
}

func getReleasesAPIURL() string {
	if u := os.Getenv(EnvVarReleasesAPIURL); u != "" {
		return u
	}
	return getBackendBaseURL() + "/api/v1/cli/releases/latest"
}

func backendURL(path string) string {
	return BackendBaseURL + "/api/v1" + path
}

// BackendURLPath builds a full backend URL from trusted route segments and
// caller-supplied resource IDs without allowing path, query, or fragment
// delimiters to change the request target.
func BackendURLPath(segments ...string) (string, error) {
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		if !isSafeBackendPathSegment(segment) {
			return "", fmt.Errorf("backend URL path segment %q contains unsafe characters; use a trusted Spotify ID or URI, and do not edit untrusted input to make it fit", segment)
		}
		escaped[i] = url.PathEscape(segment)
	}
	return backendURL("/" + strings.Join(escaped, "/")), nil
}

func isSafeBackendPathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." {
		return false
	}
	for _, r := range segment {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		switch r {
		case '-', '.', '_', '~':
			continue
		default:
			return false
		}
	}
	return true
}

// jsonMode is set to true when --json is passed on the command line.
var jsonMode bool

// SetJSONMode enables JSON output mode. Called by the CLI flag parser.
func SetJSONMode() { jsonMode = true }

// ResetJSONMode disables JSON output mode. Used in tests.
func ResetJSONMode() { jsonMode = false }

// JSONMode reports whether JSON output mode is active.
func JSONMode() bool { return jsonMode }

// apiTimeout is the timeout for API requests. Override via --timeout or EnvVarTimeout.
var apiTimeout = getAPITimeout()

func getAPITimeout() time.Duration {
	if s := os.Getenv(EnvVarTimeout); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return 30 * time.Second
}

// SetAPITimeout overrides the API request timeout.
func SetAPITimeout(d time.Duration) { apiTimeout = d }

// APITimeout returns the current API request timeout.
func APITimeout() time.Duration { return apiTimeout }

// TokenData holds the persisted OAuth tokens.
type TokenData struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       string    `json:"scopes"`
}

// IsExpired returns true if the access token has expired (with a 60s buffer).
func (t *TokenData) IsExpired() bool {
	return time.Now().After(t.ExpiresAt.Add(-60 * time.Second))
}

// ConfigDir returns the configuration directory path, respecting XDG_CONFIG_HOME.
func ConfigDir() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine home directory: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "save-to-spotify"), nil
}

// TokenPath returns the full path to the token file.
func TokenPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token.json"), nil
}

// LoadToken reads the saved token from disk.
func LoadToken() (*TokenData, error) {
	path, err := TokenPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("not authenticated — run `save-to-spotify auth login` first")
		}
		return nil, fmt.Errorf("failed to read token file: %w", err)
	}

	var token TokenData
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("corrupt token file: %w", err)
	}

	return &token, nil
}

// SaveToken writes the token to disk with restricted permissions.
func SaveToken(token *TokenData) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	path := filepath.Join(dir, "token.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write token file: %w", err)
	}

	return nil
}

// DeleteToken removes the saved token file.
func DeleteToken() error {
	path, err := TokenPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove token file: %w", err)
	}

	return nil
}

// DPoPKeyPath returns the full path to the DPoP key file.
func DPoPKeyPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "dpop_key.json"), nil
}

// LoadDPoPKey reads the saved DPoP key from disk. Returns nil, nil if no key exists.
func LoadDPoPKey() ([]byte, error) {
	path, err := DPoPKeyPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read DPoP key: %w", err)
	}
	return data, nil
}

// SaveDPoPKey writes the DPoP key to disk with restricted permissions.
func SaveDPoPKey(data []byte) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	path := filepath.Join(dir, "dpop_key.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write DPoP key: %w", err)
	}
	return nil
}

// DeleteDPoPKey removes the saved DPoP key file.
func DeleteDPoPKey() error {
	path, err := DPoPKeyPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove DPoP key: %w", err)
	}
	return nil
}

// GetClientID returns the client ID, checking the environment variable first.
func GetClientID() string {
	if id := os.Getenv(EnvVarClientID); id != "" {
		return id
	}
	return ClientID
}
