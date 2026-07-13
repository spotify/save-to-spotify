package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spotify/save-to-spotify/config"
	"github.com/spotify/save-to-spotify/internal/httpx"
)

var (
	releaseMetadataTimeout = 5 * time.Second
	releaseDownloadTimeout = 2 * time.Minute
)

const maxReleaseAssetBytes = 200 << 20 // 200 MB

type latestReleaseResponse struct {
	Version  string `json:"version"`
	AssetURL string `json:"asset_url"`
	SHA256   string `json:"sha256"`
}

type updateResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	Updated         bool   `json:"updated"`
}

type semVersion struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease []preReleaseIdentifier
}

type preReleaseIdentifier struct {
	Raw     string
	Numeric bool
	Number  int
}

func printUpdateUsage() {
	fmt.Printf(`Usage: %s update [--check]

Check for and install updates.

Flags:
  --check    Only check for updates, don't download or install
`, binName)
}

func handleUpdate(args []string) error {
	checkOnly := false

	for _, arg := range args {
		switch arg {
		case "--check":
			checkOnly = true
		case "-h", "--help":
			printUpdateUsage()
			return nil
		default:
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	info("Checking for updates...\n")

	release, err := fetchLatestVersion()
	if err != nil {
		return err
	}

	result := updateResult{
		CurrentVersion:  version,
		LatestVersion:   release.Version,
		UpdateAvailable: isNewer(version, release.Version),
	}

	if !result.UpdateAvailable {
		if config.JSONMode() {
			return printJSON(result)
		}
		fmt.Printf("Already up to date (%s).\n", result.CurrentVersion)
		return nil
	}

	if checkOnly {
		if config.JSONMode() {
			return printJSON(result)
		}
		fmt.Printf("Current: %s\n", result.CurrentVersion)
		fmt.Printf("Latest:  %s\n", result.LatestVersion)
		fmt.Printf("Run `%s update` to install.\n", binName)
		return nil
	}

	if err := downloadUpdate(release); err != nil {
		return err
	}

	result.Updated = true

	if config.JSONMode() {
		return printJSON(result)
	}

	fmt.Printf("Updated %s: %s → %s\n", binName, result.CurrentVersion, result.LatestVersion)
	return nil
}

func fetchLatestVersion() (latestReleaseResponse, error) {
	release, ghErr := fetchFromGitHub()
	if ghErr == nil {
		return release, nil
	}

	release, err := fetchFromBackend()
	if err != nil {
		return latestReleaseResponse{}, errors.Join(ghErr, err)
	}
	return release, nil
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func fetchFromGitHub() (latestReleaseResponse, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		config.GitHubReleasesURL,
		nil,
	)
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to create GitHub request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := (&http.Client{
		Timeout:   releaseMetadataTimeout,
		Transport: httpx.UserAgentTransport{UserAgent: cliUserAgent()},
	}).Do(req)
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to fetch from GitHub: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccessStatus(resp.StatusCode) {
		return latestReleaseResponse{}, fmt.Errorf("GitHub update check failed: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to read GitHub response: %w", err)
	}

	var gh githubRelease
	if err := json.Unmarshal(body, &gh); err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	latest, err := validatedVersion(gh.TagName)
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("invalid GitHub release version: %w", err)
	}

	var assetURL string
	if baseName, err := currentBinaryAssetName(); err == nil {
		assetName := fmt.Sprintf("%s-v%s.zip", baseName, latest)
		for _, a := range gh.Assets {
			if a.Name == assetName {
				assetURL = a.BrowserDownloadURL
				break
			}
		}
	}

	return latestReleaseResponse{Version: latest, AssetURL: assetURL}, nil
}

func fetchFromBackend() (latestReleaseResponse, error) {
	token, err := getValidToken()
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to get auth token for update check: %w", err)
	}

	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		config.ReleasesAPIURL,
		nil,
	)
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to create update check request: %w", err)
	}
	q := req.URL.Query()
	q.Set("os", runtime.GOOS)
	q.Set("arch", runtime.GOARCH)
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := (&http.Client{
		Timeout:   releaseMetadataTimeout,
		Transport: backendTransport(),
	}).Do(req)
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccessStatus(resp.StatusCode) {
		return latestReleaseResponse{}, fmt.Errorf("update check failed: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to read latest version response: %w", err)
	}

	var payload latestReleaseResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to parse latest version response")
	}

	latest, err := validatedVersion(payload.Version)
	if err != nil {
		return latestReleaseResponse{}, fmt.Errorf("failed to parse latest version response")
	}
	payload.Version = latest
	return payload, nil
}

