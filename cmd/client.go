package cmd

import (
	"fmt"
	"net/http"
	"runtime"

	"github.com/spotify/save-to-spotify/auth"
	"github.com/spotify/save-to-spotify/config"
	"github.com/spotify/save-to-spotify/internal/httpx"
)

func cliUserAgent() string {
	uaVersion := version
	if commit != "" && commit != "unknown" {
		uaVersion += "+" + commit
	}
	return fmt.Sprintf("%s/%s %s/%s", binName, uaVersion, runtime.GOOS, runtime.GOARCH)
}

// httpClient is the shared HTTP client
// Its Timeout is set in Execute() after flag parsing (default: 30s, override via --timeout or config.EnvVarTimeout).
// The transport applies the CLI User-Agent while still delegating to http.DefaultTransport by default.
var httpClient = &http.Client{
	Timeout:   config.APITimeout(),
	Transport: httpx.UserAgentTransport{UserAgent: cliUserAgent()},
}

// uploadClient is used for signed GCS PUTs. No timeout — large files can take many minutes.
var uploadClient = &http.Client{Transport: httpx.UserAgentTransport{UserAgent: cliUserAgent()}}

// maxResponseBytes caps API response reads to prevent memory exhaustion.
const maxResponseBytes = 10 << 20 // 10 MB

// dpopKey holds the DPoP key pair loaded during getValidToken.
// Used only for token refresh — DPoP proofs are sent to the /api/token
// endpoint, not to resource API calls.
var dpopKey *auth.DPoPKey

const minCLIVersionHeader = "X-Min-CLI-Version"

// doAPIRequest performs a backend API request, applying Bearer auth and a
// default JSON Content-Type, and enforces the minimum supported CLI version
// header when present.
func doAPIRequest(req *http.Request, token *config.TokenData) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	if req.Body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if err := enforceMinCLIVersion(resp); err != nil {
		resp.Body.Close()
		return nil, err
	}

	return resp, nil
}

func enforceMinCLIVersion(resp *http.Response) error {
	minVersion := resp.Header.Get(minCLIVersionHeader)
	if minVersion == "" {
		return nil
	}

	if _, err := parseVersion(minVersion); err != nil {
		return nil
	}

	if isNewer(version, minVersion) {
		return fmt.Errorf(
			"this version of %s (%s) is no longer supported.\nThe minimum required version is %s.\nRun `%s update` to install it",
			binName,
			version,
			normalizeVersion(minVersion),
			binName,
		)
	}

	return nil
}
