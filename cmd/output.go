package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spotify/save-to-spotify/config"
)

// printJSON writes v as compact JSON to stdout (HTML escaping disabled).
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// info writes a message to stderr only when not in JSON mode.
func info(format string, args ...any) {
	if !config.JSONMode() {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}

func printEpisodeCreated(uri, title, status string) {
	fmt.Println("Episode created successfully.")
	fmt.Printf("  URI:    %s\n", uri)
	fmt.Printf("  Title:  %s\n", title)
	fmt.Printf("  Status: %s\n", status)
	fmt.Println()
	fmt.Println("Processing — ready in a few minutes.")
	fmt.Printf("Check: %s episodes status %s --wait\n", binName, uri)
}
