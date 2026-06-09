package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spotify/save-to-spotify/config"
)

func printTimelineUsage() {
	fmt.Printf(`Usage: %[1]s timeline [command]

Commands:
  get <id>     Get timeline (chapters + companion items) for an episode
  set          Set timeline from a JSON file (replaces all existing items)
  delete <id>  Delete all timeline items for an episode

The timeline manages chapters and companion content (images, links,
Spotify entity cards) in a single command. Prefer adding
spotify_entity for Spotify-native references (tracks, albums, artists,
playlists, shows, episodes, audiobook URIs, etc.). Links and Spotify
entities can both appear in the same timeline when both destinations
are useful; just keep their time windows non-overlapping. Prefer full
spotify:... URI format anywhere a Spotify reference is accepted.
Use --json for machine-readable output:

  %[1]s --json timeline get <id> --show-id <id>
  %[1]s --json timeline set --episode-id <id> --from-file timeline.json

See 'timeline set --help' for the JSON format and validation rules.
`, binName)
}

func handleTimeline(args []string) error {
	if len(args) == 0 {
		printTimelineUsage()
		return nil
	}

	switch args[0] {
	case "get":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s timeline get <id> [--show-id <id>]\n\nGet timeline (chapters + companion content, including Spotify entity cards) for an episode.\nUse spotify_entity for Spotify-native references, link for external destinations, or both when both are useful. Prefer full spotify:... URIs.\n\nFlags:\n  --show-id <id>  Show ID or URI (default: last created show)\n\nUse --json for structured output.\n", binName)
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: %s timeline get <id> [--show-id <id>]", binName)
		}
		return handleTimelineGet(args[1], args[2:])
	case "set":
		return handleTimelineSet(args[1:])
	case "delete":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s timeline delete <id> [--show-id <id>]\n\nDelete all timeline items (chapters + companion content, including Spotify entity cards) for an episode.\n\nFlags:\n  --show-id <id>  Show ID or URI (default: last created show)\n\nUse --json for structured output.\n", binName)
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: %s timeline delete <id> [--show-id <id>]", binName)
		}
		return handleTimelineDelete(args[1], args[2:])
	case "-h", "--help", "help":
		printTimelineUsage()
		return nil
	default:
		return fmt.Errorf("unknown timeline subcommand: %s", args[0])
	}
}

// --- types (input) ---

type timelineFile struct {
	Items []timelineItem `json:"items"`
}

type timelineItem struct {
	Chapter       *timelineChapter       `json:"chapter,omitempty"`
	Image         *timelineImage         `json:"image,omitempty"`
	Link          *timelineLink          `json:"link,omitempty"`
	SpotifyEntity *timelineSpotifyEntity `json:"spotify_entity,omitempty"`
}

type timelineChapter struct {
	Title       string `json:"title"`
	StartTimeMs int    `json:"start_time_ms"`
	Description string `json:"description,omitempty"`
}

