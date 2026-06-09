package cmd

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
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

func intPtr(v int) *int {
	return &v
}

func writeTestJPEG(t *testing.T, path string, width, height int) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})

	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
}

// --- flag parsing ---

func TestParseTimelineSetFlags_AllRequired(t *testing.T) {
	f, err := parseTimelineSetFlags([]string{
		"--episode-id", "spotify:episode:abc123",
		"--from-file", "/tmp/timeline.json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.episodeID != "spotify:episode:abc123" {
		t.Errorf("episodeID = %q", f.episodeID)
	}
	if f.fromFile != "/tmp/timeline.json" {
		t.Errorf("fromFile = %q", f.fromFile)
	}
}

func TestParseTimelineSetFlags_WithShowID(t *testing.T) {
	f, err := parseTimelineSetFlags([]string{
		"--episode-id", "ep1",
		"--from-file", "tl.json",
		"--show-id", "show1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.showID != "show1" {
		t.Errorf("showID = %q", f.showID)
	}
}

func TestParseTimelineSetFlags_MissingRequired(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing episode-id", []string{"--from-file", "f.json"}, "--episode-id"},
		{"missing from-file", []string{"--episode-id", "abc"}, "--from-file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimelineSetFlags(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, should mention %q", err, tt.want)
			}
		})
	}
}

