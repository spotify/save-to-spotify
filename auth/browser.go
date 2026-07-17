package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser attempts to open a URL in the default browser.
// Returns an error if it can't (e.g., headless server).
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		// `open` is a dispatcher that exits promptly — Run (not Start)
		// makes a launch failure (e.g. no WindowServer under launchd)
		// observable to the caller instead of a silent 5-minute callback
		// wait.
		return exec.Command("open", url).Run()
	case "linux":
		// Try xdg-open first, fall back to common browsers. Start, not
		// Run: some openers exec the browser directly and only return
		// when it exits.
		for _, opener := range []string{"xdg-open", "sensible-browser", "x-www-browser", "gnome-open"} {
			if path, err := exec.LookPath(opener); err == nil {
				return exec.Command(path, url).Start()
			}
		}
		return fmt.Errorf("no browser found — use --no-browser flag for manual auth")
	case "windows":
		// rundll32's FileProtocolHandler dispatches and exits promptly.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
	default:
		return fmt.Errorf("unsupported platform %s — use --no-browser flag", runtime.GOOS)
	}
}
