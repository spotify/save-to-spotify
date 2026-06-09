package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

const defaultEpisodeReadinessWaitTimeout = 5 * time.Minute

var episodeReadinessPollInterval = 20 * time.Second

func printEpisodesUsage() {
	fmt.Printf(`Usage: %s episodes [command]

Commands:
  (none)          List episodes created via CLI
  create          Create an episode
  status <id>     Check if an episode is ready for playback
  delete <id>     Delete an episode

Note: Episode metadata is immutable after creation. To modify an episode,
delete it and recreate it with the desired fields.

Flags (for list):
  --show-id <id>  Show ID or URI (default: last show)
`, binName)
}

func handleEpisodes(args []string) error {
	if len(args) == 0 {
		return handleListEpisodes(nil)
	}
	if isHelp(args[0]) {
		printEpisodesUsage()
		return nil
	}
	// Flags without a subcommand are list filters (e.g. `episodes --show-id X`).
	if strings.HasPrefix(args[0], "-") {
		return handleListEpisodes(args)
	}

	switch args[0] {
	case "create":
		return handleEpisodesCreate(args[1:])
	case "status":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %[1]s episodes status <id> [--wait [<duration>]]\n       %[1]s episodes status --episode-id <id> [--wait [<duration>]]\n\nCheck if an episode is ready for playback.\n\nFlags:\n  --episode-id <id>       Episode ID or URI (alternative to positional argument)\n  --wait [<duration>]     Poll until the episode becomes READY; optionally override the timeout (default: %s)\n", binName, defaultEpisodeReadinessWaitTimeout)
			return nil
		}
		return handleEpisodesReadiness(args[1:])
	case "delete":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s episodes delete <id>\n\nDelete an episode.\n", binName)
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: %s episodes delete <id>", binName)
		}
		return handleEpisodesDelete(args[1], args[2:])
	default:
		return fmt.Errorf("unknown episodes subcommand: %s", args[0])
	}
}

