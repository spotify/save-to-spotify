package cmd

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spotify/save-to-spotify/config"
)

func TestFetchLatestVersion(t *testing.T) {
	newGitHubReleaseServer(t, "v0.2.0", nil)

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if got.Version != "0.2.0" {
		t.Fatalf("fetchLatestVersion version = %q, want %q", got.Version, "0.2.0")
	}
}

func TestFetchLatestVersionPreRelease(t *testing.T) {
	newGitHubReleaseServer(t, "v0.2.0-beta.1", nil)

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if got.Version != "0.2.0-beta.1" {
		t.Fatalf("fetchLatestVersion version = %q, want %q", got.Version, "0.2.0-beta.1")
	}
}

func TestFetchLatestVersionNetworkError(t *testing.T) {
	overrideURL(t, &config.GitHubReleasesURL, "http://127.0.0.1:1")

	t.Setenv(config.EnvVarAuthToken, "test-token")
	overrideURL(t, &config.ReleasesAPIURL, "http://127.0.0.1:1")

	if _, err := fetchLatestVersion(); err == nil {
		t.Fatal("fetchLatestVersion: expected error")
	}
}

func TestFetchLatestVersionSendsOSArchToBackend(t *testing.T) {
	// GitHub is primary, but if it fails, the backend fallback should send os/arch.
	overrideURL(t, &config.GitHubReleasesURL, "http://127.0.0.1:1")

	var gotOS, gotArch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOS = r.URL.Query().Get("os")
		gotArch = r.URL.Query().Get("arch")
		json.NewEncoder(w).Encode(latestReleaseResponse{Version: "0.2.0"})
	}))
	defer server.Close()

	t.Setenv(config.EnvVarAuthToken, "test-token")
	overrideURL(t, &config.ReleasesAPIURL, server.URL)

	if _, err := fetchLatestVersion(); err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if gotOS == "" {
		t.Fatal("expected ?os= query param, got empty")
	}
	if gotArch == "" {
		t.Fatal("expected ?arch= query param, got empty")
	}
}

func TestFetchLatestVersionReturnsAssetURL(t *testing.T) {
	baseName, err := currentBinaryAssetName()
	if err != nil {
		t.Fatalf("currentBinaryAssetName: %v", err)
	}
	wantURL := "https://github.com/dl/" + baseName + ".zip"
	assets := []githubAsset{
		{Name: "save-to-spotify-otheros-otherarch-v0.2.0.zip", BrowserDownloadURL: "https://github.com/dl/other.zip"},
		{Name: baseName + "-v0.2.0.zip", BrowserDownloadURL: wantURL},
	}
	newGitHubReleaseServer(t, "v0.2.0", assets)

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if got.AssetURL != wantURL {
		t.Fatalf("AssetURL = %q, want %q", got.AssetURL, wantURL)
	}
}

func TestFetchLatestVersionFallsBackToBackend(t *testing.T) {
	overrideURL(t, &config.GitHubReleasesURL, "http://127.0.0.1:1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(latestReleaseResponse{
			Version:  "0.2.0",
			AssetURL: "https://example.com/binary",
			SHA256:   "abc123",
		})
	}))
	defer server.Close()

	t.Setenv(config.EnvVarAuthToken, "test-token")
	overrideURL(t, &config.ReleasesAPIURL, server.URL)

	got, err := fetchLatestVersion()
	if err != nil {
		t.Fatalf("fetchLatestVersion: %v", err)
	}
	if got.Version != "0.2.0" {
		t.Fatalf("Version = %q, want %q", got.Version, "0.2.0")
	}
	if got.AssetURL != "https://example.com/binary" {
		t.Fatalf("AssetURL = %q, want %q", got.AssetURL, "https://example.com/binary")
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{current: "0.1.0", latest: "0.2.0", expected: true},
		{current: "0.2.0", latest: "0.2.0", expected: false},
		{current: "0.3.0", latest: "0.2.0", expected: false},
		{current: "0.1.0-alpha.8", latest: "0.1.0-alpha.9", expected: true},
		{current: "0.1.0-alpha.8", latest: "0.1.0", expected: true},
		{current: "0.1.0", latest: "0.1.0-alpha.8", expected: false},
		{current: "1.0.0", latest: "0.99.99", expected: false},
		{current: "0.1.0-alpha.2", latest: "0.1.0-beta.1", expected: true},
	}

	for _, tc := range tests {
		if got := isNewer(tc.current, tc.latest); got != tc.expected {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.expected)
		}
	}
}

