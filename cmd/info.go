package cmd

import (
	"fmt"

	"github.com/spotify/save-to-spotify/config"
)

const (
	supportURL = "https://support.spotify.com/article/save-to-spotify/"
	policyURL  = "https://www.spotify.com/legal/save-to-spotify-terms/"
	docsURL    = "https://github.com/spotify/save-to-spotify"
)

type infoOutput struct {
	Support string `json:"support_url"`
	Policy  string `json:"content_policy_url"`
	Docs    string `json:"docs_url"`
}

func printInfoUsage() {
	fmt.Printf("Usage: %s info\n\nUsage limits, content policies, and support link.\n", binName)
}

func handleInfo(args []string) error {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			printInfoUsage()
			return nil
		default:
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	if config.JSONMode() {
		return printJSON(infoOutput{
			Support: supportURL,
			Policy:  policyURL,
			Docs:    docsURL,
		})
	}

	fmt.Println("Usage limits, content policies, and support")
	fmt.Println()
	fmt.Printf("  Content policy:  %s\n", policyURL)
	fmt.Printf("  Support:         %s\n", supportURL)
	fmt.Printf("  Documentation:   %s\n", docsURL)

	return nil
}
