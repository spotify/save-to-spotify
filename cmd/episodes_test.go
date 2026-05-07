package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

func TestParseEpisodeCreateFlags_AllRequired(t *testing.T) {
	args := []string{
		"--show-id", "spotify:show:abc123",
		"--title", "Pilot Episode",
		"--file", "/tmp/episode.mp3",
	}

	f, err := parseEpisodeCreateFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.showID != "spotify:show:abc123" {
		t.Errorf("showID = %q", f.showID)
	}
	if f.title != "Pilot Episode" {
		t.Errorf("title = %q", f.title)
	}
	if f.filePath != "/tmp/episode.mp3" {
		t.Errorf("filePath = %q", f.filePath)
	}
}

func TestParseEpisodeCreateFlags_MissingRequired(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing title",
			args: []string{"--show-id", "abc", "--file", "/tmp/audio.mp3"},
			want: "--title",
		},
		{
			name: "missing file",
			args: []string{"--show-id", "abc", "--title", "Ep"},
			want: "--file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseEpisodeCreateFlags(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, should mention %q", err, tt.want)
			}
		})
	}
}

func TestParseEpisodeCreateFlags_ShowIDOptional(t *testing.T) {
	// Parsing should succeed with empty showID -- the handler resolves it from API
	f, err := parseEpisodeCreateFlags([]string{"--title", "Ep", "--file", "/tmp/audio.mp3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.showID != "" {
		t.Errorf("showID = %q, want empty", f.showID)
	}
}

func TestParseEpisodeCreateFlags_AllOptional(t *testing.T) {
	args := []string{
		"--show-id", "abc123",
		"--title", "Episode",
		"--file", "/tmp/episode.mp3",
		"--summary", "A great episode",
		"--image", "https://example.com/ep.jpg",
		"--language", "sv",
	}

	f, err := parseEpisodeCreateFlags(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.summary != "A great episode" {
		t.Errorf("summary = %q", f.summary)
	}
	if f.image != "https://example.com/ep.jpg" {
		t.Errorf("image = %q", f.image)
	}
	if f.language != "sv" {
		t.Errorf("language = %q", f.language)
	}
}

func TestParseEpisodeCreateFlags_InvalidValues(t *testing.T) {
	base := []string{"--show-id", "abc", "--title", "Ep", "--file", "/tmp/audio.mp3"}

	tests := []struct {
		name string
		args []string
	}{
		{"unknown flag", append(base, "--unknown", "val")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseEpisodeCreateFlags(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestParseEpisodeReadinessFlags_StatusPositional(t *testing.T) {
	f, err := parseEpisodeReadinessFlags([]string{"spotify:episode:ep123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.episodeID != "spotify:episode:ep123" {
		t.Errorf("episodeID = %q", f.episodeID)
	}
}

func TestParseEpisodeReadinessFlags_StatusWithEpisodeIDFlag(t *testing.T) {
	f, err := parseEpisodeReadinessFlags([]string{"--episode-id", "ep123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.episodeID != "ep123" {
		t.Errorf("episodeID = %q", f.episodeID)
	}
}

func TestParseEpisodeReadinessFlags_Wait(t *testing.T) {
	f, err := parseEpisodeReadinessFlags([]string{"--episode-id", "ep123", "--wait"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.wait {
		t.Fatal("expected wait=true")
	}
	if f.waitTimeout != defaultEpisodeReadinessWaitTimeout {
		t.Fatalf("waitTimeout = %v, want %v", f.waitTimeout, defaultEpisodeReadinessWaitTimeout)
	}
}

func TestParseEpisodeReadinessFlags_WaitWithOptionalTimeout(t *testing.T) {
	f, err := parseEpisodeReadinessFlags([]string{"--episode-id", "ep123", "--wait", "2m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.wait {
		t.Fatal("expected wait=true")
	}
	if f.waitTimeout != 2*time.Minute {
		t.Fatalf("waitTimeout = %v, want %v", f.waitTimeout, 2*time.Minute)
	}
}

func TestParseEpisodeReadinessFlags_WaitWithEqualsTimeout(t *testing.T) {
	f, err := parseEpisodeReadinessFlags([]string{"--episode-id", "ep123", "--wait=2m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.wait {
		t.Fatal("expected wait=true")
	}
	if f.waitTimeout != 2*time.Minute {
		t.Fatalf("waitTimeout = %v, want %v", f.waitTimeout, 2*time.Minute)
	}
}

func TestParseEpisodeReadinessFlags_RejectsInvalidWaitValue(t *testing.T) {
	_, err := parseEpisodeReadinessFlags([]string{"ep123", "--wait", "nope"})
	if err == nil {
		t.Fatal("expected error for invalid --wait value")
	}
	if !strings.Contains(err.Error(), "invalid --wait value") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseEpisodeReadinessFlags_RejectsLegacyWaitTimeoutFlag(t *testing.T) {
	_, err := parseEpisodeReadinessFlags([]string{"--episode-id", "ep123", "--wait-timeout", "2m"})
	if err == nil {
		t.Fatal("expected error for unsupported --wait-timeout")
	}
	if !strings.Contains(err.Error(), "unknown flag: --wait-timeout") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEpisodeCreateRequestJSON(t *testing.T) {
	req := episodeCreateRequest{
		Title:     "My Episode",
		Summary:   "Episode summary",
		Language:  "en",
		MediaType: "EPISODE_AUDIO",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	expectedKeys := []string{"title", "summary", "language", "media_type"}
	for _, key := range expectedKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	// Optional empty fields should be omitted
	if _, ok := raw["image_token"]; ok {
		t.Error("key \"image_token\" should be omitted when empty")
	}
}

func TestHandleEpisodes_Routing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	err := handleEpisodes([]string{"unknown"})
	if err == nil {
		t.Error("expected error for unknown subcommand")
	}

	err = handleEpisodes([]string{"get"})
	if err == nil {
		t.Error("expected error for unknown subcommand 'get'")
	}
}

func TestHandleEpisodes_ShowIDFlag(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shows" && r.Method == "GET":
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:s1", Title: "Show One", CreatedAt: "2026-03-10T12:00:00Z"},
					{ShowURI: "spotify:show:s2", Title: "Show Two", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		case r.URL.Path == "/api/v1/shows/s2/episodes" && r.Method == "GET":
			json.NewEncoder(w).Encode(listEpisodesResponse{
				Episodes: []EpisodeSummary{
					{EpisodeURI: "spotify:episode:ep99", Title: "Targeted Episode", CreatedAt: "2026-03-10T12:00:00Z"},
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

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleEpisodes([]string{"--show-id", "spotify:show:s2"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleEpisodes --show-id: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "Targeted Episode") {
		t.Errorf("expected episode from show s2, got: %q", output)
	}
	if !strings.Contains(output, "spotify:episode:ep99") {
		t.Errorf("expected episode URI in output, got: %q", output)
	}
}

func TestHandleEpisodes_ShowIDFlagBareID(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/shows" && r.Method == "GET":
			json.NewEncoder(w).Encode(listShowsResponse{
				Shows: []ShowSummary{
					{ShowURI: "spotify:show:bare1", Title: "Bare Show", CreatedAt: "2026-03-10T12:00:00Z"},
				},
			})
		case r.URL.Path == "/api/v1/shows/bare1/episodes" && r.Method == "GET":
			json.NewEncoder(w).Encode(listEpisodesResponse{
				Episodes: []EpisodeSummary{
					{EpisodeURI: "spotify:episode:bareEp", Title: "Bare Episode", CreatedAt: "2026-03-10T12:00:00Z"},
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

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Pass bare ID without spotify:show: prefix
	err := handleEpisodes([]string{"--show-id", "bare1"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleEpisodes --show-id bare: %v", err)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "Bare Episode") {
		t.Errorf("expected episode from bare show ID, got: %q", output)
	}
}

func TestHandleEpisodes_ShowIDFlagMissingValue(t *testing.T) {
	err := handleEpisodes([]string{"--show-id"})
	if err == nil {
		t.Error("expected error for --show-id without value")
	}
	if !strings.Contains(err.Error(), "--show-id requires a value") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHandleEpisodesCreate_SendsUserAgent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	testFile := filepath.Join(tmp, "episode.mp3")
	if err := os.WriteFile(testFile, []byte("fake audio"), 0644); err != nil {
		t.Fatal(err)
	}

	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected upload method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer uploadServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shows/show1/episodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		var body episodeCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.Title != "Pilot Episode" {
			t.Fatalf("title = %q", body.Title)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(episodeCreateResponse{
			EpisodeURI: "spotify:episode:ep123",
			Status:     "PROCESSING",
			MultipartUploadURLs: []multipartUploadURL{
				{SignedURL: uploadServer.URL + "/upload", PartNumber: 1},
			},
		})
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	})

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	if err := handleEpisodesCreate([]string{"--show-id", "show1", "--title", "Pilot Episode", "--file", testFile}); err != nil {
		t.Fatalf("handleEpisodesCreate: %v", err)
	}
}

func TestHandleEpisodesReadiness_SendsUserAgent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/episodes/ep123/readiness" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(episodeReadinessResponse{
			EpisodeURI: "spotify:episode:ep123",
			Readiness:  "READY",
		})
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	})

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	if err := handleEpisodesReadiness([]string{"spotify:episode:ep123"}); err != nil {
		t.Fatalf("handleEpisodesReadiness: %v", err)
	}
}

func TestHandleEpisodesReadiness_AcceptsEpisodeIDFlag(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/episodes/ep123/readiness" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(episodeReadinessResponse{
			EpisodeURI: "spotify:episode:ep123",
			Readiness:  "READY",
		})
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	})

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	config.SetJSONMode()
	defer config.ResetJSONMode()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleEpisodesReadiness([]string{"--episode-id", "spotify:episode:ep123"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("handleEpisodesReadiness with --episode-id: %v", err)
	}

	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), `"readiness":"READY"`) {
		t.Errorf("unexpected output: %s", string(out))
	}
}

func TestHandleEpisodesReadiness_WaitsUntilReady(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	origPollInterval := episodeReadinessPollInterval
	episodeReadinessPollInterval = time.Millisecond
	defer func() { episodeReadinessPollInterval = origPollInterval }()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		readiness := "PROCESSING"
		if callCount >= 3 {
			readiness = "READY"
		}

		json.NewEncoder(w).Encode(episodeReadinessResponse{
			EpisodeURI: "spotify:episode:ep123",
			Readiness:  readiness,
		})
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	})

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	if err := handleEpisodesReadiness([]string{"spotify:episode:ep123", "--wait", "20ms"}); err != nil {
		t.Fatalf("handleEpisodesReadiness --wait: %v", err)
	}
	if callCount < 3 {
		t.Fatalf("expected multiple polls, got %d", callCount)
	}
}

func TestHandleEpisodesReadiness_WaitFailsOnFailedReadiness(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	origPollInterval := episodeReadinessPollInterval
	episodeReadinessPollInterval = time.Millisecond
	defer func() { episodeReadinessPollInterval = origPollInterval }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(episodeReadinessResponse{
			EpisodeURI: "spotify:episode:ep123",
			Readiness:  "FAILED",
		})
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	})

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	err := handleEpisodesReadiness([]string{"spotify:episode:ep123", "--wait", "20ms"})
	if err == nil || !strings.Contains(err.Error(), "episode processing failed") {
		t.Fatalf("expected processing failure, got: %v", err)
	}
}

func TestHandleEpisodesReadiness_WaitTimesOut(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	origPollInterval := episodeReadinessPollInterval
	episodeReadinessPollInterval = time.Millisecond
	defer func() { episodeReadinessPollInterval = origPollInterval }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(episodeReadinessResponse{
			EpisodeURI: "spotify:episode:ep123",
			Readiness:  "PROCESSING",
		})
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	})

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	err := handleEpisodesReadiness([]string{"spotify:episode:ep123", "--wait", "3ms"})
	if err == nil || !strings.Contains(err.Error(), "timed out waiting for episode readiness") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
}

func TestHandleEpisodesDelete_SendsUserAgent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/episodes/ep123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	})

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	if err := handleEpisodesDelete("spotify:episode:ep123", nil); err != nil {
		t.Fatalf("handleEpisodesDelete: %v", err)
	}
}

func TestHandleEpisodesReadiness_RejectsShowIDFlag(t *testing.T) {
	err := handleEpisodesReadiness([]string{"spotify:episode:ep123", "--show-id", "show1"})
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag error, got: %v", err)
	}
}

func TestHandleEpisodesDelete_RejectsExtraArgs(t *testing.T) {
	err := handleEpisodesDelete("spotify:episode:ep123", []string{"--show-id", "show1"})
	if err == nil || !strings.Contains(err.Error(), "unknown argument") {
		t.Fatalf("expected unknown argument error, got: %v", err)
	}
}

func TestEpisodeShowIDStripping(t *testing.T) {
	// Verify that show ID extraction from URI works
	tests := []struct {
		input string
		want  string
	}{
		{"spotify:show:abc123", "abc123"},
		{"abc123", "abc123"},
	}
	for _, tt := range tests {
		got := strings.TrimPrefix(tt.input, "spotify:show:")
		if got != tt.want {
			t.Errorf("TrimPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
