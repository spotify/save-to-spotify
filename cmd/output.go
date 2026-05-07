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
// Use this instead of fmt.Fprintf(os.Stderr, ...) for all progress/status messages.
func info(format string, args ...any) {
	if !config.JSONMode() {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