func isNewer(current, latest string) bool {
	if current == "dev" {
		_, err := parseVersion(latest)
		return err == nil
	}
	cmp, err := compareVersions(current, latest)
	return err == nil && cmp < 0
}

func compareVersions(current, latest string) (int, error) {
	a, err := parseVersion(current)
	if err != nil {
		return 0, err
	}
	b, err := parseVersion(latest)
	if err != nil {
		return 0, err
	}

	if a.Major != b.Major {
		return compareInts(a.Major, b.Major), nil
	}
	if a.Minor != b.Minor {
		return compareInts(a.Minor, b.Minor), nil
	}
	if a.Patch != b.Patch {
		return compareInts(a.Patch, b.Patch), nil
	}

	switch {
	case len(a.PreRelease) == 0 && len(b.PreRelease) == 0:
		return 0, nil
	case len(a.PreRelease) == 0:
		return 1, nil
	case len(b.PreRelease) == 0:
		return -1, nil
	}

	for i := 0; i < len(a.PreRelease) && i < len(b.PreRelease); i++ {
		left := a.PreRelease[i]
		right := b.PreRelease[i]

		switch {
		case left.Numeric && right.Numeric:
			if left.Number != right.Number {
				return compareInts(left.Number, right.Number), nil
			}
		case left.Numeric && !right.Numeric:
			return -1, nil
		case !left.Numeric && right.Numeric:
			return 1, nil
		default:
			if left.Raw != right.Raw {
				return compareStrings(left.Raw, right.Raw), nil
			}
		}
	}

	return compareInts(len(a.PreRelease), len(b.PreRelease)), nil
}

