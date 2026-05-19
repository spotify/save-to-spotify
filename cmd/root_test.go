package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

func TestGetValidToken_EnvVarOverride(t *testing.T) {
	// No token file — env var should be sufficient.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "env-token-123")

	token, err := getValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "env-token-123" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "env-token-123")
	}
	if token.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", token.TokenType, "Bearer")
	}
	if token.RefreshToken != "" {
		t.Errorf("RefreshToken = %q, want empty", token.RefreshToken)
	}
}

func TestGetValidToken_EnvVarTakesPrecedence(t *testing.T) {
	// File-based token exists, but env var should win.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	fileToken := &config.TokenData{
		AccessToken:  "file-token",
		RefreshToken: "file-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(fileToken); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	t.Setenv(config.EnvVarAuthToken, "env-token-wins")

	token, err := getValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "env-token-wins" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "env-token-wins")
	}
}

func TestHandleToken_JSONOutput(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "my-test-token")

	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleToken()

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleToken: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if parsed["access_token"] != "my-test-token" {
		t.Errorf("access_token = %v, want %q", parsed["access_token"], "my-test-token")
	}
}

func TestGetValidToken_FallsBackToFile(t *testing.T) {
	// Env var unset — should fall back to file-based token.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv(config.EnvVarAuthToken, "")

	fileToken := &config.TokenData{
		AccessToken:  "file-token",
		RefreshToken: "file-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(fileToken); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	token, err := getValidToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token.AccessToken != "file-token" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "file-token")
	}
}

func TestCLIUserAgent(t *testing.T) {
	oldCommit := commit
	t.Cleanup(func() { commit = oldCommit })

	commit = "abc123"
	want := fmt.Sprintf("%s/%s+abc123 %s/%s", binName, version, runtime.GOOS, runtime.GOARCH)
	if got := cliUserAgent(); got != want {
		t.Fatalf("cliUserAgent() = %q, want %q", got, want)
	}

	commit = "unknown"
	want = fmt.Sprintf("%s/%s %s/%s", binName, version, runtime.GOOS, runtime.GOARCH)
	if got := cliUserAgent(); got != want {
		t.Fatalf("cliUserAgent() = %q, want %q", got, want)
	}
}

func TestShouldStartUpdateCheck(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "regular command", args: []string{"upload"}, want: true},
		{name: "help command", args: []string{"help"}, want: false},
		{name: "update command", args: []string{"update"}, want: false},
		{name: "no args", args: []string{}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStartUpdateCheck(tt.args); got != tt.want {
				t.Fatalf("shouldStartUpdateCheck(%v) = %t, want %t", tt.args, got, tt.want)
			}
		})
	}
}

func TestShouldStartUpdateCheckDisabledByEnv(t *testing.T) {
	t.Setenv(config.EnvVarNoUpdateCheck, "1")

	if got := shouldStartUpdateCheck([]string{"upload"}); got {
		t.Fatal("shouldStartUpdateCheck should be false when disabled by env var")
	}
}

func TestPluginHintEmittedWhenCLAUDECODE(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	os.Args = []string{"save-to-spotify", "version"}
	_ = Execute()

	w.Close()
	os.Stderr = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	want := `<claude-code-hint v="1" type="plugin" value="save-to-spotify@claude-plugins-official" />`
	if !containsLine(output, want) {
		t.Errorf("stderr should contain plugin hint line\ngot: %s", output)
	}
}

func TestPluginHintNotEmittedWithoutCLAUDECODE(t *testing.T) {
	t.Setenv("CLAUDECODE", "")

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	os.Args = []string{"save-to-spotify", "version"}
	_ = Execute()

	w.Close()
	os.Stderr = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if strings.Contains(output, "claude-code-hint") {
		t.Errorf("stderr should not contain plugin hint when CLAUDECODE is unset\ngot: %s", output)
	}
}

func containsLine(s, substr string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == substr {
			return true
		}
	}
	return false
}
