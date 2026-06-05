package cmd

import (
	"fmt"

	"github.com/spotify/save-to-spotify/auth"
)

const feedbackURL = "https://community.spotify.com/t5/forums/postpage/board-id/Spotify_Developer"

func printFeedbackUsage() {
	fmt.Printf(`Usage: %s feedback

Open the Spotify Developer community forum to report an issue or share feedback.
`, binName)
}

func handleFeedback(args []string) error {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			printFeedbackUsage()
			return nil
		default:
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	fmt.Printf("Opening %s\n", feedbackURL)
	return auth.OpenBrowser(feedbackURL)
}
