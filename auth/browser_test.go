package auth

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestLaunchAndWatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test commands use sh")
	}

	t.Run("fast failure is propagated", func(t *testing.T) {
		// Mirrors xdg-open exiting 3/4 when no handler or tool exists.
		err := launchAndWatch(exec.Command("sh", "-c", "exit 3"), time.Second)
		if err == nil {
			t.Fatal("expected the opener's non-zero exit to surface as an error")
		}
	})

	t.Run("fast clean exit succeeds", func(t *testing.T) {
		if err := launchAndWatch(exec.Command("sh", "-c", "exit 0"), time.Second); err != nil {
			t.Fatalf("clean exit should succeed, got %v", err)
		}
	})

	t.Run("long-running process is assumed to be the browser", func(t *testing.T) {
		start := time.Now()
		err := launchAndWatch(exec.Command("sleep", "5"), 100*time.Millisecond)
		if err != nil {
			t.Fatalf("still-running process should count as success, got %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("returned after %v — should return at the grace window, not block on the process", elapsed)
		}
	})
}