func TestParseTimelineSetFlags_UnknownFlag(t *testing.T) {
	_, err := parseTimelineSetFlags([]string{"--episode-id", "abc", "--from-file", "f.json", "--unknown"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("error = %q", err)
	}
}

// --- validation ---

func TestValidateTimeline_ValidMixed(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "Intro", StartTimeMs: 0}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 5000, Image: "img.jpg"}},
		{Chapter: &timelineChapter{Title: "End", StartTimeMs: 60000}},
		{Link: &timelineLink{StartTimeMs: 20000, DurationMs: 5000, URL: "https://example.com"}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_ChaptersOnly(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "Intro", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "End", StartTimeMs: 60000}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_CompanionOnly(t *testing.T) {
	items := []timelineItem{
		{Image: &timelineImage{StartTimeMs: 0, DurationMs: 5000, Image: "img.jpg"}},
		{Link: &timelineLink{StartTimeMs: 10000, DurationMs: 5000, URL: "https://example.com"}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_TooFewChapters(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "Only", StartTimeMs: 0}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_FirstChapterNotZero(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 1000}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 2000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "first chapter must start at 0") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ChaptersNotIncreasing(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Chapter: &timelineChapter{Title: "C", StartTimeMs: 30000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "strictly increasing") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ChapterTooShort(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 3000}},
		{Chapter: &timelineChapter{Title: "C", StartTimeMs: 60000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "at least 5s long") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_TooManyShortChapters(t *testing.T) {
	// 6 chapters, 4 of them < 30s — ceil(6*0.15+1) = ceil(1.9) = 2, so max 2 short chapters
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 10000}},
		{Chapter: &timelineChapter{Title: "C", StartTimeMs: 20000}},
		{Chapter: &timelineChapter{Title: "D", StartTimeMs: 25000}},
		{Chapter: &timelineChapter{Title: "E", StartTimeMs: 35000}},
		{Chapter: &timelineChapter{Title: "F", StartTimeMs: 300000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "too many short chapters") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ShortChaptersWithinLimit(t *testing.T) {
	// 6 chapters, 1 short — ceil(6*0.15+1) = 2, so 1 short is fine
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 10000}},
		{Chapter: &timelineChapter{Title: "C", StartTimeMs: 60000}},
		{Chapter: &timelineChapter{Title: "D", StartTimeMs: 120000}},
		{Chapter: &timelineChapter{Title: "E", StartTimeMs: 180000}},
		{Chapter: &timelineChapter{Title: "F", StartTimeMs: 300000}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_FinalChapterMayBeShort(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Chapter: &timelineChapter{Title: "C", StartTimeMs: 80000}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_ChapterEmptyTitle(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "", StartTimeMs: 60000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "empty title") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ImageMissingDuration(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 0, Image: "img.jpg"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duration_ms must be positive") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ImageMissingPath(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 5000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "image file path is required") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ImageTokenMissingDimensions(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 5000, ImageToken: "tok"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "width and height are required") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ImageTokenWithDimensions(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 5000, ImageToken: "tok", Width: 640, Height: 480}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_ImageInvalidURL(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 5000, Image: "img.jpg", URL: "not-a-url"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "HTTP(S) URL") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_LinkMissingURL(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Link: &timelineLink{StartTimeMs: 10000, DurationMs: 5000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "valid HTTP(S) URL") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_LinkInvalidURL(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Link: &timelineLink{StartTimeMs: 10000, DurationMs: 5000, URL: "ftp://nope"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "valid HTTP(S) URL") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_LinkMissingDuration(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Link: &timelineLink{StartTimeMs: 10000, DurationMs: 0, URL: "https://example.com"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duration_ms must be positive") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_SpotifyEntityWithDuration(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{SpotifyEntity: &timelineSpotifyEntity{StartTimeMs: 10000, DurationMs: intPtr(5000), URI: "spotify:track:abc"}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_SpotifyEntityWithoutDuration(t *testing.T) {
	items := []timelineItem{
		{SpotifyEntity: &timelineSpotifyEntity{StartTimeMs: 10000, URI: "spotify:album:xyz"}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_SpotifyEntityAndLinkCanCoexist(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{SpotifyEntity: &timelineSpotifyEntity{StartTimeMs: 10000, DurationMs: intPtr(5000), URI: "spotify:track:abc"}},
		{Link: &timelineLink{StartTimeMs: 16000, DurationMs: 5000, URL: "https://example.com/source"}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_SpotifyEntityMissingURI(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{SpotifyEntity: &timelineSpotifyEntity{StartTimeMs: 10000}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "uri is required") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_SpotifyEntityRequiresSpotifyURI(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{SpotifyEntity: &timelineSpotifyEntity{StartTimeMs: 10000, URI: "https://open.spotify.com/track/abc"}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "full Spotify URI") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_SpotifyEntityInvalidDuration(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{SpotifyEntity: &timelineSpotifyEntity{StartTimeMs: 10000, DurationMs: intPtr(0), URI: "spotify:track:abc"}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duration_ms must be positive when provided") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_CompanionOverlap(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 20000, Image: "a.jpg"}},
		{Link: &timelineLink{StartTimeMs: 25000, DurationMs: 5000, URL: "https://example.com"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_CompanionNoOverlap(t *testing.T) {
	// Adjacent (edge = edge) should be OK
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 5000, Image: "a.jpg"}},
		{Link: &timelineLink{StartTimeMs: 15000, DurationMs: 5000, URL: "https://example.com"}},
	}
	if err := validateTimeline(items); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTimeline_SpotifyEntityOverlap(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Image: &timelineImage{StartTimeMs: 10000, DurationMs: 10000, Image: "a.jpg"}},
		{SpotifyEntity: &timelineSpotifyEntity{StartTimeMs: 15000, URI: "spotify:track:abc"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overlaps") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ItemNoneSet(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{}, // none set
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_ItemMultipleSet(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: 0}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
		{Chapter: &timelineChapter{Title: "C", StartTimeMs: 120000}, Image: &timelineImage{StartTimeMs: 120000, DurationMs: 5000, Image: "x.jpg"}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error = %q", err)
	}
}

func TestValidateTimeline_NegativeStartTime(t *testing.T) {
	items := []timelineItem{
		{Chapter: &timelineChapter{Title: "A", StartTimeMs: -1}},
		{Chapter: &timelineChapter{Title: "B", StartTimeMs: 60000}},
	}
	err := validateTimeline(items)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Errorf("error = %q", err)
	}
}

// --- routing ---

func TestHandleTimeline_Routing(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown subcommand", []string{"bogus"}, "unknown timeline subcommand"},
		{"get without id", []string{"get"}, "usage:"},
		{"delete without id", []string{"delete"}, "usage:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleTimeline(tt.args)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want %q", err, tt.want)
			}
		})
	}
}

// --- HTTP integration helpers ---

func setupTimelineTest(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	if err := config.SaveToken(&config.TokenData{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		ExpiresAt:    time.Now().Add(1 * time.Hour),
	}); err != nil {
		t.Fatalf("failed to save token: %v", err)
	}
	config.ResetJSONMode()
}

func writeTimelineJSON(t *testing.T, dir string, content string) string {
	t.Helper()
	path := filepath.Join(dir, "timeline.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write timeline.json: %v", err)
	}
	return path
}

// --- set ---

func TestHandleTimelineSet_Success(t *testing.T) {
	setupTimelineTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PUT" && strings.Contains(r.URL.Path, "/timeline") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"items":[{"chapter":{"chapter_uri":"spotify:chapter:a1","title":"Intro","start_time_ms":0}},{"chapter":{"chapter_uri":"spotify:chapter:a2","title":"End","start_time_ms":60000}}]}`)
			return
		}
		// shows list for resolveShowIDForEpisode
		if r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/shows") {
			fmt.Fprint(w, `{"shows":[{"show_uri":"spotify:show:s1","title":"Test"}]}`)
			return
		}
		// episodes list
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/episodes") {
			fmt.Fprint(w, `{"episodes":[{"episode_uri":"spotify:episode:ep1"}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	tmp := t.TempDir()
	tlPath := writeTimelineJSON(t, tmp, `{
		"items": [
			{"chapter": {"title": "Intro", "start_time_ms": 0}},
			{"chapter": {"title": "End", "start_time_ms": 60000}}
		]
	}`)

	config.SetJSONMode()
	defer config.ResetJSONMode()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleTimelineSet([]string{"--episode-id", "ep1", "--from-file", tlPath, "--show-id", "s1"})

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	output := string(out)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp timelineResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, output)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
}

func TestHandleTimelineSet_LocalImageAddsDimensionsToPayload(t *testing.T) {
	setupTimelineTest(t)

	type capturedImage struct {
		StartTimeMs int    `json:"start_time_ms"`
		DurationMs  int    `json:"duration_ms"`
		ImageToken  string `json:"image_token"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
	}
	type capturedItem struct {
		Image *capturedImage `json:"image,omitempty"`
	}
	type capturedTimeline struct {
		Items []capturedItem `json:"items"`
	}

	var uploadCalls int
	var captured capturedTimeline

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/images":
			uploadCalls++
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"upload_token":"tok_123"}`)
			return
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/timeline"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			if err := json.Unmarshal(body, &captured); err != nil {
				t.Fatalf("failed to unmarshal request body: %v\nbody=%s", err, string(body))
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"items":[
				{"chapter":{"chapter_uri":"spotify:chapter:a1","title":"Intro","start_time_ms":0}},
				{"image":{"companion_uri":"spotify:companion:i1","start_time_ms":10000,"duration_ms":5000,"image_token":"tok_123","width":640,"height":480}},
				{"chapter":{"chapter_uri":"spotify:chapter:a2","title":"End","start_time_ms":60000}}
			]}`)
			return
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/shows"):
			fmt.Fprint(w, `{"shows":[{"show_uri":"spotify:show:s1","title":"Test"}]}`)
			return
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/episodes"):
			fmt.Fprint(w, `{"episodes":[{"episode_uri":"spotify:episode:ep1"}]}`)
			return
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	tmp := t.TempDir()
	imagePath := filepath.Join(tmp, "cover.jpg")
	writeTestJPEG(t, imagePath, 640, 480)
	tlPath := writeTimelineJSON(t, tmp, `{
		"items": [
			{"chapter": {"title": "Intro", "start_time_ms": 0}},
			{"image": {"start_time_ms": 10000, "duration_ms": 5000, "image": "cover.jpg"}},
			{"chapter": {"title": "End", "start_time_ms": 60000}}
		]
	}`)

	config.SetJSONMode()
	defer config.ResetJSONMode()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleTimelineSet([]string{"--episode-id", "ep1", "--from-file", tlPath, "--show-id", "s1"})

	w.Close()
	os.Stdout = old
	_, _ = io.ReadAll(r)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uploadCalls != 1 {
		t.Fatalf("image uploads = %d, want 1", uploadCalls)
	}
	if len(captured.Items) != 3 || captured.Items[1].Image == nil {
		t.Fatalf("captured timeline missing image item: %+v", captured)
	}
	got := captured.Items[1].Image
	if got.ImageToken != "tok_123" {
		t.Fatalf("image_token = %q, want %q", got.ImageToken, "tok_123")
	}
	if got.Width != 640 || got.Height != 480 {
		t.Fatalf("dimensions = %dx%d, want 640x480", got.Width, got.Height)
	}
}

func TestHandleTimelineSet_ValidationError(t *testing.T) {
	setupTimelineTest(t)

	tmp := t.TempDir()
	// Only 1 chapter — should fail validation
	tlPath := writeTimelineJSON(t, tmp, `{
		"items": [
			{"chapter": {"title": "Only", "start_time_ms": 0}}
		]
	}`)

	err := handleTimelineSet([]string{"--episode-id", "ep1", "--from-file", tlPath})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "at least 2") {
		t.Errorf("error = %q", err)
	}
}

func TestHandleTimelineSet_InvalidJSON(t *testing.T) {
	setupTimelineTest(t)

	tmp := t.TempDir()
	tlPath := writeTimelineJSON(t, tmp, `{not valid json`)

	err := handleTimelineSet([]string{"--episode-id", "ep1", "--from-file", tlPath})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("error = %q", err)
	}
}

func TestHandleTimelineSet_FileNotFound(t *testing.T) {
	setupTimelineTest(t)

	err := handleTimelineSet([]string{"--episode-id", "ep1", "--from-file", "/nonexistent/timeline.json"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "failed to read") {
		t.Errorf("error = %q", err)
	}
}

// --- get ---

func TestHandleTimelineGet_Success(t *testing.T) {
	setupTimelineTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/timeline") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"items":[
				{"chapter":{"chapter_uri":"spotify:chapter:c1","title":"Intro","start_time_ms":0}},
				{"image":{"companion_uri":"spotify:companion:i1","start_time_ms":10000,"duration_ms":5000,"image_token":"tok","title":"Book Cover"}},
				{"spotify_entity":{"companion_uri":"spotify:companion:s1","start_time_ms":15000,"uri":"spotify:track:abc"}},
				{"chapter":{"chapter_uri":"spotify:chapter:c2","title":"End","start_time_ms":60000}},
				{"link":{"companion_uri":"spotify:companion:l1","start_time_ms":20000,"duration_ms":5000,"url":"https://example.com"}}
			]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleTimelineGet("ep1", []string{"--show-id", "s1"})

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	output := string(out)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "Intro") {
		t.Errorf("output missing chapter title: %s", output)
	}
	if !strings.Contains(output, "Book Cover") {
		t.Errorf("output missing image title: %s", output)
	}
	if !strings.Contains(output, "spotify:track:abc") {
		t.Errorf("output missing spotify entity URI: %s", output)
	}
	if strings.Contains(output, "https://example.com") {
		t.Errorf("output should not include link URL: %s", output)
	}
	if !strings.Contains(output, "spotify:companion:l1") {
		t.Errorf("output missing link companion URI: %s", output)
	}
}

func TestHandleTimelineGet_JSONOutput(t *testing.T) {
	setupTimelineTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/timeline") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"items":[
				{"image":{"companion_uri":"spotify:companion:i1","start_time_ms":10000,"duration_ms":5000,"url":"https://example.com/image","title":"Book Cover"}},
				{"link":{"companion_uri":"spotify:companion:l1","start_time_ms":12000,"duration_ms":5000,"url":"https://example.com/link"}},
				{"spotify_entity":{"companion_uri":"spotify:companion:s1","start_time_ms":15000,"uri":"spotify:track:abc"}}
			]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	config.SetJSONMode()
	defer config.ResetJSONMode()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleTimelineGet("ep1", []string{"--show-id", "s1"})

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	output := string(out)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nOutput: %s", err, output)
	}
	items, ok := parsed["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("items = %#v, want three items", parsed["items"])
	}
	imageItem, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item = %#v, want object", items[0])
	}
	image, ok := imageItem["image"].(map[string]any)
	if !ok {
		t.Fatalf("image = %#v, want object", imageItem["image"])
	}
	if _, ok := image["url"]; ok {
		t.Fatalf("image url should be omitted from response output, got %#v", image["url"])
	}
	linkItem, ok := items[1].(map[string]any)
	if !ok {
		t.Fatalf("item = %#v, want object", items[1])
	}
	link, ok := linkItem["link"].(map[string]any)
	if !ok {
		t.Fatalf("link = %#v, want object", linkItem["link"])
	}
	if _, ok := link["url"]; ok {
		t.Fatalf("link url should be omitted from response output, got %#v", link["url"])
	}
	entityItem, ok := items[2].(map[string]any)
	if !ok {
		t.Fatalf("item = %#v, want object", items[2])
	}
	entity, ok := entityItem["spotify_entity"].(map[string]any)
	if !ok {
		t.Fatalf("spotify_entity = %#v, want object", entityItem["spotify_entity"])
	}
	if _, ok := entity["duration_ms"]; ok {
		t.Fatalf("duration_ms should be omitted when absent, got %#v", entity["duration_ms"])
	}
}

