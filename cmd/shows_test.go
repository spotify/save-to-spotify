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
		Title:           "My Show",
		Summary:         "Description",
		Language:        "en",
		ImageToken:      "tok_abc123",
		PlaybackControl: "PLAYBACK_CONTROL_CHAPTER_SKIP",
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
	expectedKeys := []string{"title", "summary", "language", "image_token", "playback_control"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	// image_token and playback_control should be omitted when empty
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
	if _, ok := raw2["playback_control"]; ok {
		t.Error("playback_control should be omitted when empty")
	}
}

func TestParseShowCreateFlags_PlaybackControl(t *testing.T) {
	f, err := parseShowCreateFlags([]string{
		"--title", "Show",
		"--playback-control", "chapter-skip",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.playbackControl != "chapter-skip" {
		t.Errorf("playbackControl = %q, want %q", f.playbackControl, "chapter-skip")
	}
	if f.playbackControlAPI != "PLAYBACK_CONTROL_CHAPTER_SKIP" {
		t.Errorf("playbackControlAPI = %q, want %q", f.playbackControlAPI, "PLAYBACK_CONTROL_CHAPTER_SKIP")
	}

	if _, err := parseShowCreateFlags([]string{
		"--title", "Show",
		"--playback-control", "bogus",
	}); err == nil {
		t.Fatal("expected error for unknown playback control mode")
	}

	if _, err := parseShowCreateFlags([]string{
		"--title", "Show",
		"--playback-control",
	}); err == nil {
		t.Fatal("expected error for missing --playback-control value")
	}
}

// setupShowsTest saves a valid token under a temp config dir and points the
// backend at a mock server running handler. Cleanup is automatic.
func setupShowsTest(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

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
}

func TestHandleShowsCreate_Integration(t *testing.T) {
	setupShowsTest(t, func(w http.ResponseWriter, r *http.Request) {
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

		var body showCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, "bad request", 400)
			return
		}
		if body.Title != "Test Show" {
			t.Errorf("title = %q", body.Title)
		}
		if body.PlaybackControl != "" {
			t.Errorf("playback_control = %q, want empty when flag is unset", body.PlaybackControl)
		}

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(showCreateResponse{
			ShowURI: "spotify:show:abc123",
		})
	})

	err := handleShowsCreate([]string{
		"--title", "Test Show",
		"--summary", "A test show",
	})
	if err != nil {
		t.Fatalf("handleShowsCreate: %v", err)
	}
}

func TestHandleShowsCreate_PrototypeDefaults(t *testing.T) {
	setupShowsTest(t, func(w http.ResponseWriter, r *http.Request) {
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
	})

	// Only pass --title -- everything else should be defaulted
	if err := handleShowsCreate([]string{"--title", "Minimal Show"}); err != nil {
		t.Fatalf("handleShowsCreate: %v", err)
	}
}

func TestHandleShowsCreate_JSONOutput(t *testing.T) {
	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	setupShowsTest(t, func(w http.ResponseWriter, r *http.Request) {
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
		if body.PlaybackControl != "PLAYBACK_CONTROL_CHAPTER_SKIP" {
			t.Errorf("playback_control = %q, want %q", body.PlaybackControl, "PLAYBACK_CONTROL_CHAPTER_SKIP")
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(showCreateResponse{
			ShowURI:         "spotify:show:json123",
			PlaybackControl: "PLAYBACK_CONTROL_CHAPTER_SKIP",
		})
	})

	output := captureOutput(t, func() error {
		return handleShowsCreate([]string{
			"--title", "JSON Show",
			"--summary", "A test",
			"--playback-control", "chapter-skip",
		})
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	if parsed["show_uri"] != "spotify:show:json123" {
		t.Errorf("show_uri = %v", parsed["show_uri"])
	}
	if parsed["playback_control"] != "chapter-skip" {
		t.Errorf("playback_control = %v, want %q", parsed["playback_control"], "chapter-skip")
	}
}

// The backend silently drops playback control values it doesn't recognize:
// the create still succeeds, and the missing echo in the response is the only
// failure signal. The CLI must not fabricate the field in that case.
func TestHandleShowsCreate_PlaybackControlNotConfirmed(t *testing.T) {
	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	setupShowsTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(showCreateResponse{
			ShowURI: "spotify:show:drop123",
		})
	})

	output := captureOutput(t, func() error {
		return handleShowsCreate([]string{
			"--title", "Dropped Show",
			"--playback-control", "chapter-skip",
		})
	})

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if parsed["show_uri"] != "spotify:show:drop123" {
		t.Errorf("show_uri = %v", parsed["show_uri"])
	}
	if _, ok := parsed["playback_control"]; ok {
		t.Errorf("playback_control = %v, must be absent when the server did not confirm it", parsed["playback_control"])
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
