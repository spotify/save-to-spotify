package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spotify/save-to-spotify/config"
)

// progressReader wraps an io.Reader and prints upload progress to stderr.
// It only updates the display at most every 100ms to avoid excessive writes.
type progressReader struct {
	reader     io.Reader
	total      int64
	read       int64
	lastUpdate time.Time
	filename   string
}

func newProgressReader(r io.Reader, total int64, filename string) *progressReader {
	return &progressReader{
		reader:   r,
		total:    total,
		filename: filename,
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)

	if n > 0 && time.Since(pr.lastUpdate) >= 100*time.Millisecond {
		pr.lastUpdate = time.Now()
		pr.printProgress()
	}

	return n, err
}

func (pr *progressReader) printProgress() {
	pct := float64(pr.read) / float64(pr.total) * 100
	if pct > 100 {
		pct = 100
	}

	filled := int(pct / 5) // 20 chars wide
	bar := make([]byte, 20)
	for i := range bar {
		if i < filled {
			bar[i] = '='
		} else if i == filled {
			bar[i] = '>'
		} else {
			bar[i] = ' '
		}
	}

	readMB := float64(pr.read) / (1024 * 1024)
	totalMB := float64(pr.total) / (1024 * 1024)

	fmt.Fprintf(os.Stderr, "\rUploading %s... [%s] %3.0f%% %.1f/%.1f MB",
		pr.filename, string(bar), pct, readMB, totalMB)
}

func (pr *progressReader) finish() {
	pr.printProgress()
	fmt.Fprintf(os.Stderr, "\n")
}

// activityBar shows an indeterminate (bouncing) progress bar for API calls.
// Only active when stderr is a TTY and not in JSON mode.
type activityBar struct {
	label string
	done  chan struct{}
}

const (
	actBarWidth   = 20
	actCursorSize = 6 // length of the "=====>" cursor
)

// startActivity starts an animated activity bar on stderr if in a terminal.
// Call stop() when the operation completes.
func startActivity(label string) *activityBar {
	ab := &activityBar{label: label, done: make(chan struct{})}
	if isTerminal(os.Stderr) && !config.JSONMode() {
		go ab.run()
	}
	return ab
}

func (ab *activityBar) run() {
	pos, dir := 0, 1
	maxPos := actBarWidth - actCursorSize
	for {
		select {
		case <-ab.done:
			return
		case <-time.After(80 * time.Millisecond):
			ab.render(pos)
			pos += dir
			if pos >= maxPos {
				dir = -1
			}
			if pos <= 0 {
				dir = 1
			}
		}
	}
}

func (ab *activityBar) render(pos int) {
	bar := make([]byte, actBarWidth)
	for i := range bar {
		bar[i] = ' '
	}
	for i := range actCursorSize - 1 {
		bar[pos+i] = '='
	}
	bar[pos+actCursorSize-1] = '>'
	fmt.Fprintf(os.Stderr, "\r%s... [%s]", ab.label, string(bar))
}

// stop halts the activity bar and prints a final status line.
// ok=true prints a checkmark, ok=false prints an X.
func (ab *activityBar) stop(ok bool) {
	close(ab.done)
	if isTerminal(os.Stderr) && !config.JSONMode() {
		mark := "\u2713"
		if !ok {
			mark = "\u2717"
		}
		fmt.Fprintf(os.Stderr, "\r%s... [%s] %s\n", ab.label, strings.Repeat("=", actBarWidth), mark)
	}
}
