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

	"github.com/spotify/save-to-spotify/config"
)

const defaultShowTitle = "My Personal Podcast"

type uploadFlags struct {
	filePath string
	title    string
	showID   string
	newShow  string // non-empty = create a new show with this title
	summary  string
	image    string // local image file path (.jpg/.png, max 1 MB)
	language string
}

func parseUploadFlags(args []string) (*uploadFlags, error) {
	f := &uploadFlags{}

	if len(args) == 0 {
		return nil, fmt.Errorf("usage: %s upload <file> --title <title> [flags]", binName)
	}

	// First non-flag arg is the positional file arg
	if strings.HasPrefix(args[0], "-") {
		return nil, fmt.Errorf("usage: %s upload <file> --title <title> [flags]", binName)
	}
	f.filePath = args[0]

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--title requires a value")
			}
			i++
			f.title = args[i]
		case "--show-id":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--show-id requires a value")
			}
			i++
			f.showID = args[i]
		case "--summary":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--summary requires a value")
			}
			i++
			f.summary = args[i]
		case "--new-show":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--new-show requires a show title")
			}
			i++
			f.newShow = args[i]
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

	if f.title == "" {
		return nil, fmt.Errorf("missing required flag: --title")
	}

	if f.newShow != "" && f.showID != "" {
		return nil, fmt.Errorf("--new-show and --show-id are mutually exclusive")
	}

	return f, nil
}

func printUploadUsage() {
	fmt.Printf(`Usage: %s upload <file> --title <title> [flags]

Upload a media file and create an episode.

Positional:
  <file>                  Path to a local audio file

Required flags:
  --title <title>         Episode title

Optional flags:
  --show-id <id>          Target show URI or ID (default: last created show)
  --new-show <title>      Create a new show with this title
  --summary <text>        Episode description
  --image <path>          Cover image file (.jpg/.png, max 1 MB)
  --language <code>       Language code (default: en)

Examples:
  save-to-spotify upload episode.mp3 --title "My First Episode"
  save-to-spotify upload episode.mp3 --title "Ep 2" --show-id spotify:show:abc123
  save-to-spotify --json upload ep.mp3 --title "Ep 3" | jq .episode_uri
`, binName)
}

func handleUpload(args []string) error {
	for _, a := range args {
		if isHelp(a) {
			printUploadUsage()
			return nil
		}
	}

	flags, err := parseUploadFlags(args)
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

	// 1. Resolve or create show
	showID, err := resolveOrCreateShow(flags, token)
	if err != nil {
		return err
	}

	showIDClean := strings.TrimPrefix(showID, "spotify:show:")

	// 2. Create episode (returns multipart upload URLs)
	fileInfo, err := os.Stat(flags.filePath)
	if err != nil {
		return fmt.Errorf("cannot access file: %w", err)
	}

	imageToken, err := uploadImage(token, flags.image)
	if err != nil {
		return err
	}

	summary := flags.summary
	if summary == "" {
		summary = "(no description)"
	}

	language := flags.language
	if language == "" {
		language = "en"
	}

	reqBody := episodeCreateRequest{
		Title:      flags.title,
		Summary:    summary,
		Language:   language,
		MediaType:  mediaTypeForFile(flags.filePath),
		ImageToken: imageToken,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := config.BackendURLPath("shows", showIDClean, "episodes")
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

	if !isSuccessStatus(resp.StatusCode) {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result episodeCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	// 3. Upload file via multipart
	if len(result.MultipartUploadURLs) == 0 {
		return fmt.Errorf("episode created but no upload URLs returned")
	}
	contentType := mediaContentType(flags.filePath)
	if err := uploadMultipart(result.MultipartUploadURLs, contentType, flags.filePath, fileInfo.Size()); err != nil {
		return err
	}

	if config.JSONMode() {
		return printJSON(struct {
			EpisodeURI string `json:"episode_uri"`
			Title      string `json:"title"`
			Status     string `json:"status"`
		}{result.EpisodeURI, flags.title, result.Status})
	}

	fmt.Println("Episode created successfully.")
	fmt.Printf("  URI:    %s\n", result.EpisodeURI)
	fmt.Printf("  Title:  %s\n", flags.title)
	fmt.Printf("  Status: %s\n", result.Status)

	return nil
}

// resolveOrCreateShow resolves the target show from --show-id, the API, or auto-creates one.
// Returns the show ID (with or without URI prefix).
func resolveOrCreateShow(flags *uploadFlags, token *config.TokenData) (string, error) {
	// 1. --new-show forces creation
	if flags.newShow != "" {
		return createShow(flags, token)
	}

	// 2. Explicit --show-id
	if flags.showID != "" {
		// Look up title from API if possible
		showURI := flags.showID
		if !strings.HasPrefix(showURI, "spotify:show:") {
			showURI = "spotify:show:" + showURI
		}
		var title string
		shows, err := listShows(token)
		if err == nil {
			for _, s := range shows {
				if s.ShowURI == showURI {
					title = s.Title
					break
				}
			}
		}
		if title != "" {
			info("Using show: %s (%s)\n", showURI, title)
		} else {
			info("Using show: %s\n", showURI)
		}
		return flags.showID, nil
	}

	// 3. Last show from API
	last, err := lastShow(token)
	if err == nil {
		info("Using show: %s (%s)\n", last.ShowURI, last.Title)
		return last.ShowURI, nil
	}

	// 4. Auto-create show
	return createShow(flags, token)
}

// createShow creates a new show via the backend API.
func createShow(flags *uploadFlags, token *config.TokenData) (string, error) {
	title := flags.newShow
	if title == "" {
		title = defaultShowTitle
	}

	imageToken, err := uploadImage(token, flags.image)
	if err != nil {
		return "", err
	}

	lang := flags.language
	if lang == "" {
		lang = "en"
	}

	reqBody := showCreateRequest{
		Title:      title,
		Summary:    "(no description)",
		Language:   lang,
		ImageToken: imageToken,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := config.BackendURLPath("shows")
	if err != nil {
		return "", fmt.Errorf("failed to build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Creating show")
	resp, err := doAPIRequest(req, token)
	ab.stop(err == nil && resp != nil && isSuccessStatus(resp.StatusCode))
	if err != nil {
		return "", fmt.Errorf("request failed — check your connectivity: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if !isSuccessStatus(resp.StatusCode) {
		return "", fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var result showCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	info("Show created: %s (%s)\n", result.ShowURI, title)

	return result.ShowURI, nil
}
