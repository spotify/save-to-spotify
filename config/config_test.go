package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestJSONMode(t *testing.T) {
	// Initially false
	if JSONMode() {
		t.Error("JSONMode should be false initially")
	}

	SetJSONMode()
	t.Cleanup(ResetJSONMode)

	if !JSONMode() {
		t.Error("JSONMode should be true after SetJSONMode()")
	}

	ResetJSONMode()
	if JSONMode() {
		t.Error("JSONMode should be false after ResetJSONMode()")
	}
}

func TestBackendURLPath(t *testing.T) {
	origURL := BackendBaseURL
	BackendBaseURL = "https://example.test"
	t.Cleanup(func() { BackendBaseURL = origURL })

	got, err := BackendURLPath("shows", "Show_123-~.", "episodes", "EP99", "timeline")
	if err != nil {
		t.Fatalf("BackendURLPath: %v", err)
	}

	want := "https://example.test/api/v1/shows/Show_123-~./episodes/EP99/timeline"
	if got != want {
		t.Errorf("BackendURLPath() = %q, want %q", got, want)
	}
}

func TestBackendURLPathRejectsUnsafeSegments(t *testing.T) {
	tests := []string{
		"",
		".",
		"..",
		"show#fragment",
		"show?query",
		"show/child",
		"show%2Fchild",
		"spotify:show:abc123",
	}

	for _, segment := range tests {
		t.Run(segment, func(t *testing.T) {
			_, err := BackendURLPath("shows", segment, "episodes")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "unsafe") {
				t.Errorf("error = %q, want unsafe segment error", err)
			}
		})
	}
}

func TestAPITimeout_Default(t *testing.T) {
	// Clear any env override
	t.Setenv(EnvVarTimeout, "")

	// Reset to default
	SetAPITimeout(getAPITimeout())

	d := APITimeout()
	if d != 30*time.Second {
		t.Errorf("APITimeout() = %v, want 30s", d)
	}
}

func TestAPITimeout_EnvVar(t *testing.T) {
	t.Setenv(EnvVarTimeout, "2m")

	d := getAPITimeout()
	if d != 2*time.Minute {
		t.Errorf("getAPITimeout() = %v, want 2m", d)
	}
}

func TestAPITimeout_InvalidEnvVar(t *testing.T) {
	t.Setenv(EnvVarTimeout, "not-a-duration")

	d := getAPITimeout()
	if d != 30*time.Second {
		t.Errorf("getAPITimeout() = %v, want 30s (fallback)", d)
	}
}

func TestScopes(t *testing.T) {
	if Scopes != "sts-content-management" {
		t.Errorf("Scopes = %q, want sts-content-management", Scopes)
	}
	for _, disallowed := range []string{"user-library-read", "user-library-modify"} {
		if strings.Contains(Scopes, disallowed) {
			t.Errorf("Scopes should not include %q", disallowed)
		}
	}
}

func TestDPoPKey_SaveLoadDelete(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Load when no file exists — returns nil, nil
	data, err := LoadDPoPKey()
	if err != nil {
		t.Fatalf("LoadDPoPKey (missing): %v", err)
	}
	if data != nil {
		t.Error("LoadDPoPKey should return nil when file does not exist")
	}

	// Save
	keyData := []byte(`{"kty":"EC","crv":"P-256","x":"test","y":"test","d":"test"}`)
	if err := SaveDPoPKey(keyData); err != nil {
		t.Fatalf("SaveDPoPKey: %v", err)
	}

	// Verify file permissions
	path, _ := DPoPKeyPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat dpop key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	// Load back
	loaded, err := LoadDPoPKey()
	if err != nil {
		t.Fatalf("LoadDPoPKey: %v", err)
	}
	if string(loaded) != string(keyData) {
		t.Errorf("loaded data = %q, want %q", loaded, keyData)
	}

	// Delete
	if err := DeleteDPoPKey(); err != nil {
		t.Fatalf("DeleteDPoPKey: %v", err)
	}

	// Verify gone
	data, err = LoadDPoPKey()
	if err != nil {
		t.Fatalf("LoadDPoPKey (after delete): %v", err)
	}
	if data != nil {
		t.Error("LoadDPoPKey should return nil after delete")
	}

	// Delete again is a no-op
	if err := DeleteDPoPKey(); err != nil {
		t.Errorf("DeleteDPoPKey (idempotent): %v", err)
	}
}

func TestDPoPKeyPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := DPoPKeyPath()
	if err != nil {
		t.Fatalf("DPoPKeyPath: %v", err)
	}
	want := filepath.Join(tmp, "save-to-spotify", "dpop_key.json")
	if path != want {
		t.Errorf("DPoPKeyPath = %q, want %q", path, want)
	}
}

func setHeadersEnv(t *testing.T, val string) {
	t.Helper()
	t.Setenv(EnvVarHeaders, val)
	orig := additionalHeaders
	additionalHeaders = parseAdditionalHeaders()
	t.Cleanup(func() { additionalHeaders = orig })
}

func TestParseAdditionalHeaders(t *testing.T) {
	setHeadersEnv(t, `{"X-STS-Test":"true","X-STS-Foo":"bar"}`)

	if got := AdditionalHeaders()["X-Sts-Test"]; got != "true" {
		t.Errorf("X-Sts-Test = %q, want %q", got, "true")
	}
	if got := AdditionalHeaders()["X-Sts-Foo"]; got != "bar" {
		t.Errorf("X-Sts-Foo = %q, want %q", got, "bar")
	}
}

func TestParseAdditionalHeaders_Empty(t *testing.T) {
	setHeadersEnv(t, "")

	if AdditionalHeaders() != nil {
		t.Errorf("expected nil, got %v", AdditionalHeaders())
	}
}

func TestParseAdditionalHeaders_InvalidJSON(t *testing.T) {
	setHeadersEnv(t, "not json")

	if AdditionalHeaders() != nil {
		t.Errorf("expected nil for invalid JSON, got %v", AdditionalHeaders())
	}
}

func TestParseAdditionalHeaders_RejectsNonSTS(t *testing.T) {
	setHeadersEnv(t, `{"X-STS-Test":"true","X-Custom":"bad","Authorization":"evil"}`)

	if got := AdditionalHeaders()["X-Sts-Test"]; got != "true" {
		t.Errorf("X-Sts-Test = %q, want %q", got, "true")
	}
	if len(AdditionalHeaders()) != 1 {
		t.Errorf("expected 1 header, got %d: %v", len(AdditionalHeaders()), AdditionalHeaders())
	}
}

func TestParseAdditionalHeaders_CanonicalizesAnyCasing(t *testing.T) {
	setHeadersEnv(t, `{"x-sts-trace-id":"123"}`)

	if got := AdditionalHeaders()["X-Sts-Trace-Id"]; got != "123" {
		t.Errorf("X-Sts-Trace-Id = %q, want %q", got, "123")
	}
}

func TestParseAdditionalHeaders_CaseCollisionIsDeterministic(t *testing.T) {
	setHeadersEnv(t, `{"X-STS-TEST":"upper","X-STS-Test":"mixed"}`)

	if len(AdditionalHeaders()) != 1 {
		t.Fatalf("expected 1 header, got %v", AdditionalHeaders())
	}
	// Keys are filtered in sorted order, so the last sorted spelling wins.
	if got := AdditionalHeaders()["X-Sts-Test"]; got != "mixed" {
		t.Errorf("X-Sts-Test = %q, want %q", got, "mixed")
	}
}

func TestParseAdditionalHeaders_RejectsInvalidName(t *testing.T) {
	setHeadersEnv(t, `{"X-Sts-Trace Id":"abc"}`)

	if AdditionalHeaders() != nil {
		t.Errorf("expected nil for invalid header name, got %v", AdditionalHeaders())
	}
}

func TestParseAdditionalHeaders_RejectsInvalidValue(t *testing.T) {
	setHeadersEnv(t, `{"X-STS-Test":"a\nb"}`)

	if AdditionalHeaders() != nil {
		t.Errorf("expected nil for invalid header value, got %v", AdditionalHeaders())
	}
}

func TestSetAdditionalHeaders_AppliesFiltering(t *testing.T) {
	orig := additionalHeaders
	t.Cleanup(func() { additionalHeaders = orig })

	SetAdditionalHeaders(map[string]string{
		"x-sts-test":    "1",
		"Authorization": "evil",
	})

	if got := AdditionalHeaders()["X-Sts-Test"]; got != "1" {
		t.Errorf("X-Sts-Test = %q, want %q", got, "1")
	}
	if len(AdditionalHeaders()) != 1 {
		t.Errorf("expected 1 header, got %v", AdditionalHeaders())
	}
}
