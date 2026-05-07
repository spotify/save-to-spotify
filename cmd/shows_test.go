package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

func TestParseShowCreateFlags_AllExplicit(t *testing.T) {
	args := []string{
		"--title", "My Show",
		"--summary", "A great show",
		"--image", "cover.jpg",
		"--language", "en",
	}

	f, err := parseShowCreateFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.title != "My Show" {
		t.Errorf("title = %q, want %q", f.title, "My Show")
	}
	if f.summary != "A great show" {
		t.Errorf("summary = %q, want %q", f.summary, "A great show")
	}
	if f.image != "cover.jpg" {
		t.Errorf("image = %q, want %q", f.image, "cover.jpg")
	}
	if f.language != "en" {
		t.Errorf("language = %q, want %q", f.language, "en")
	}
}

func TestParseShowCreateFlags_TitleOnly(t *testing.T) {
	f, err := parseShowCreateFlags([]string{"--title", "Only Title"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.title != "Only Title" {
		t.Errorf("title = %q, want %q", f.title, "Only Title")
	}
	if f.summary != "(no description)" {
		t.Errorf("summary = %q, want %q", f.summary, "(no description)")
	}
	if f.image != "" {
		t.Errorf("image = %q, want empty", f.image)
	}
	if f.language != "en" {
		t.Errorf("language = %q, want %q", f.language, "en")
	}
}

func TestParseShowCreateFlags_MissingTitle(t *testing.T) {
	_, err := parseShowCreateFlags([]string{"--summary", "Desc"})
	if err == nil {
		t.Fatal("expected error for missing --title")
	}
	if !strings.Contains(err.Error(), "--title") {
		t.Errorf("error = %q, should mention --title", err)
	}
}

func TestParseShowCreateFlags_OptionalOverrides(t *testing.T) {
	args := []string{
		"--title", "Show",
		"--language", "es",
	}

	f, err := parseShowCreateFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.language != "es" {
		t.Errorf("language = %q, want %q", f.language, "es")
	}
}

func TestParseShowCreateFlags_UnknownFlag(t *testing.T) {
	args := []string{
		"--title", "Show",
		"--unknown", "value",
	}
	_, err := parseShowCreateFlags(args)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestShowCreateRequestJSON(t *testing.T) {
	req := showCreateRequest{
		Title:      "My Show",
		Summary:    "Description",
		Language:   "en",
		ImageToken: "tok_abc123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify JSON keys match API spec
	expectedKeys := []string{"title", "summary", "language", "image_token"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	// image_token should be omitted when empty
	req2 := showCreateRequest{
		Title:    "No Image",
		Summary:  "Desc",
		Language: "en",
	}
	data2, _ := json.Marshal(req2)
	var raw2 map[string]interface{}
	json.Unmarshal(data2, &raw2)
	if _, ok := raw2["image_token"]; ok {
		t.Error("image_token should be omitted when empty")
	}
}

func TestHandleShowsCreate_Integration(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Mock backend API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shows" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		if r.Method != "POST" {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", 405)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("X-Feature-Flags") != "" {
			t.Errorf("X-Feature-Flags should not be set, got %q", r.Header.Get("X-Feature-Flags"))
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		// Parse request body
		var body showCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, "bad request", 400)
			return
		}
		if body.Title != "Test Show" {
			t.Errorf("title = %q", body.Title)
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(showCreateResponse{
			ShowURI: "spotify:show:abc123",
		})
	}))
	defer server.Close()

	// Save a valid token
	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		Scopes:       "user-read-private",
	}
	token.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	err := handleShowsCreate([]string{
		"--title", "Test Show",
		"--summary", "A test show",
	})
	if err != nil {
		t.Fatalf("handleShowsCreate: %v", err)
	}
}

func TestHandleShowsCreate_PrototypeDefaults(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shows" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}

		var body showCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, "bad request", 400)
			return
		}
		if body.Summary != "(no description)" {
			t.Errorf("summary = %q, want %q", body.Summary, "(no description)")
		}
		if body.ImageToken != "" {
			t.Errorf("image_token = %q, want empty", body.ImageToken)
		}
		if body.Language != "en" {
			t.Errorf("language = %q, want %q", body.Language, "en")
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(showCreateResponse{
			ShowURI: "spotify:show:def456",
		})
	}))
	defer server.Close()

	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		Scopes:       "user-read-private",
	}
	token.ExpiresAt = time.Now().Add(24 * time.Hour)
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	// Only pass --title -- everything else should be defaulted
	err := handleShowsCreate([]string{"--title", "Minimal Show"})
	if err != nil {
		t.Fatalf("handleShowsCreate: %v", err)
	}
}

func TestHandleShowsCreate_JSONOutput(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shows" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", 404)
			return
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(showCreateResponse{
			ShowURI: "spotify:show:json123",
		})
	}))
	defer server.Close()

	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleShowsCreate([]string{
		"--title", "JSON Show",
		"--summary", "A test",
	})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleShowsCreate: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	if parsed["show_uri"] != "spotify:show:json123" {
		t.Errorf("show_uri = %v", parsed["show_uri"])
	}
}

func TestHandleShowsGet_UsesBackend(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shows/abc123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"show_uri":"spotify:show:abc123","title":"Backend Show","summary":"From backend","language":"en","publisher":"Spotify","total_episodes":7,"explicit":false}`)
	}))
	defer server.Close()

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleShowsGet("spotify:show:abc123")

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleShowsGet: %v", err)
	}

	buf, _ := io.ReadAll(r)
	output := string(buf)

	for _, want := range []string{
		"Show: Backend Show",
		"ID:          abc123",
		"Publisher:   Spotify",
		"Language:    en",
		"Episodes:    7",
		"Description: From backend",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestHandleShowsDelete_SendsUserAgent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shows/abc123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
		Scopes:       "user-read-private",
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	if err := handleShowsDelete("spotify:show:abc123"); err != nil {
		t.Fatalf("handleShowsDelete: %v", err)
	}
}

func TestHandleShows_Routing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	err := handleShows([]string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}

	err = handleShows([]string{"get"})
	if err == nil {
		t.Error("expected error for get without ID")
	}
}
