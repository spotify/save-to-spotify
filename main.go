package main

import (
	"fmt"
	"os"

	"github.com/spotify/save-to-spotify/cmd"
	"github.com/spotify/save-to-spotify/config"
)

func main() {
	if err := cmd.Execute(); err != nil {
		if config.JSONMode() {
			fmt.Printf(`{"error":%q}`+"\n", err.Error())
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
}