func TestHandleTimelineGet_Empty(t *testing.T) {
	setupTimelineTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && strings.Contains(r.URL.Path, "/timeline") {
			fmt.Fprint(w, `{"items":[]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleTimelineGet("ep1", []string{"--show-id", "s1"})

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	output := string(out)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "No timeline items") {
		t.Errorf("expected empty message, got: %s", output)
	}
}

// --- delete ---

func TestHandleTimelineDelete_Success(t *testing.T) {
	setupTimelineTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/timeline") {
			w.WriteHeader(204)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleTimelineDelete("ep1", []string{"--show-id", "s1"})

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	output := string(out)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "deleted") {
		t.Errorf("expected deletion confirmation, got: %s", output)
	}
}

func TestHandleTimelineDelete_JSONOutput(t *testing.T) {
	setupTimelineTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && strings.Contains(r.URL.Path, "/timeline") {
			w.WriteHeader(204)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	config.SetJSONMode()
	defer config.ResetJSONMode()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := handleTimelineDelete("ep1", []string{"--show-id", "s1"})

	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	output := string(out)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nOutput: %s", err, output)
	}
	if parsed["ok"] != true {
		t.Errorf("expected ok=true, got %v", parsed["ok"])
	}
}

func TestHandleTimelineDeleteRejectsUnsafeIDsBeforeRequest(t *testing.T) {
	tests := []struct {
		name      string
		showID    string
		episodeID string
	}{
		{
			name:      "unsafe show ID fragment",
			showID:    "VICTIMID#",
			episodeID: "ANY",
		},
		{
			name:      "unsafe episode ID fragment",
			showID:    "SAFEID",
			episodeID: "ANY#",
		},
		{
			name:      "unsafe show ID query",
			showID:    "VICTIMID?delete=true",
			episodeID: "ANY",
		},
		{
			name:      "unsafe episode ID query",
			showID:    "SAFEID",
			episodeID: "ANY?delete=true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTimelineTest(t)

			requested := make(chan struct{}, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case requested <- struct{}{}:
				default:
				}
				http.NotFound(w, r)
			}))
			defer server.Close()

			origURL := config.BackendBaseURL
			config.BackendBaseURL = server.URL
			defer func() { config.BackendBaseURL = origURL }()

			err := handleTimelineDelete(tt.episodeID, []string{"--show-id", tt.showID})
			if err == nil {
				t.Fatal("expected unsafe ID error")
			}
			if !strings.Contains(err.Error(), "unsafe") {
				t.Fatalf("error = %q, want unsafe ID error", err)
			}

			select {
			case <-requested:
				t.Fatal("backend received a request for an unsafe ID")
			default:
			}
		})
	}
}

func TestHandleTimelineDelete_APIError(t *testing.T) {
	setupTimelineTest(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":"internal server error"}`)
	}))
	defer server.Close()

	origURL := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	defer func() { config.BackendBaseURL = origURL }()

	err := handleTimelineDelete("ep1", []string{"--show-id", "s1"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "API error") {
		t.Errorf("error = %q", err)
	}
}