type timelineImage struct {
	StartTimeMs int    `json:"start_time_ms"`
	DurationMs  int    `json:"duration_ms"`
	Image       string `json:"image,omitempty"`       // local file path (input only, cleared before PUT)
	ImageToken  string `json:"image_token,omitempty"` // set after upload
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

type timelineLink struct {
	StartTimeMs int    `json:"start_time_ms"`
	DurationMs  int    `json:"duration_ms"`
	URL         string `json:"url"`
}

type timelineSpotifyEntity struct {
	StartTimeMs int    `json:"start_time_ms"`
	DurationMs  *int   `json:"duration_ms,omitempty"`
	URI         string `json:"uri"`
}

// --- types (response) ---

type timelineResponse struct {
	Items []timelineResponseItem `json:"items"`
}

type timelineResponseItem struct {
	Chapter       *timelineResponseChapter       `json:"chapter,omitempty"`
	Image         *timelineResponseImage         `json:"image,omitempty"`
	Link          *timelineResponseLink          `json:"link,omitempty"`
	SpotifyEntity *timelineResponseSpotifyEntity `json:"spotify_entity,omitempty"`
}

type timelineResponseChapter struct {
	ChapterURI  string `json:"chapter_uri"`
	Title       string `json:"title"`
	StartTimeMs int    `json:"start_time_ms"`
	Description string `json:"description,omitempty"`
}

type timelineResponseImage struct {
	CompanionURI string `json:"companion_uri"`
	StartTimeMs  int    `json:"start_time_ms"`
	DurationMs   int    `json:"duration_ms"`
	Title        string `json:"title,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
}

type timelineResponseLink struct {
	CompanionURI string `json:"companion_uri"`
	StartTimeMs  int    `json:"start_time_ms"`
	DurationMs   int    `json:"duration_ms"`
}

type timelineResponseSpotifyEntity struct {
	CompanionURI string `json:"companion_uri"`
	StartTimeMs  int    `json:"start_time_ms"`
	DurationMs   *int   `json:"duration_ms,omitempty"`
	URI          string `json:"uri"`
}

// --- set ---

type timelineSetFlags struct {
	episodeID string
	showID    string
	fromFile  string
}

func parseTimelineSetFlags(args []string) (*timelineSetFlags, error) {
	f := &timelineSetFlags{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--episode-id":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--episode-id requires a value")
			}
			i++
			f.episodeID = args[i]
		case "--show-id":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--show-id requires a value")
			}
			i++
			f.showID = args[i]
		case "--from-file":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--from-file requires a value")
			}
			i++
			f.fromFile = args[i]
		default:
			return nil, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if f.episodeID == "" {
		return nil, fmt.Errorf("missing required flag: --episode-id")
	}
	if f.fromFile == "" {
		return nil, fmt.Errorf("missing required flag: --from-file")
	}

	return f, nil
}

func printTimelineSetUsage() {
	fmt.Printf(`Usage: %s timeline set [flags]

Set (replace all) timeline items on an episode.

Required flags:
  --episode-id <id>    Episode ID or URI
  --from-file <path>   JSON file containing {"items": [...]}

Optional flags:
  --show-id <id>       Show ID or URI (default: last created show)

Each item in the JSON must be exactly one of: chapter, image, link,
or spotify_entity. Prefer including spotify_entity when the referenced
thing already exists on Spotify. Links and Spotify entities can both
appear in the same chapter/segment if both are useful. Prefer full
spotify:... URI format over bare IDs or open.spotify.com URLs.

Example timeline.json:
  {
    "items": [
      {"chapter": {"title": "Introduction", "start_time_ms": 0}},
      {"chapter": {"title": "Main Topic", "start_time_ms": 60000}},
      {"image": {"start_time_ms": 75000, "duration_ms": 30000, "image": "book-cover.jpg", "url": "https://example.com/book", "title": "Get the Book"}},
      {"spotify_entity": {"start_time_ms": 120000, "duration_ms": 45000, "uri": "spotify:track:abc123"}},
      {"spotify_entity": {"start_time_ms": 170000, "uri": "spotify:episode:def456"}},
      {"link": {"start_time_ms": 180000, "duration_ms": 15000, "url": "https://sponsor.example.com"}},
      {"chapter": {"title": "Wrap Up", "start_time_ms": 300000}}
    ]
  }

Rules:
  Chapters: optional; if present, ≥ 2 required, first at 0ms, strictly increasing, ≥ 5s apart, title required
            Final chapter may be shorter than 5s; only non-final chapter gaps must be ≥ 5s
            Note: PCA rejects timelines where too many chapters are < 30s (limit: ceil(N*0.15+1))
  Images:   duration_ms > 0, image file required (.jpg/.png, max 1 MB)
            width/height are auto-read for local files; required with image_token
  Links:    duration_ms > 0, valid HTTP(S) URL required
  Spotify entities: full spotify:... URI required, duration_ms optional but must be > 0 when set
                    Use for Spotify-native references like tracks, albums, artists,
                    playlists, shows, episodes, and other catalog/entity URIs
                    A link and a spotify_entity can both be present; they just must not overlap
  Companion content (images + links + spotify entities) must not overlap in time

Examples:
  %s timeline set --episode-id spotify:episode:abc123 --from-file timeline.json
  %s --json timeline set --episode-id abc123 --from-file tl.json | jq '.items | length'
`, binName, binName, binName)
}

// --- helpers ---

const maxImageDimension = 4096

const minChapterDurationMs = 5000

const shortChapterThresholdMs = 30000

// imageSize decodes width and height from an image file without reading the full image into memory.
func imageSize(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

// --- validation ---

func isHTTPURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

func isSpotifyURI(u string) bool {
	if !strings.HasPrefix(u, "spotify:") {
		return false
	}
	parts := strings.Split(u, ":")
	if len(parts) < 3 {
		return false
	}
	for _, part := range parts[1:] {
		if part == "" {
			return false
		}
	}
	return true
}

func validateTimeline(items []timelineItem) error {
	if len(items) == 0 {
		return fmt.Errorf("timeline must have at least one item")
	}

	// Per-item validation + collect chapters and companions
	var chapters []timelineChapter
	var chapterIndices []int

	type companionTiming struct {
		startMs int
		endMs   int
		index   int
	}
	var companions []companionTiming

	for i, item := range items {
		count := 0
		if item.Chapter != nil {
			count++
		}
		if item.Image != nil {
			count++
		}
		if item.Link != nil {
			count++
		}
		if item.SpotifyEntity != nil {
			count++
		}
		if count == 0 {
			return fmt.Errorf("item at index %d must have exactly one of chapter, image, link, or spotify_entity", i)
		}
		if count > 1 {
			return fmt.Errorf("item at index %d must have exactly one of chapter, image, link, or spotify_entity (found %d)", i, count)
		}

		if item.Chapter != nil {
			ch := *item.Chapter
			if ch.StartTimeMs < 0 {
				return fmt.Errorf("chapter at index %d: start_time_ms must be non-negative", i)
			}
			if ch.Title == "" {
				return fmt.Errorf("chapter at index %d has an empty title", i)
			}
			chapters = append(chapters, ch)
			chapterIndices = append(chapterIndices, i)
		}

		if item.Image != nil {
			img := *item.Image
			if img.StartTimeMs < 0 {
				return fmt.Errorf("image at index %d: start_time_ms must be non-negative", i)
			}
			if img.DurationMs <= 0 {
				return fmt.Errorf("image at index %d: duration_ms must be positive", i)
			}
			if img.Image == "" && img.ImageToken == "" {
				return fmt.Errorf("image at index %d: image file path is required", i)
			}
			if img.Image == "" {
				if img.Width <= 0 || img.Height <= 0 {
					return fmt.Errorf("image at index %d: width and height are required when using image_token", i)
				}
			} else {
				if img.Width < 0 || img.Height < 0 {
					return fmt.Errorf("image at index %d: width and height must be positive when provided", i)
				}
			}
			if img.URL != "" && !isHTTPURL(img.URL) {
				return fmt.Errorf("image at index %d: url must be an HTTP(S) URL", i)
			}
			companions = append(companions, companionTiming{
				startMs: img.StartTimeMs,
				endMs:   img.StartTimeMs + img.DurationMs,
				index:   i,
			})
		}

		if item.Link != nil {
			lnk := *item.Link
			if lnk.StartTimeMs < 0 {
				return fmt.Errorf("link at index %d: start_time_ms must be non-negative", i)
			}
			if lnk.DurationMs <= 0 {
				return fmt.Errorf("link at index %d: duration_ms must be positive", i)
			}
			if lnk.URL == "" || !isHTTPURL(lnk.URL) {
				return fmt.Errorf("link at index %d: url must be a valid HTTP(S) URL", i)
			}
			companions = append(companions, companionTiming{
				startMs: lnk.StartTimeMs,
				endMs:   lnk.StartTimeMs + lnk.DurationMs,
				index:   i,
			})
		}

		if item.SpotifyEntity != nil {
			entity := *item.SpotifyEntity
			if entity.StartTimeMs < 0 {
				return fmt.Errorf("spotify_entity at index %d: start_time_ms must be non-negative", i)
			}
			if entity.DurationMs != nil && *entity.DurationMs <= 0 {
				return fmt.Errorf("spotify_entity at index %d: duration_ms must be positive when provided", i)
			}
			if entity.URI == "" {
				return fmt.Errorf("spotify_entity at index %d: uri is required", i)
			}
			if !isSpotifyURI(entity.URI) {
				return fmt.Errorf("spotify_entity at index %d: uri must be a full Spotify URI like spotify:<type>:<id>", i)
			}

			endMs := entity.StartTimeMs
			if entity.DurationMs != nil {
				endMs += *entity.DurationMs
			}
			companions = append(companions, companionTiming{
				startMs: entity.StartTimeMs,
				endMs:   endMs,
				index:   i,
			})
		}
	}

	// Chapter-specific validation
	if len(chapters) > 0 && len(chapters) < 2 {
		return fmt.Errorf("at least 2 chapters are required (found %d)", len(chapters))
	}
	if len(chapters) > 0 && chapters[0].StartTimeMs != 0 {
		return fmt.Errorf("first chapter must start at 0 ms")
	}
	for i := 1; i < len(chapters); i++ {
		if chapters[i].StartTimeMs <= chapters[i-1].StartTimeMs {
			return fmt.Errorf("chapters must have strictly increasing start_time_ms (duplicate or unsorted at index %d)", chapterIndices[i])
		}
		if i < len(chapters)-1 && chapters[i].StartTimeMs-chapters[i-1].StartTimeMs < minChapterDurationMs {
			return fmt.Errorf("chapter at index %d must be at least %ds long (found %dms between start times)", chapterIndices[i-1], minChapterDurationMs/1000, chapters[i].StartTimeMs-chapters[i-1].StartTimeMs)
		}
	}

	if len(chapters) >= 2 {
		shortCount := 0
		for i := 1; i < len(chapters); i++ {
			if chapters[i].StartTimeMs-chapters[i-1].StartTimeMs < shortChapterThresholdMs {
				shortCount++
			}
		}
		maxShort := int(math.Ceil(float64(len(chapters))*0.15 + 1))
		if shortCount > maxShort {
			return fmt.Errorf("too many short chapters (< 30s): %d found, max allowed is %d for %d chapters", shortCount, maxShort, len(chapters))
		}
	}

	// Companion overlap check
	if len(companions) > 1 {
		sort.Slice(companions, func(i, j int) bool {
			return companions[i].startMs < companions[j].startMs
		})
		for i := 1; i < len(companions); i++ {
			if companions[i-1].endMs > companions[i].startMs {
				return fmt.Errorf("companion content overlaps: item at index %d ending at %dms overlaps with item at index %d starting at %dms", companions[i-1].index, companions[i-1].endMs, companions[i].index, companions[i].startMs)
			}
		}
	}

	return nil
}

// --- handlers ---

func handleTimelineSet(args []string) error {
	if len(args) > 0 && isHelp(args[0]) {
		printTimelineSetUsage()
		return nil
	}

	flags, err := parseTimelineSetFlags(args)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(flags.fromFile)
	if err != nil {
		return fmt.Errorf("failed to read timeline file: %w", err)
	}

	var tl timelineFile
	if err := json.Unmarshal(data, &tl); err != nil {
		return fmt.Errorf("failed to parse timeline file: %w", err)
	}

	if err := validateTimeline(tl.Items); err != nil {
		return err
	}

	token, err := getValidToken()
	if err != nil {
		return err
	}

	episodeID := strings.TrimPrefix(flags.episodeID, "spotify:episode:")

	showID, err := resolveShowIDForEpisode(token, episodeID, flags.showID)
	if err != nil {
		return err
	}

	jsonDir := filepath.Dir(flags.fromFile)
	for i := range tl.Items {
		if tl.Items[i].Image == nil || tl.Items[i].Image.Image == "" {
			continue
		}
		img := tl.Items[i].Image
		imgPath := img.Image
		if !filepath.IsAbs(imgPath) {
			imgPath = filepath.Join(jsonDir, imgPath)
		}

		w, h, err := imageSize(imgPath)
		if err != nil {
			return fmt.Errorf("failed to read image dimensions at index %d (%s): %w", i, img.Image, err)
		}
		if w < 1 || h < 1 || w > maxImageDimension || h > maxImageDimension {
			return fmt.Errorf("image at index %d (%s): dimensions %dx%d out of range (must be between 1x1 and %dx%d)", i, img.Image, w, h, maxImageDimension, maxImageDimension)
		}
		img.Width = w
		img.Height = h

		uploadToken, err := uploadImage(token, imgPath)
		if err != nil {
			return fmt.Errorf("failed to upload image at index %d (%s): %w", i, img.Image, err)
		}
		img.ImageToken = uploadToken
		img.Image = "" // clear local path before sending to backend
	}

	body, err := json.Marshal(tl)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := config.BackendURLPath("shows", showID, "episodes", episodeID, "timeline")
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "PUT", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Setting timeline")
	resp, err := doAPIRequest(req, token)
	ab.stop(err == nil && resp != nil && isSuccessStatus(resp.StatusCode))
	if err != nil {
		return fmt.Errorf("request failed — check your connectivity: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if !isSuccessStatus(resp.StatusCode) {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result timelineResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if config.JSONMode() {
		return printJSON(result)
	}

	printTimelineHuman(result, episodeID)
	return nil
}

func handleTimelineGet(id string, extraArgs []string) error {
	for _, a := range extraArgs {
		if isHelp(a) {
			fmt.Printf("Usage: %s timeline get <id> [--show-id <id>]\n\nGet timeline (chapters + companion content, including Spotify entity cards) for an episode.\nUse spotify_entity for Spotify-native references, link for external destinations, or both when both are useful. Prefer full spotify:... URIs.\n\nFlags:\n  --show-id <id>  Show ID or URI (default: last created show)\n\nUse --json for structured output.\n", binName)
			return nil
		}
		if strings.HasPrefix(a, "-") && a != "--show-id" {
			return fmt.Errorf("unknown flag: %s", a)
		}
	}

	token, err := getValidToken()
	if err != nil {
		return err
	}

	episodeID := strings.TrimPrefix(id, "spotify:episode:")

	showID, err := resolveShowIDForEpisode(token, episodeID, parseShowIDFlag(extraArgs))
	if err != nil {
		return err
	}

	url, err := config.BackendURLPath("shows", showID, "episodes", episodeID, "timeline")
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Fetching timeline")
	resp, err := doAPIRequest(req, token)
	ab.stop(err == nil && resp != nil && isSuccessStatus(resp.StatusCode))
	if err != nil {
		return fmt.Errorf("request failed — check your connectivity: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if !isSuccessStatus(resp.StatusCode) {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result timelineResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if config.JSONMode() {
		return printJSON(result)
	}

	if len(result.Items) == 0 {
		fmt.Println("No timeline items found for this episode.")
		return nil
	}

	printTimelineHuman(result, episodeID)
	return nil
}

func handleTimelineDelete(id string, extraArgs []string) error {
	for _, a := range extraArgs {
		if isHelp(a) {
			fmt.Printf("Usage: %s timeline delete <id> [--show-id <id>]\n\nDelete all timeline items (chapters + companion content, including Spotify entity cards) for an episode.\n\nFlags:\n  --show-id <id>  Show ID or URI (default: last created show)\n\nUse --json for structured output.\n", binName)
			return nil
		}
		if strings.HasPrefix(a, "-") && a != "--show-id" {
			return fmt.Errorf("unknown flag: %s", a)
		}
	}

	token, err := getValidToken()
	if err != nil {
		return err
	}

	episodeID := strings.TrimPrefix(id, "spotify:episode:")

	showID, err := resolveShowIDForEpisode(token, episodeID, parseShowIDFlag(extraArgs))
	if err != nil {
		return err
	}

	url, err := config.BackendURLPath("shows", showID, "episodes", episodeID, "timeline")
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Deleting timeline")
	resp, err := doAPIRequest(req, token)
	ab.stop(err == nil && resp != nil && isSuccessStatus(resp.StatusCode))
	if err != nil {
		return fmt.Errorf("request failed — check your connectivity: %w", err)
	}
	defer resp.Body.Close()

	if !isSuccessStatus(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	if config.JSONMode() {
		return printJSON(map[string]any{"ok": true, "episode_id": episodeID})
	}

	fmt.Printf("Timeline deleted for episode %s.\n", episodeID)
	return nil
}

// --- human-readable output ---

func printTimelineHuman(result timelineResponse, episodeID string) {
	var chapters []timelineResponseChapter
	var images []timelineResponseImage
	var links []timelineResponseLink
	var spotifyEntities []timelineResponseSpotifyEntity

	for _, item := range result.Items {
		if item.Chapter != nil {
			chapters = append(chapters, *item.Chapter)
		}
		if item.Image != nil {
			images = append(images, *item.Image)
		}
		if item.Link != nil {
			links = append(links, *item.Link)
		}
		if item.SpotifyEntity != nil {
			spotifyEntities = append(spotifyEntities, *item.SpotifyEntity)
		}
	}

	fmt.Printf("Timeline for episode %s:\n", episodeID)

	if len(chapters) > 0 {
		sort.Slice(chapters, func(i, j int) bool {
			return chapters[i].StartTimeMs < chapters[j].StartTimeMs
		})
		fmt.Println("\nChapters:")
		for i, ch := range chapters {
			fmt.Printf("  %d. %s (%dms)\n", i+1, ch.Title, ch.StartTimeMs)
			if ch.Description != "" {
				fmt.Printf("     %s\n", ch.Description)
			}
		}
	}

	if len(images) > 0 {
		sort.Slice(images, func(i, j int) bool {
			return images[i].StartTimeMs < images[j].StartTimeMs
		})
		fmt.Println("\nImages:")
		for _, img := range images {
			title := img.Title
			if title == "" {
				title = "(untitled)"
			}
			fmt.Printf("  %s at %dms for %dms\n", title, img.StartTimeMs, img.DurationMs)
			fmt.Printf("    URI: %s\n", img.CompanionURI)
			if img.Width > 0 && img.Height > 0 {
				fmt.Printf("    Dimensions: %dx%d\n", img.Width, img.Height)
			}
		}
	}

	if len(links) > 0 {
		sort.Slice(links, func(i, j int) bool {
			return links[i].StartTimeMs < links[j].StartTimeMs
		})
		fmt.Println("\nLinks:")
		for _, lnk := range links {
			fmt.Printf("  Link at %dms for %dms\n", lnk.StartTimeMs, lnk.DurationMs)
			fmt.Printf("    URI: %s\n", lnk.CompanionURI)
		}
	}

	if len(spotifyEntities) > 0 {
		sort.Slice(spotifyEntities, func(i, j int) bool {
			return spotifyEntities[i].StartTimeMs < spotifyEntities[j].StartTimeMs
		})
		fmt.Println("\nSpotify References:")
		for _, entity := range spotifyEntities {
			if entity.DurationMs != nil {
				fmt.Printf("  %s at %dms for %dms\n", entity.URI, entity.StartTimeMs, *entity.DurationMs)
			} else {
				fmt.Printf("  %s at %dms\n", entity.URI, entity.StartTimeMs)
			}
			fmt.Printf("    URI: %s\n", entity.CompanionURI)
		}
	}
}
