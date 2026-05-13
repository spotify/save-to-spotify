package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spotify/save-to-spotify/cmd"
	"github.com/spotify/save-to-spotify/config"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var se *cmd.SilentError
		if errors.As(err, &se) {
			os.Exit(se.Code)
		}
		if config.JSONMode() {
			fmt.Printf(`{"error":%q}`+"\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
}
