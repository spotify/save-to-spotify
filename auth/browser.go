package auth

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// openerGraceWindow is how long a Linux opener is watched after launch:
// failures (missing handler/tool) surface within it, while a still-running
// process is assumed to be the browser and left alone.
const openerGraceWindow = time.Second

// launchAndWatch starts cmd and observes it for the grace window. A quick
// exit propagates the result (an opener failing fast returns its error); a
// process still running when the window closes is treated as success and
// reaped by the background Wait whenever it eventually exits, so no zombie
// is left behind.
func launchAndWatch(cmd *exec.Cmd, grace time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(grace):
		return nil
	}
}

// LinuxOpeners are the URL-opener binaries probed on Linux, in preference
// order. Shared with the browser-capability detection in cmd — keep the two
// behaviors in sync by keeping the list in one place.
var LinuxOpeners = []string{"xdg-open", "sensible-browser", "x-www-browser", "gnome-open"}

// OpenBrowser attempts to open a URL in the default browser.
// Returns an error if it can't (e.g., headless server).
//
// This function launches unconditionally — it does not re-check for a
// display session. Callers on flows where headlessness matters are expected
// to gate on the capability detection in cmd (isHeadless/canOpenBrowser)
// first, or to handle the returned error with a manual fallback.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		// `open` is a dispatcher that exits promptly — Run (not Start)
		// makes a launch failure (e.g. no WindowServer under launchd)
		// observable to the caller instead of a silent 5-minute callback
		// wait.
		return exec.Command("open", url).Run()
	case "linux":
		// Try xdg-open first, fall back to common browsers. These can't
		// simply Run: some openers exec the browser directly and only
		// return when it exits. But a plain Start would discard quick
		// failures (xdg-open exits 3/4 when no handler or tool exists) —
		// watch the process briefly instead.
		for _, opener := range LinuxOpeners {
			if path, err := exec.LookPath(opener); err == nil {
				return launchAndWatch(exec.Command(path, url), openerGraceWindow)
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
