package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

const binName = "save-to-spotify"

// version is set at build time via -ldflags from `git describe --tags --always`.
var version = "dev"

// commit is set at build time via -ldflags.
var commit = "unknown"

// Execute parses args and dispatches to the appropriate command.
func Execute() error {
	if os.Getenv("CLAUDECODE") != "" {
		fmt.Fprintln(os.Stderr, `<claude-code-hint v="1" type="plugin" value="save-to-spotify@claude-plugins-official" />`)
	}

	args := os.Args[1:]

	filtered := args[:0]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			config.SetJSONMode()
		case "--timeout":
			if i+1 >= len(args) {
				return fmt.Errorf("--timeout requires a value (examples: 30s, 2m)")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil || d <= 0 {
				return fmt.Errorf("invalid --timeout value: %s (examples: 30s, 2m)", args[i])
			}
			config.SetAPITimeout(d)
		default:
			filtered = append(filtered, args[i])
		}
	}
	args = filtered
	httpClient.Timeout = config.APITimeout()

	var finishUpdateCheck func()
	if shouldStartUpdateCheck(args) {
		finishUpdateCheck = StartUpdateCheck()
	}

	if len(args) == 0 {
		printUsage()
		return nil
	}

	var err error
	switch args[0] {
	case "upload":
		err = handleUpload(args[1:])
	case "list":
		err = handleList(args[1:])
	case "auth":
		err = handleAuth(args[1:])
	case "token":
		if len(args) > 1 && isHelp(args[1]) {
			fmt.Printf("Usage: %s token\n\nPrint the current access token (for piping to other commands).\n", binName)
			return nil
		}
		err = handleToken()
	case "shows", "show":
		err = handleShows(args[1:])
	case "episodes", "episode":
		err = handleEpisodes(args[1:])
	case "timeline":
		err = handleTimeline(args[1:])
	case "update":
		err = handleUpdate(args[1:])
	case "feedback":
		err = handleFeedback(args[1:])
	case "version", "--version":
		if config.JSONMode() {
			return printJSON(map[string]any{"version": version, "commit": commit})
		}
		fmt.Printf("%s %s (%s)\n", binName, version, commit)
		return nil
	case "help", "--help", "-h":
		printUsage()
		return nil
	case "create":
		err = fmt.Errorf("unknown command: create. Did you mean \"shows create\" or \"episodes create\"?")
	case "delete":
		err = fmt.Errorf("unknown command: delete. Did you mean \"shows delete\" or \"episodes delete\"?")
	case "get":
		err = fmt.Errorf("unknown command: get. Did you mean \"shows get\"?")
	case "set":
		err = fmt.Errorf("unknown command: set. Did you mean \"timeline set\"?")
	case "status":
		err = fmt.Errorf("unknown command: status. Did you mean \"episodes status\"?")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", args[0])
		printUsage()
		err = fmt.Errorf("unknown command: %s", args[0])
	}

	if err == nil && finishUpdateCheck != nil {
		finishUpdateCheck()
	}

	return err
}

func shouldStartUpdateCheck(args []string) bool {
	if len(args) == 0 || os.Getenv(config.EnvVarNoUpdateCheck) != "" {
		return false
	}
	if isHelp(args[0]) || args[0] == "update" || args[0] == "version" || args[0] == "--version" || args[0] == "feedback" {
		return false
	}
	for i, arg := range args {
		if arg == "-h" || arg == "--help" {
			return false
		}
		if arg == "help" && i <= 1 {
			return false
		}
	}
	return true
}

func printUsage() {
	fmt.Printf(`%[1]s — Save to Spotify

Usage:
  %[1]s <command> [flags]

Commands:
  upload <file>           Upload a media file and create an episode
  shows                   List shows created via CLI
  shows create            Create a new show
  shows get <id>          Get a show by ID
  shows delete <id>       Delete a show
  episodes                List episodes created via CLI
  episodes create         Create an episode
  episodes delete <id>    Delete an episode
  episodes status <id>    Check episode readiness
  list                    List episodes for the last created show
  list shows              List shows created via CLI
  auth login              Authenticate with Spotify
  auth status             Show current authentication status
  auth logout             Remove stored credentials
  timeline                Manage timeline (chapters + images + links + Spotify references)
  timeline get <id>       Get timeline for an episode
  timeline set            Set (replace) timeline from a JSON file
  timeline delete <id>    Delete all timeline items for an episode
  update                  Check for and install updates
  feedback                Report an issue or share feedback
  token                   Print the current access token (for piping)
  version                 Print version

Auth flags:
  --no-browser        Don't open a browser (for headless/remote servers)
  --port <port>       Local callback port (default: 8085)

Global flags:
  --json              Output results as JSON (for scripting and agents)
  --timeout <dur>     API request timeout (default: 30s, e.g. 1m, 90s)

Note: Show and episode metadata is immutable after creation. To modify,
delete and recreate with the desired fields.

Environment variables:
  SAVE_TO_SPOTIFY_AUTH_TOKEN    Bearer token (skips OAuth; no refresh)
  SAVE_TO_SPOTIFY_BACKEND_URL   Override the backend URL (default: https://saveto.spotify.com)
  SAVE_TO_SPOTIFY_TIMEOUT       API timeout duration (e.g. 30s, 2m)
  SAVE_TO_SPOTIFY_CLIENT_ID     OAuth client ID override
  SAVE_TO_SPOTIFY_NO_UPDATE_CHECK    Disable the passive update check that runs after successful commands
  SAVE_TO_SPOTIFY_RELEASES_URL       Override the releases download URL
  SAVE_TO_SPOTIFY_RELEASES_API_URL   Override the fallback version check URL (backend)
  SAVE_TO_SPOTIFY_GITHUB_RELEASES_URL   Override the primary version check URL (GitHub Releases API)
  SAVE_TO_SPOTIFY_HEADERS            Additional backend headers (JSON object, X-STS-* only)
`, binName)
}
