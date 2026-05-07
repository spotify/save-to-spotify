package cmd

import (
	"fmt"

	"github.com/spotify/save-to-spotify/config"
)

func printListUsage() {
	fmt.Printf(`Usage: %s list [command] [flags]

Commands:
  (none)       List episodes for the last created show (same as 'list episodes')
  episodes     List episodes for the last created show
  shows        List shows created via CLI

Flags:
  --show-id <id>   Filter episodes to a specific show (episodes only)
`, binName)
}

func handleList(args []string) error {
	if len(args) > 0 && isHelp(args[0]) {
		printListUsage()
		return nil
	}

	if len(args) == 0 {
		return handleListEpisodes(nil)
	}

	switch args[0] {
	case "episodes":
		return handleListEpisodes(args[1:])
	case "shows":
		return handleShowsList()
	default:
		return fmt.Errorf("unknown list subcommand: %s", args[0])
	}
}

func handleListEpisodes(args []string) error {
	var showIDFilter string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--show-id":
			if i+1 >= len(args) {
				return fmt.Errorf("--show-id requires a value")
			}
			i++
			showIDFilter = args[i]
		case "-h", "--help":
			fmt.Printf("Usage: %s list [episodes] [--show-id <id>]\n\nList episodes for a show.\n", binName)
			return nil
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	token, err := getValidToken()
	if err != nil {
		return err
	}

	// Determine which show to list episodes for.
	var targetShowURI string
	var targetShowTitle string

	if showIDFilter != "" {
		targetShowURI = showIDFilter
	} else {
		last, err := lastShow(token)
		if err != nil {
			if config.JSONMode() {
				return printJSON(map[string]any{"episodes": []EpisodeSummary{}})
			}
			fmt.Println("No shows found. Use 'upload' to create a show and episode.")
			return nil
		}
		targetShowURI = last.ShowURI
		targetShowTitle = last.Title
	}

	episodes, err := listEpisodes(token, targetShowURI)
	if err != nil {
		return err
	}

	if config.JSONMode() {
		if episodes == nil {
			episodes = []EpisodeSummary{}
		}
		return printJSON(map[string]any{"episodes": episodes})
	}

	if len(episodes) == 0 {
		fmt.Printf("No episodes found for show %s.\n", targetShowURI)
		return nil
	}

	header := fmt.Sprintf("Episodes for: %s", targetShowURI)
	if targetShowTitle != "" {
		header = fmt.Sprintf("Episodes for: %s (%s)", targetShowURI, targetShowTitle)
	}
	fmt.Println(header)
	fmt.Println()
	for _, e := range episodes {
		fmt.Printf("  %-43s  %-20s  %s\n", e.EpisodeURI, e.Title, e.CreatedAt)
	}

	return nil
}