// EpisodeSummary represents an episode returned by the list episodes API.
type EpisodeSummary struct {
	EpisodeURI string `json:"episode_uri"`
	Title      string `json:"title"`
	Language   string `json:"language"`
	MediaType  string `json:"media_type"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

type listEpisodesResponse struct {
	Episodes []EpisodeSummary `json:"episodes"`
}

// listEpisodes fetches all episodes for a show from the backend API.
func listEpisodes(token *config.TokenData, showID string) ([]EpisodeSummary, error) {
	showID = strings.TrimPrefix(showID, "spotify:show:")
	url, err := config.BackendURLPath("shows", showID, "episodes")
	if err != nil {
		return nil, fmt.Errorf("failed to build request URL: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := doAPIRequest(req, token)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if !isSuccessStatus(resp.StatusCode) {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result listEpisodesResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Episodes, nil
}

// resolveShowIDForEpisode finds the show ID for a given episode ID.
// showIDOverride takes precedence. Otherwise, searches the user's shows via API.
func resolveShowIDForEpisode(token *config.TokenData, episodeID, showIDOverride string) (string, error) {
	if showIDOverride != "" {
		return strings.TrimPrefix(showIDOverride, "spotify:show:"), nil
	}

	episodeURI := episodeID
	if !strings.HasPrefix(episodeURI, "spotify:episode:") {
		episodeURI = "spotify:episode:" + episodeID
	}

	shows, err := listShows(token)
	if err != nil {
		return "", fmt.Errorf("--show-id is required (could not list shows): %w", err)
	}

	for _, show := range shows {
		showID := strings.TrimPrefix(show.ShowURI, "spotify:show:")
		episodes, err := listEpisodes(token, showID)
		if err != nil {
			continue
		}
		for _, ep := range episodes {
			if ep.EpisodeURI == episodeURI {
				return showID, nil
			}
		}
	}

	return "", fmt.Errorf("--show-id is required: episode %s not found in any of your shows", episodeID)
}

// parseShowIDFlag extracts the value of --show-id from a list of extra args.
func parseShowIDFlag(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--show-id" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

type episodeCreateFlags struct {
	showID   string
	title    string
	filePath string
	summary  string
	image    string // local image file path (.jpg/.png, max 1 MB)
	language string
}

func parseEpisodeCreateFlags(args []string) (*episodeCreateFlags, error) {
	f := &episodeCreateFlags{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--show-id":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--show-id requires a value")
			}
			i++
			f.showID = args[i]
		case "--title":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--title requires a value")
			}
			i++
			f.title = args[i]
		case "--file":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--file requires a value")
			}
			i++
			f.filePath = args[i]
		case "--summary":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--summary requires a value")
			}
			i++
			f.summary = args[i]
		case "--image":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--image requires a file path")
			}
			i++
			f.image = args[i]
		case "--language":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--language requires a value")
			}
			i++
			f.language = args[i]
		default:
			return nil, fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	// Validate required flags
	var missing []string
	if f.title == "" {
		missing = append(missing, "--title")
	}
	if f.filePath == "" {
		missing = append(missing, "--file")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required flags: %s", strings.Join(missing, ", "))
	}

	return f, nil
}

type episodeCreateRequest struct {
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Language   string `json:"language"`
	MediaType  string `json:"media_type"`
	ImageToken string `json:"image_token,omitempty"`
}

type multipartUploadURL struct {
	SignedURL  string `json:"signed_url"`
	PartNumber int    `json:"part_number"`
}

type episodeCreateResponse struct {
	EpisodeURI          string               `json:"episode_uri"`
	CusObjectId         string               `json:"cus_object_id"`
	MultipartUploadURLs []multipartUploadURL `json:"multipart_upload_urls"`
	Status              string               `json:"status"`
}

func printEpisodesCreateUsage() {
	fmt.Printf(`Usage: %s episodes create [flags]

Create an episode.

Required flags:
  --title <title>            Episode title
  --file <path>              Local audio file to upload

Optional flags:
  --show-id <id>             Show ID or URI (default: last created show)
  --summary <text>           Episode description
  --image <path>             Cover image file (.jpg/.png, max 1 MB)
  --language <code>          Language code (default: en)

Examples:
  save-to-spotify episodes create --title "Ep 1" --file audio.mp3 --summary "First episode"
  save-to-spotify --json episodes create --title "Ep 1" --file audio.mp3 --summary "First episode" | jq .episode_uri
`, binName)
}

func handleEpisodesCreate(args []string) error {
	if len(args) > 0 && isHelp(args[0]) {
		printEpisodesCreateUsage()
		return nil
	}

	flags, err := parseEpisodeCreateFlags(args)
	if err != nil {
		return err
	}

	if err := validateMediaFile(flags.filePath); err != nil {
		return err
	}

	token, err := getValidToken()
	if err != nil {
		return err
	}

	// Default show ID from API
	if flags.showID == "" {
		show, err := lastShow(token)
		if err != nil {
			return fmt.Errorf("could not default to last show (pass --show-id to override): %w", err)
		}
		flags.showID = show.ShowURI
		info("Using show: %s (%s)\n", show.ShowURI, show.Title)
	}

	showID := strings.TrimPrefix(flags.showID, "spotify:show:")

	language := flags.language
	if language == "" {
		language = "en"
	}

	imageToken, err := uploadImage(token, flags.image)
	if err != nil {
		return err
	}

	reqBody := episodeCreateRequest{
		Title:      flags.title,
		Summary:    flags.summary,
		Language:   language,
		MediaType:  mediaTypeForFile(flags.filePath),
		ImageToken: imageToken,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := config.BackendURLPath("shows", showID, "episodes")
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Creating episode")
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

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result episodeCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.MultipartUploadURLs) == 0 {
		return fmt.Errorf("episode created but no upload URLs returned")
	}

	fileInfo, err := os.Stat(flags.filePath)
	if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}
	contentType := mediaContentType(flags.filePath)
	if err := uploadMultipart(result.MultipartUploadURLs, contentType, flags.filePath, fileInfo.Size()); err != nil {
		return err
	}

	if config.JSONMode() {
		return printJSON(result)
	}

	fmt.Println("Episode created successfully.")
	fmt.Printf("  URI:    %s\n", result.EpisodeURI)
	fmt.Printf("  Title:  %s\n", flags.title)
	fmt.Printf("  Status: %s\n", result.Status)

	return nil
}

type episodeReadinessResponse struct {
	EpisodeURI string `json:"episode_uri"`
	Readiness  string `json:"readiness"`
}

type episodeReadinessFlags struct {
	episodeID   string
	wait        bool
	waitTimeout time.Duration
}

func parseEpisodeReadinessFlags(args []string) (*episodeReadinessFlags, error) {
	f := &episodeReadinessFlags{waitTimeout: defaultEpisodeReadinessWaitTimeout}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "--wait=") {
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--wait="))
			if err != nil || d <= 0 {
				return nil, fmt.Errorf("invalid --wait value: %s (examples: --wait 30s, --wait=2m)", arg)
			}
			f.wait = true
			f.waitTimeout = d
			continue
		}

		switch arg {
		case "--episode-id":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--episode-id requires a value")
			}
			i++
			if f.episodeID != "" {
				return nil, fmt.Errorf("episode ID provided more than once")
			}
			f.episodeID = args[i]
		case "--wait":
			f.wait = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				d, err := time.ParseDuration(args[i+1])
				if err == nil && d > 0 {
					i++
					f.waitTimeout = d
				} else if f.episodeID != "" {
					return nil, fmt.Errorf("invalid --wait value: %s (examples: --wait 30s, --wait=2m)", args[i+1])
				}
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return nil, fmt.Errorf("unknown flag: %s", args[i])
			}
			if f.episodeID != "" {
				return nil, fmt.Errorf("episode ID provided more than once")
			}
			f.episodeID = args[i]
		}
	}

	if f.episodeID == "" {
		return nil, fmt.Errorf("usage: %s episodes status <episode_id>", binName)
	}

	return f, nil
}

func fetchEpisodeReadiness(token *config.TokenData, episodeID string) (*episodeReadinessResponse, error) {
	episodeID = strings.TrimPrefix(episodeID, "spotify:episode:")

	url, err := config.BackendURLPath("episodes", episodeID, "readiness")
	if err != nil {
		return nil, fmt.Errorf("failed to build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := doAPIRequest(req, token)
	if err != nil {
		return nil, fmt.Errorf("request failed — check your connectivity: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	var result episodeReadinessResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &result, nil
}

func handleEpisodesReadiness(args []string) error {
	flags, err := parseEpisodeReadinessFlags(args)
	if err != nil {
		return err
	}

	token, err := getValidToken()
	if err != nil {
		return err
	}

	var result *episodeReadinessResponse
	if flags.wait {
		result, err = waitForEpisodeReadiness(token, flags.episodeID, flags.waitTimeout)
		if err != nil {
			return err
		}
	} else {
		ab := startActivity("Checking episode status")
		result, err = fetchEpisodeReadiness(token, flags.episodeID)
		ab.stop(err == nil)
		if err != nil {
			return err
		}
	}

	if config.JSONMode() {
		return printJSON(result)
	}

	fmt.Printf("Episode: %s\n", result.EpisodeURI)
	fmt.Printf("Readiness: %s\n", result.Readiness)

	return nil
}

func waitForEpisodeReadiness(token *config.TokenData, episodeID string, waitTimeout time.Duration) (*episodeReadinessResponse, error) {
	deadline := time.Now().Add(waitTimeout)
	info("Waiting for episode readiness (timeout: %s, polling every %s)\n", waitTimeout, episodeReadinessPollInterval)

	var lastReadiness string
	for {
		result, err := fetchEpisodeReadiness(token, episodeID)
		if err != nil {
			return nil, err
		}
		if result.Readiness != lastReadiness {
			info("Readiness: %s\n", result.Readiness)
			lastReadiness = result.Readiness
		}

		switch result.Readiness {
		case "READY":
			return result, nil
		case "FAILED":
			return nil, fmt.Errorf("episode processing failed")
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timed out waiting for episode readiness after %s (last readiness: %s)", waitTimeout, result.Readiness)
		}

		sleepFor := episodeReadinessPollInterval
		if remaining < sleepFor {
			sleepFor = remaining
		}
		time.Sleep(sleepFor)
	}
}

// --- Episode Delete ---

func handleEpisodesDelete(episodeID string, extraArgs []string) error {
	if len(extraArgs) > 0 {
		return fmt.Errorf("unknown argument: %s", extraArgs[0])
	}

	episodeID = strings.TrimPrefix(episodeID, "spotify:episode:")

	token, err := getValidToken()
	if err != nil {
		return err
	}

	url, err := config.BackendURLPath("episodes", episodeID)
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Deleting episode")
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
		return printJSON(map[string]string{"status": "deleted", "episode_id": episodeID})
	}

	fmt.Printf("Episode %s deleted successfully.\n", episodeID)
	return nil
}