func parseVersion(v string) (semVersion, error) {
	v = normalizeVersion(v)
	if v == "" {
		return semVersion{}, fmt.Errorf("empty version")
	}

	parts := strings.SplitN(v, "-", 2)
	coreParts := strings.Split(parts[0], ".")
	if len(coreParts) != 3 {
		return semVersion{}, fmt.Errorf("invalid version %q", v)
	}

	major, err := strconv.Atoi(coreParts[0])
	if err != nil {
		return semVersion{}, fmt.Errorf("invalid major version %q", v)
	}
	minor, err := strconv.Atoi(coreParts[1])
	if err != nil {
		return semVersion{}, fmt.Errorf("invalid minor version %q", v)
	}
	patch, err := strconv.Atoi(coreParts[2])
	if err != nil {
		return semVersion{}, fmt.Errorf("invalid patch version %q", v)
	}

	out := semVersion{Major: major, Minor: minor, Patch: patch}
	if len(parts) == 1 {
		return out, nil
	}

	for _, ident := range strings.Split(parts[1], ".") {
		if ident == "" {
			return semVersion{}, fmt.Errorf("invalid pre-release version %q", v)
		}

		item := preReleaseIdentifier{Raw: ident}
		if n, err := strconv.Atoi(ident); err == nil {
			item.Numeric = true
			item.Number = n
		}
		out.PreRelease = append(out.PreRelease, item)
	}

	return out, nil
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func validatedVersion(raw string) (string, error) {
	v := normalizeVersion(raw)
	if _, err := parseVersion(v); err != nil {
		return "", err
	}
	return v, nil
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func downloadUpdate(release latestReleaseResponse) error {
	binaryName, err := currentBinaryAssetName()
	if err != nil {
		return err
	}
	if release.AssetURL == "" {
		return fmt.Errorf("update failed: release did not include an asset URL for %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	info("Downloading v%s (%s-%s)...\n", release.Version, runtime.GOOS, runtime.GOARCH)

	binaryData, err := downloadReleaseAsset(release.AssetURL)
	if err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}

	if release.SHA256 != "" {
		actual := sha256.Sum256(binaryData)
		if hex.EncodeToString(actual[:]) != strings.ToLower(release.SHA256) {
			return fmt.Errorf("checksum verification failed — aborting update")
		}
	} else {
		checksumBody, err := downloadReleaseAsset(release.AssetURL + ".sha256")
		if err != nil {
			return fmt.Errorf("failed to download update checksum: %w", err)
		}
		if err := verifyRawChecksum(binaryData, checksumBody); err != nil {
			return err
		}
	}
	info("Verifying checksum... ok\n")

	binary, err := extractBinary(binaryData, binaryName)
	if err != nil {
		return fmt.Errorf("failed to extract update: %w", err)
	}

	return installUpdate(binary, release.Version)
}

// extractBinary returns the executable payload from a downloaded asset. If the
// asset is a zip, the single contained file is returned; otherwise the bytes
// are returned unchanged.
func extractBinary(data []byte, binaryName string) ([]byte, error) {
	if !isZip(data) {
		return data, nil
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip asset: %w", err)
	}
	var match *zip.File
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if base == binaryName || base == binName {
			match = f
			break
		}
		if match == nil {
			match = f
		}
	}
	if match == nil {
		return nil, fmt.Errorf("zip asset is empty")
	}
	rc, err := match.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open %s in zip: %w", match.Name, err)
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, maxReleaseAssetBytes))
}

func isZip(data []byte) bool {
	return len(data) >= 4 && data[0] == 'P' && data[1] == 'K' && data[2] == 0x03 && data[3] == 0x04
}

func currentBinaryAssetName() (string, error) {
	switch runtime.GOOS {
	case "darwin", "linux":
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	switch runtime.GOARCH {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	return fmt.Sprintf("%s-%s-%s", binName, runtime.GOOS, runtime.GOARCH), nil
}

func downloadReleaseAsset(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		url,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	resp, err := (&http.Client{Timeout: releaseDownloadTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if !isSuccessStatus(resp.StatusCode) {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, maxReleaseAssetBytes))
}

func verifyRawChecksum(data []byte, checksumBody []byte) error {
	fields := strings.Fields(string(checksumBody))
	if len(fields) == 0 {
		return fmt.Errorf("malformed checksum response")
	}

	expected := strings.ToLower(fields[0])
	actual := sha256.Sum256(data)
	if hex.EncodeToString(actual[:]) != expected {
		return fmt.Errorf("checksum verification failed — aborting update")
	}

	return nil
}

func installUpdate(newBinary []byte, _ string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to locate current executable: %w", err)
	}
	return installInPlace(newBinary, exe)
}

func installInPlace(newBinary []byte, exe string) error {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, "."+binName+".update-*")
	if err != nil {
		return wrapUpdatePathError(err, dir)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(newBinary); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return wrapUpdatePathError(err, tmpPath)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return wrapUpdatePathError(err, tmpPath)
	}
	if err := os.Chmod(tmpPath, 0755); err != nil {
		_ = os.Remove(tmpPath)
		return wrapUpdatePathError(err, tmpPath)
	}
	if err := os.Rename(tmpPath, exe); err != nil {
		_ = os.Remove(tmpPath)
		return wrapUpdatePathError(err, exe)
	}
	return nil
}

func wrapUpdatePathError(err error, path string) error {
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("cannot update: permission denied on %s", path)
	}
	return fmt.Errorf("cannot update %s: %w", path, err)
}
