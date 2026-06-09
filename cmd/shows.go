package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spotify/save-to-spotify/config"
)

// ShowSummary represents a show returned by the list shows API.
type ShowSummary struct {
	ShowURI               string `json:"show_uri"`
	Title                 string `json:"title"`
	Language              string `json:"language"`
	CreatedAt             string `json:"created_at"`
	LastEpisodeUploadedAt string `json:"last_episode_uploaded_at,omitempty"`
}

type listShowsResponse struct {
	Shows []ShowSummary `json:"shows"`
}

// listShows fetches all shows for the authenticated user from the backend API.
func listShows(token *config.TokenData) ([]ShowSummary, error) {
	url, err := config.BackendURLPath("shows")
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

	var result listShowsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Shows, nil
}

// lastShow returns the most recently created show, or an error if none exist.
func lastShow(token *config.TokenData) (*ShowSummary, error) {
	shows, err := listShows(token)
	if err != nil {
		return nil, err
	}
	if len(shows) == 0 {
		return nil, fmt.Errorf("no shows found — create one first with 'shows create'")
	}
	return &shows[len(shows)-1], nil
}

func printShowsUsage() {
	fmt.Printf(`Usage: %s shows [command]

Commands:
  (none)       List shows created via CLI
  create       Create a new show
  get <id>     Get a show by ID
  delete <id>  Delete a show

Note: Show metadata is immutable after creation. To modify a show,
delete it and recreate it with the desired fields.
`, binName)
}

func handleShows(args []string) error {
	if len(args) == 0 {
		return handleShowsList()
	}

	switch args[0] {
	case "create":
		return handleShowsCreate(args[1:])
	case "get":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s shows get <id>\n\nGet a show by ID.\n", binName)
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: %s shows get <id>", binName)
		}
		return handleShowsGet(args[1])
	case "delete":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s shows delete <id>\n\nDelete a show and all its episodes.\n", binName)
			return nil
		}
		if len(args) < 2 {
			return fmt.Errorf("usage: %s shows delete <id>", binName)
		}
		return handleShowsDelete(args[1])
	case "-h", "--help", "help":
		printShowsUsage()
		return nil
	default:
		return fmt.Errorf("unknown shows subcommand: %s", args[0])
	}
}

func handleShowsList() error {
	token, err := getValidToken()
	if err != nil {
		return err
	}

	shows, err := listShows(token)
	if err != nil {
		return err
	}

	if config.JSONMode() {
		return printJSON(map[string]any{"shows": shows})
	}

	if len(shows) == 0 {
		fmt.Println("No shows created yet. Use 'shows create' to create one.")
		return nil
	}

	fmt.Println("Shows:")
	fmt.Println()
	for _, s := range shows {
		fmt.Printf("  %-40s  %-20s  %s\n", s.ShowURI, s.Title, s.CreatedAt)
	}

	return nil
}

type showCreateFlags struct {
	title    string
	summary  string
	image    string // local image file path (.jpg/.png, max 1 MB)
	language string
}

func parseShowCreateFlags(args []string) (*showCreateFlags, error) {
	f := &showCreateFlags{
		language: "en",
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--title requires a value")
			}
			i++
			f.title = args[i]
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

	if f.title == "" {
		return nil, fmt.Errorf("missing required flag: --title")
	}

	if f.summary == "" {
		f.summary = "(no description)"
	}

	return f, nil
}

type showCreateRequest struct {
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	Language   string `json:"language"`
	ImageToken string `json:"image_token,omitempty"`
}

type showCreateResponse struct {
	ShowURI string `json:"show_uri"`
}

func printShowsCreateUsage() {
	fmt.Printf(`Usage: %s shows create [flags]

Create a new show.

Required flags:
  --title <title>        Show title

Optional flags:
  --summary <text>       Show description (default: "(no description)")
  --image <path>         Cover image file (.jpg/.png, max 1 MB)
  --language <code>      Language code (default: en)
`, binName)
}

func handleShowsCreate(args []string) error {
	if len(args) > 0 && isHelp(args[0]) {
		printShowsCreateUsage()
		return nil
	}

	flags, err := parseShowCreateFlags(args)
	if err != nil {
		return err
	}

	token, err := getValidToken()
	if err != nil {
		return err
	}

	imageToken, err := uploadImage(token, flags.image)
	if err != nil {
		return err
	}

	reqBody := showCreateRequest{
		Title:      flags.title,
		Summary:    flags.summary,
		Language:   flags.language,
		ImageToken: imageToken,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url, err := config.BackendURLPath("shows")
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Creating show")
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

	var result showCreateResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if config.JSONMode() {
		return printJSON(result)
	}

	fmt.Println("Show created successfully.")
	fmt.Printf("  URI:   %s\n", result.ShowURI)
	fmt.Printf("  Title: %s\n", flags.title)

	return nil
}

// --- Show Delete ---

func handleShowsDelete(id string) error {
	id = strings.TrimPrefix(id, "spotify:show:")

	token, err := getValidToken()
	if err != nil {
		return err
	}

	url, err := config.BackendURLPath("shows", id)
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Deleting show")
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
		return printJSON(map[string]string{"status": "deleted", "show_id": id})
	}

	fmt.Printf("Show %s deleted successfully.\n", id)
	return nil
}

type showGetResult struct {
	ShowURI       string `json:"show_uri"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	Name          string `json:"name"`
	Publisher     string `json:"publisher"`
	Language      string `json:"language"`
	Explicit      bool   `json:"explicit"`
	Description   string `json:"description"`
	Summary       string `json:"summary"`
	TotalEpisodes int    `json:"total_episodes"`
}

func handleShowsGet(id string) error {
	token, err := getValidToken()
	if err != nil {
		return err
	}

	// Strip URI prefix if provided
	id = strings.TrimPrefix(id, "spotify:show:")

	url, err := config.BackendURLPath("shows", id)
	if err != nil {
		return fmt.Errorf("failed to build request URL: %w", err)
	}
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	ab := startActivity("Fetching show")
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

	if resp.StatusCode != 200 {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}

	if config.JSONMode() {
		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}
		return printJSON(raw)
	}

	var show showGetResult
	if err := json.Unmarshal(body, &show); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	name := show.Title
	if name == "" {
		name = show.Name
	}
	description := show.Description
	if description == "" {
		description = show.Summary
	}
	showID := show.ID
	if showID == "" {
		showID = strings.TrimPrefix(show.ShowURI, "spotify:show:")
	}
	if showID == "" {
		showID = id
	}

	fmt.Printf("Show: %s\n", name)
	fmt.Printf("  ID:          %s\n", showID)
	fmt.Printf("  Publisher:   %s\n", show.Publisher)
	fmt.Printf("  Language:    %s\n", show.Language)
	fmt.Printf("  Episodes:    %d\n", show.TotalEpisodes)
	fmt.Printf("  Description: %s\n", description)

	return nil
}