func TestVerifyRawChecksum(t *testing.T) {
	data := []byte("release binary")
	sum := sha256.Sum256(data)

	if err := verifyRawChecksum(data, []byte(hex.EncodeToString(sum[:])+"\n")); err != nil {
		t.Fatalf("verifyRawChecksum: %v", err)
	}
}

func TestVerifyRawChecksumMalformed(t *testing.T) {
	if err := verifyRawChecksum([]byte("release binary"), []byte(" \n")); err == nil {
		t.Fatal("verifyRawChecksum: expected malformed checksum error")
	}
}

func TestInstallInPlace(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, binName)
	if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := installInPlace([]byte("binary-v1"), exe); err != nil {
		t.Fatalf("installInPlace: %v", err)
	}

	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", exe, err)
	}
	if string(data) != "binary-v1" {
		t.Fatalf("binary contents = %q, want %q", string(data), "binary-v1")
	}

	info, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("Stat(%s): %v", exe, err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("mode = %v, want 0755", info.Mode().Perm())
	}

	linkInfo, err := os.Lstat(exe)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", exe, err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("installed binary is a symlink, want regular file")
	}
}

func TestInstallInPlaceLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, binName)
	if err := os.WriteFile(exe, []byte("old"), 0755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := installInPlace([]byte("binary-v1"), exe); err != nil {
		t.Fatalf("installInPlace: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != binName {
		t.Fatalf("dir entries = %v, want [%s]", names, binName)
	}
}

func TestDownloadUpdateUsesBackendAssetURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	binaryData := []byte("backend-binary-content")
	sum := sha256.Sum256(binaryData)
	expectedSHA := hex.EncodeToString(sum[:])

	var assetHits, checksumsHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			assetHits++
			w.Write(binaryData)
		case "/checksums.txt":
			checksumsHits++
			w.Write([]byte("deadbeef  some-file\n"))
		}
	}))
	defer server.Close()

	release := latestReleaseResponse{
		Version:  "0.9.0",
		AssetURL: server.URL + "/binary",
		SHA256:   expectedSHA,
	}

	if err := downloadUpdate(release); err != nil {
		t.Fatalf("downloadUpdate: %v", err)
	}
	if assetHits != 1 {
		t.Fatalf("asset server hits = %d, want 1", assetHits)
	}
	if checksumsHits != 0 {
		t.Fatalf("checksums.txt fetched %d times, want 0 when sha256 provided", checksumsHits)
	}
}

func TestDownloadUpdateUsesAssetURLSidecarChecksum(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	binaryData := []byte("backend-binary-content")
	sum := sha256.Sum256(binaryData)
	expectedSHA := hex.EncodeToString(sum[:])

	var assetHits, sidecarHits, checksumsHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/binary":
			assetHits++
			w.Write(binaryData)
		case "/binary.sha256":
			sidecarHits++
			w.Write([]byte(expectedSHA + "\n"))
		case "/checksums.txt":
			checksumsHits++
			w.Write([]byte("deadbeef  some-file\n"))
		}
	}))
	defer server.Close()

	release := latestReleaseResponse{
		Version:  "0.9.0",
		AssetURL: server.URL + "/binary",
	}

	if err := downloadUpdate(release); err != nil {
		t.Fatalf("downloadUpdate: %v", err)
	}
	if assetHits != 1 {
		t.Fatalf("asset server hits = %d, want 1", assetHits)
	}
	if sidecarHits != 1 {
		t.Fatalf("asset sidecar hits = %d, want 1", sidecarHits)
	}
	if checksumsHits != 0 {
		t.Fatalf("checksums.txt fetched %d times, want 0 when sidecar checksum is used", checksumsHits)
	}
}

