package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

func TestParseUploadFlags_PositionalFileAndTitle(t *testing.T) {
	f, err := parseUploadFlags([]string{"./episode.mp3", "--title", "Episode 1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.filePath != "./episode.mp3" {
		t.Errorf("filePath = %q, want %q", f.filePath, "./episode.mp3")
	}
	if f.title != "Episode 1" {
		t.Errorf("title = %q, want %q", f.title, "Episode 1")
	}
	if f.newShow != "" {
		t.Errorf("newShow = %q, want empty", f.newShow)
	}
}

func TestParseUploadFlags_AllFlags(t *testing.T) {
	f, err := parseUploadFlags([]string{
		"/tmp/audio.mp3",
		"--title", "My Episode",
		"--show-id", "spotify:show:abc123",
		"--summary", "A description",
		"--image", "https://example.com/img.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.filePath != "/tmp/audio.mp3" {
		t.Errorf("filePath = %q", f.filePath)
	}
	if f.title != "My Episode" {
		t.Errorf("title = %q", f.title)
	}
	if f.showID != "spotify:show:abc123" {
		t.Errorf("showID = %q", f.showID)
	}
	if f.summary != "A description" {
		t.Errorf("summary = %q", f.summary)
	}
	if f.image != "https://example.com/img.jpg" {
		t.Errorf("image = %q", f.image)
	}
}

func TestParseUploadFlags_NewShowAndShowIDMutuallyExclusive(t *testing.T) {
	_, err := parseUploadFlags([]string{
		"./episode.mp3",
		"--title", "Ep",
		"--new-show", "Custom Show",
		"--show-id", "spotify:show:abc123",
	})
	if err == nil {
		t.Fatal("expected error when both --new-show and --show-id are set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, should mention mutually exclusive", err)
	}
}

func TestParseUploadFlags_MissingFile(t *testing.T) {
	_, err := parseUploadFlags(nil)
	if err == nil {
		t.Fatal("expected error for missing file arg")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error = %q, should contain usage", err)
	}
}

func TestParseUploadFlags_MissingTitle(t *testing.T) {
	_, err := parseUploadFlags([]string{"./episode.mp3"})
	if err == nil {
		t.Fatal("expected error for missing --title")
	}
	if !strings.Contains(err.Error(), "--title") {
		t.Errorf("error = %q, should mention --title", err)
	}
}

func TestParseUploadFlags_FlagAsFirstArg(t *testing.T) {
	_, err := parseUploadFlags([]string{"--title", "Episode"})
	if err == nil {
		t.Fatal("expected error when first arg is a flag")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("error = %q, should contain usage", err)
	}
}

func TestParseUploadFlags_NewShowFlag(t *testing.T) {
	f, err := parseUploadFlags([]string{"./episode.mp3", "--title", "Ep", "--new-show", "Brand New Show"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.newShow != "Brand New Show" {
		t.Errorf("newShow = %q, want %q", f.newShow, "Brand New Show")
	}
}

func TestParseUploadFlags_UnknownFlag(t *testing.T) {
	_, err := parseUploadFlags([]string{"./episode.mp3", "--title", "Ep", "--unknown", "val"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
}

func TestResolveOrCreateShow_ExplicitShowID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Mock server for the listShows call that resolveOrCreateShow makes to find the title
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(listShowsResponse{Shows: nil})
	}))
	defer server.Close()

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	flags := &uploadFlags{showID: "spotify:show:explicit123"}
	token := &config.TokenData{AccessToken: "test-token"}

	showID, err := resolveOrCreateShow(flags, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showID != "spotify:show:explicit123" {
		t.Errorf("showID = %q, want %q", showID, "spotify:show:explicit123")
	}
}

func TestResolveOrCreateShow_FromAPI(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/shows" && r.Method == "GET" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:fromapi456", Title: "API Show", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer server.Close()

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	flags := &uploadFlags{} // no showID, no newShow -- should use last show from API

	showID, err := resolveOrCreateShow(flags, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showID != "spotify:show:fromapi456" {
		t.Errorf("showID = %q, want %q", showID, "spotify:show:fromapi456")
	}
}

func TestResolveOrCreateShow_AutoCreate(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/shows" && r.Method == "GET" {
			// Return empty list to trigger auto-create
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(listShowsResponse{Shows: nil})
			return
		}
		if r.URL.Path == "/api/v1/shows" && r.Method == "POST" {
			var body showCreateRequest
			json.NewDecoder(r.Body).Decode(&body)
			if body.Title != "My Personal Podcast" {
				t.Errorf("show title = %q, want %q", body.Title, "My Personal Podcast")
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(showCreateResponse{
				ShowURI: "spotify:show:new789",
			})
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer server.Close()

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	flags := &uploadFlags{} // no newShow, no showID, no API shows -> auto-create with default title

	showID, err := resolveOrCreateShow(flags, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showID != "spotify:show:new789" {
		t.Errorf("showID = %q, want %q", showID, "spotify:show:new789")
	}
}

func TestResolveOrCreateShow_NewShowAlwaysCreates(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/shows" && r.Method == "POST" {
			var body showCreateRequest
			json.NewDecoder(r.Body).Decode(&body)
			if body.Title != "Brand New Show" {
				t.Errorf("show title = %q, want %q", body.Title, "Brand New Show")
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(showCreateResponse{
				ShowURI: "spotify:show:brandnew",
			})
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer server.Close()

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	token := &config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}
	if err := config.SaveToken(token); err != nil {
		t.Fatal(err)
	}

	flags := &uploadFlags{newShow: "Brand New Show"}

	showID, err := resolveOrCreateShow(flags, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if showID != "spotify:show:brandnew" {
		t.Errorf("showID = %q, want %q", showID, "spotify:show:brandnew")
	}
}
