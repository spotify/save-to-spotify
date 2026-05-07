package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spotify/save-to-spotify/config"
)

func TestValidateMediaFile(t *testing.T) {
	dir := t.TempDir()

	// Valid .mp3 file
	validFile := filepath.Join(dir, "episode.mp3")
	if err := os.WriteFile(validFile, []byte("fake audio"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateMediaFile(validFile); err != nil {
		t.Errorf("valid .mp3: unexpected error: %v", err)
	}

	// Invalid extension
	pdfFile := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdfFile, []byte("fake pdf"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateMediaFile(pdfFile); err == nil {
		t.Error("invalid extension: expected error")
	}

	// Nonexistent file
	if err := validateMediaFile(filepath.Join(dir, "nope.mp3")); err == nil {
		t.Error("nonexistent: expected error")
	}

	// Oversized file (use sparse file via seek, now 1 GB limit)
	bigFile := filepath.Join(dir, "big.mp3")
	f, err := os.Create(bigFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(1024*1024*1024, 0); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	if err := validateMediaFile(bigFile); err == nil {
		t.Error("oversized: expected error")
	}
}

func TestUploadFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "episode.mp3")
	content := []byte("fake audio content for upload test")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	var receivedBody []byte
	var receivedContentType string
	var receivedUserAgent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			http.Error(w, "method not allowed", 405)
			return
		}
		receivedContentType = r.Header.Get("Content-Type")
		receivedUserAgent = r.Header.Get("User-Agent")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(200)
	}))
	defer server.Close()

	err := uploadFile(server.URL, "audio/mpeg", testFile, int64(len(content)))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if string(receivedBody) != string(content) {
		t.Errorf("received body length = %d, want %d", len(receivedBody), len(content))
	}
	if receivedContentType != "audio/mpeg" {
		t.Errorf("received content-type = %q, want %q", receivedContentType, "audio/mpeg")
	}
	if receivedUserAgent != cliUserAgent() {
		t.Errorf("received user-agent = %q, want %q", receivedUserAgent, cliUserAgent())
	}
}

func TestUploadFile_ServerError(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "episode.mp3")
	if err := os.WriteFile(testFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", 403)
	}))
	defer server.Close()

	err := uploadFile(server.URL, "audio/mpeg", testFile, 4)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if err.Error() == "" {
		t.Error("error should not be empty")
	}
}

func TestUploadMultipart_SendsUserAgentOnEveryPart(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "episode.mp3")
	content := []byte("123456789")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	parts := []multipartUploadURL{
		{SignedURL: server.URL + "/part1", PartNumber: 1},
		{SignedURL: server.URL + "/part2", PartNumber: 2},
		{SignedURL: server.URL + "/part3", PartNumber: 3},
	}

	if err := uploadMultipart(parts, "audio/mpeg", testFile, int64(len(content))); err != nil {
		t.Fatalf("uploadMultipart: %v", err)
	}
	if requests != len(parts) {
		t.Fatalf("requests = %d, want %d", requests, len(parts))
	}
}

func TestUploadImage_SendsUserAgent(t *testing.T) {
	dir := t.TempDir()
	imageFile := filepath.Join(dir, "cover.jpg")
	if err := os.WriteFile(imageFile, []byte{0xff, 0xd8, 0xff, 0x00}, 0644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/images" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"upload_token":"tok_123"}`))
	}))
	defer server.Close()

	original := config.BackendBaseURL
	config.BackendBaseURL = server.URL
	t.Cleanup(func() { config.BackendBaseURL = original })

	token := &config.TokenData{AccessToken: "test-token"}
	got, err := uploadImage(token, imageFile)
	if err != nil {
		t.Fatalf("uploadImage: %v", err)
	}
	if got != "tok_123" {
		t.Fatalf("upload token = %q, want %q", got, "tok_123")
	}
}

func TestIsTerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if isTerminal(f) {
		t.Error("temp file should not be a terminal")
	}
}
