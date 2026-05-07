package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

func TestHandleListEpisodes_DefaultShow(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shows" && r.Method == "GET":
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:abc123", Title: "My Show", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		case r.URL.Path == "/api/v1/shows/abc123/episodes" && r.Method == "GET":
			json.NewEncoder(w).Encode(listEpisodesResponse{
				Episodes: []EpisodeSummary{
					{EpisodeURI: "spotify:episode:ep1", Title: "Pilot", CreatedAt: "2026-03-10T12:00:00Z"},
					{EpisodeURI: "spotify:episode:ep2", Title: "Episode 2", CreatedAt: "2026-03-10T14:00:00Z"},
				},
			})
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleListEpisodes(nil)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleListEpisodes: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "Episodes for: spotify:show:abc123 (My Show)") {
		t.Errorf("missing header in output: %q", output)
	}
	if !strings.Contains(output, "spotify:episode:ep1") {
		t.Errorf("missing ep1 in output: %q", output)
	}
	if !strings.Contains(output, "spotify:episode:ep2") {
		t.Errorf("missing ep2 in output: %q", output)
	}
}

func TestHandleListEpisodes_FilterByShowID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shows" && r.Method == "GET":
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:show1", Title: "Show 1", CreatedAt: "2026-03-10T12:00:00Z"},
					{ShowURI: "spotify:show:show2", Title: "Show 2", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		case r.URL.Path == "/api/v1/shows/show1/episodes" && r.Method == "GET":
			json.NewEncoder(w).Encode(listEpisodesResponse{
				Episodes: []EpisodeSummary{
					{EpisodeURI: "spotify:episode:ep1", Title: "Ep From Show1", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleListEpisodes([]string{"--show-id", "spotify:show:show1"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleListEpisodes: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "Ep From Show1") {
		t.Errorf("missing show1 episode in output: %q", output)
	}
}

func TestHandleListEpisodes_NoShows(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/shows" && r.Method == "GET" {
			json.NewEncoder(w).Encode(listShowsResponse{Shows: nil})
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleListEpisodes(nil)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleListEpisodes: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "No shows found") {
		t.Errorf("output = %q, expected 'No shows found' message", output)
	}
}

func TestHandleListEpisodes_NoEpisodes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shows" && r.Method == "GET":
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:empty", Title: "Empty Show", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		case r.URL.Path == "/api/v1/shows/empty/episodes" && r.Method == "GET":
			json.NewEncoder(w).Encode(listEpisodesResponse{Episodes: nil})
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleListEpisodes(nil)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleListEpisodes: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "No episodes found for show spotify:show:empty") {
		t.Errorf("output = %q", output)
	}
}

func TestHandleListEpisodes_JSONOutput(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shows" && r.Method == "GET":
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:abc123", Title: "My Show", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		case r.URL.Path == "/api/v1/shows/abc123/episodes" && r.Method == "GET":
			json.NewEncoder(w).Encode(listEpisodesResponse{
				Episodes: []EpisodeSummary{
					{EpisodeURI: "spotify:episode:ep1", Title: "Pilot", CreatedAt: "2026-03-10T12:00:00Z"},
					{EpisodeURI: "spotify:episode:ep2", Title: "Episode 2", CreatedAt: "2026-03-10T14:00:00Z"},
				},
			})
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleListEpisodes(nil)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleListEpisodes: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	episodes, ok := parsed["episodes"].([]interface{})
	if !ok {
		t.Fatalf("episodes is not an array: %v", parsed["episodes"])
	}
	if len(episodes) != 2 {
		t.Fatalf("expected 2 episodes, got %d", len(episodes))
	}

	ep1 := episodes[0].(map[string]interface{})
	if ep1["episode_uri"] != "spotify:episode:ep1" {
		t.Errorf("episode_uri = %v", ep1["episode_uri"])
	}
	if ep1["title"] != "Pilot" {
		t.Errorf("title = %v", ep1["title"])
	}
}

func TestHandleListEpisodes_JSONOutput_NoShows(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/shows" && r.Method == "GET" {
			json.NewEncoder(w).Encode(listShowsResponse{Shows: nil})
			return
		}
		http.Error(w, "not found", 404)
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleListEpisodes(nil)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleListEpisodes: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	episodes, ok := parsed["episodes"].([]interface{})
	if !ok {
		t.Fatalf("episodes is not an array: %v", parsed["episodes"])
	}
	if len(episodes) != 0 {
		t.Errorf("expected empty episodes array, got %d items", len(episodes))
	}
}

func TestHandleListEpisodes_JSONOutput_NoEpisodes(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shows" && r.Method == "GET":
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:empty", Title: "Empty Show", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		case r.URL.Path == "/api/v1/shows/empty/episodes" && r.Method == "GET":
			json.NewEncoder(w).Encode(listEpisodesResponse{Episodes: nil})
		default:
			http.Error(w, "not found", 404)
		}
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleListEpisodes(nil)

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleListEpisodes: %v", err)
	}

	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	episodes, ok := parsed["episodes"].([]interface{})
	if !ok {
		t.Fatalf("episodes is not an array: %v", parsed["episodes"])
	}
	if len(episodes) != 0 {
		t.Errorf("expected empty episodes array, got %d items", len(episodes))
	}
}

func TestHandleList_Routing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Unknown subcommand
	err := handleList([]string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}
}
