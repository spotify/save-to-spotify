package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spotify/save-to-spotify/config"
)

func TestPrintJSON_ValidOutput(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printJSON(map[string]string{"key": "value"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("printJSON: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, `"key":"value"`) {
		t.Errorf("output = %q, want JSON with key:value", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Error("output should end with newline")
	}
}

func TestPrintJSON_NoHTMLEscaping(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printJSON(map[string]string{"url": "https://example.com?a=1&b=2"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("printJSON: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Without SetEscapeHTML(false), & would become \u0026
	if strings.Contains(output, `\u0026`) {
		t.Errorf("HTML escaping should be disabled: %q", output)
	}
	if !strings.Contains(output, "&") {
		t.Errorf("output should contain literal &: %q", output)
	}
}

func TestInfo_SuppressedInJSONMode(t *testing.T) {
	config.SetJSONMode()
	t.Cleanup(config.ResetJSONMode)

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	info("this should not appear\n")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output != "" {
		t.Errorf("info() should produce no output in JSON mode, got %q", output)
	}
}

func TestInfo_VisibleWhenNotJSON(t *testing.T) {
	config.ResetJSONMode()

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	info("hello %s\n", "world")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if output != "hello world\n" {
		t.Errorf("info() output = %q, want %q", output, "hello world\n")
	}
}
