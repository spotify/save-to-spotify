package cmd

import (
	"bytes"
	"io"
	"testing"

	"github.com/spotify/save-to-spotify/config"
)

func TestProgressReader(t *testing.T) {
	content := []byte("hello world, this is test data for progress reader")
	reader := bytes.NewReader(content)

	pr := newProgressReader(reader, int64(len(content)), "test.mp3")

	result, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(result) != string(content) {
		t.Errorf("read content = %q, want %q", string(result), string(content))
	}

	if pr.read != int64(len(content)) {
		t.Errorf("bytes read = %d, want %d", pr.read, len(content))
	}
}

func TestActivityBar_StopOK(t *testing.T) {
	// Activity bar should not panic and stop cleanly.
	// In non-TTY environments (like tests), it does nothing visually.
	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	ab := startActivity("Testing")
	ab.stop(true)
	// No panic = pass
}

func TestActivityBar_StopFail(t *testing.T) {
	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	ab := startActivity("Testing")
	ab.stop(false)
	// No panic = pass
}
