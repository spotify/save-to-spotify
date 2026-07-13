package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

func TestStartUpdateCheckNewVersion(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	newGitHubReleaseServer(t, "v9.9.9", nil)

	stderr := captureStderr(t, func() {
		StartUpdateCheck()()
	})
	if !strings.Contains(stderr, "A newer version of "+binName+" is available: 9.9.9") {
		t.Fatalf("stderr = %q, want update notice", stderr)
	}
}

func TestStartUpdateCheckCurrent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	newGitHubReleaseServer(t, "v"+version, nil)

	stderr := captureStderr(t, func() {
		StartUpdateCheck()()
	})
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty output", stderr)
	}
}

func TestStartUpdateCheckCached(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := saveUpdateCheckCache(&updateCheckCache{
		LatestVersion: "9.9.9",
		CheckedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("saveUpdateCheckCache: %v", err)
	}

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		json.NewEncoder(w).Encode(githubRelease{TagName: "v9.9.9"})
	}))
	defer server.Close()

	overrideURL(t, &config.GitHubReleasesURL, server.URL)

	captureStderr(t, func() {
		StartUpdateCheck()()
	})

	if hits.Load() != 0 {
		t.Fatalf("HTTP hits = %d, want 0", hits.Load())
	}
}

func TestStartUpdateCheckStaleCache(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if err := saveUpdateCheckCache(&updateCheckCache{
		LatestVersion: "9.9.8",
		CheckedAt:     time.Now().Add(-48 * time.Hour).UTC(),
	}); err != nil {
		t.Fatalf("saveUpdateCheckCache: %v", err)
	}

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		json.NewEncoder(w).Encode(githubRelease{TagName: "v9.9.9"})
	}))
	defer server.Close()

	overrideURL(t, &config.GitHubReleasesURL, server.URL)

	captureStderr(t, func() {
		StartUpdateCheck()()
	})

	if hits.Load() == 0 {
		t.Fatal("expected stale cache to trigger HTTP request")
	}
}

func TestStartUpdateCheckDisabled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarNoUpdateCheck, "1")

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		json.NewEncoder(w).Encode(githubRelease{TagName: "v9.9.9"})
	}))
	defer server.Close()

	overrideURL(t, &config.GitHubReleasesURL, server.URL)

	stderr := captureStderr(t, func() {
		StartUpdateCheck()()
	})

	if hits.Load() != 0 {
		t.Fatalf("HTTP hits = %d, want 0", hits.Load())
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty output", stderr)
	}
}

func TestStartUpdateCheckTimeout(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	oldWait := updateNoticeWaitTimeout
	updateNoticeWaitTimeout = 20 * time.Millisecond
	defer func() { updateNoticeWaitTimeout = oldWait }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(githubRelease{TagName: "v9.9.9"})
	}))
	defer server.Close()

	overrideURL(t, &config.GitHubReleasesURL, server.URL)

	stderr := captureStderr(t, func() {
		StartUpdateCheck()()
	})

	if stderr != "" {
		t.Fatalf("stderr = %q, want empty output", stderr)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	fn()

	w.Close()
	os.Stderr = oldStderr

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll(stderr): %v", err)
	}
	return string(data)
}
