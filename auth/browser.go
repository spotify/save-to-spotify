package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser attempts to open a URL in the default browser.
// Returns an error if it can't (e.g., headless server).
func OpenBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		// Try xdg-open first, fall back to common browsers
		for _, opener := range []string{"xdg-open", "sensible-browser", "x-www-browser", "gnome-open"} {
			if path, err := exec.LookPath(opener); err == nil {
				cmd = exec.Command(path, url)
				break
			}
		}
		if cmd == nil {
			return fmt.Errorf("no browser found — use --no-browser flag for manual auth")
		}
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("unsupported platform %s — use --no-browser flag", runtime.GOOS)
	}

	return cmd.Start()
}