func TestExtractBinaryExtractsZipAsset(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if w, err := zw.Create("notes.txt"); err != nil {
		t.Fatalf("Create(notes.txt): %v", err)
	} else if _, err := w.Write([]byte("not the binary")); err != nil {
		t.Fatalf("Write(notes.txt): %v", err)
	}
	if w, err := zw.Create("nested/save-to-spotify-darwin-arm64"); err != nil {
		t.Fatalf("Create(binary): %v", err)
	} else if _, err := w.Write([]byte("binary-payload")); err != nil {
		t.Fatalf("Write(binary): %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}

	got, err := extractBinary(buf.Bytes(), "save-to-spotify-darwin-arm64")
	if err != nil {
		t.Fatalf("extractBinary: %v", err)
	}
	if string(got) != "binary-payload" {
		t.Fatalf("extracted payload = %q, want %q", string(got), "binary-payload")
	}
}

func TestDownloadUpdateRequiresAssetURL(t *testing.T) {
	err := downloadUpdate(latestReleaseResponse{Version: "0.9.1"})
	if err == nil || !strings.Contains(err.Error(), "asset URL") {
		t.Fatalf("downloadUpdate err = %v, want asset URL error", err)
	}
}

func TestHandleUpdateAlreadyCurrent(t *testing.T) {
	orig := version
	version = "1.0.0"
	defer func() { version = orig }()

	newGitHubReleaseServer(t, "v"+version, nil)

	output := captureOutput(t, func() error {
		return handleUpdate(nil)
	})
	if !strings.Contains(output, "Already up to date") {
		t.Fatalf("stdout = %q, want up-to-date message", output)
	}
}

func TestHandleUpdateCheckOnly(t *testing.T) {
	newGitHubReleaseServer(t, "v9.9.9", nil)

	output := captureOutput(t, func() error {
		return handleUpdate([]string{"--check"})
	})
	if !strings.Contains(output, "Current: "+version) {
		t.Fatalf("stdout = %q, want current version", output)
	}
	if !strings.Contains(output, "Latest:  9.9.9") {
		t.Fatalf("stdout = %q, want latest version", output)
	}
}

func TestDoAPIRequest_MinCLIVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(minCLIVersionHeader, "9.9.9")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}

	token := &config.TokenData{AccessToken: "test-token"}
	if _, err := doAPIRequest(req, token); err == nil {
		t.Fatal("doAPIRequest: expected minimum version error")
	} else if !strings.Contains(err.Error(), "no longer supported") {
		t.Fatalf("doAPIRequest error = %q, want unsupported version message", err)
	}
}

// newGitHubReleaseServer serves a GitHub latest-release response and points
// config.GitHubReleasesURL at it for the duration of the test.
func newGitHubReleaseServer(t *testing.T, tagName string, assets []githubAsset) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubRelease{TagName: tagName, Assets: assets})
	}))
	t.Cleanup(server.Close)
	overrideURL(t, &config.GitHubReleasesURL, server.URL)
}

// overrideURL points a config URL variable at url and restores it when the
// test ends.
func overrideURL(t *testing.T, target *string, url string) {
	t.Helper()
	orig := *target
	*target = url
	t.Cleanup(func() { *target = orig })
}

func captureOutput(t *testing.T, fn func() error) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = oldStdout })

	if err := fn(); err != nil {
		w.Close()
		os.Stdout = oldStdout
		t.Fatalf("fn: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll(stdout): %v", err)
	}
	return string(data)
}
