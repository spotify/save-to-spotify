package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The upstream kokoro-onnx repo mixes library releases, language-specific
// model releases (v1.1-zh), and quantized variants (fp16, int8) — the
// resolver must skip all of those and land on the newest release carrying
// the canonical full-precision files.
func TestResolveKokoroModelAssets(t *testing.T) {
	const releasesJSON = `[
		{"tag_name": "v0.4.9", "assets": [{"name": "kokoro_onnx-0.4.9.tar.gz", "browser_download_url": "https://example.com/lib.tar.gz"}]},
		{"tag_name": "model-files-v1.1", "assets": [
			{"name": "kokoro-v1.1-zh.onnx", "browser_download_url": "https://example.com/kokoro-v1.1-zh.onnx"},
			{"name": "voices-v1.1-zh.bin", "browser_download_url": "https://example.com/voices-v1.1-zh.bin"}
		]},
		{"tag_name": "model-files-v1.0", "assets": [
			{"name": "kokoro-v1.0.fp16-gpu.onnx", "browser_download_url": "https://example.com/kokoro-v1.0.fp16-gpu.onnx"},
			{"name": "kokoro-v1.0.fp16.onnx", "browser_download_url": "https://example.com/kokoro-v1.0.fp16.onnx"},
			{"name": "kokoro-v1.0.int8.onnx", "browser_download_url": "https://example.com/kokoro-v1.0.int8.onnx"},
			{"name": "kokoro-v1.0.onnx", "browser_download_url": "https://example.com/kokoro-v1.0.onnx"},
			{"name": "voices-v1.0.bin", "browser_download_url": "https://example.com/voices-v1.0.bin"}
		]}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(releasesJSON))
	}))
	defer srv.Close()
	t.Setenv("SAVE_TO_SPOTIFY_KOKORO_RELEASES_API_URL", srv.URL)

	assets, err := resolveKokoroModelAssets()
	if err != nil {
		t.Fatalf("resolveKokoroModelAssets: %v", err)
	}

	if got := assets["kokoro-v*.onnx"]; !strings.HasSuffix(got, "/kokoro-v1.0.onnx") {
		t.Errorf("onnx asset = %q, want the canonical kokoro-v1.0.onnx (not zh or quantized variants)", got)
	}
	if got := assets["voices-v*.bin"]; !strings.HasSuffix(got, "/voices-v1.0.bin") {
		t.Errorf("voices asset = %q, want voices-v1.0.bin", got)
	}
}

func TestResolveKokoroModelAssetsFutureRelease(t *testing.T) {
	// A future standard-named release must win over older ones.
	const releasesJSON = `[
		{"tag_name": "model-files-v1.2", "assets": [
			{"name": "kokoro-v1.2.onnx", "browser_download_url": "https://example.com/kokoro-v1.2.onnx"},
			{"name": "voices-v1.2.bin", "browser_download_url": "https://example.com/voices-v1.2.bin"}
		]},
		{"tag_name": "model-files-v1.0", "assets": [
			{"name": "kokoro-v1.0.onnx", "browser_download_url": "https://example.com/kokoro-v1.0.onnx"},
			{"name": "voices-v1.0.bin", "browser_download_url": "https://example.com/voices-v1.0.bin"}
		]}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(releasesJSON))
	}))
	defer srv.Close()
	t.Setenv("SAVE_TO_SPOTIFY_KOKORO_RELEASES_API_URL", srv.URL)

	assets, err := resolveKokoroModelAssets()
	if err != nil {
		t.Fatalf("resolveKokoroModelAssets: %v", err)
	}
	if got := assets["kokoro-v*.onnx"]; !strings.HasSuffix(got, "/kokoro-v1.2.onnx") {
		t.Errorf("onnx asset = %q, want the newest kokoro-v1.2.onnx", got)
	}
}

func TestRunOpenAITest(t *testing.T) {
	audio := []byte("RIFF-fake-wav-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("auth header = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["response_format"] != "wav" || body["voice"] != "alloy" {
			t.Errorf("unexpected payload: %v", body)
		}
		w.Write(audio)
	}))
	defer srv.Close()
	// SDK semantics: OPENAI_BASE_URL is the complete base INCLUDING /v1 —
	// the handler asserting /v1/audio/speech catches /v1 duplication.
	t.Setenv("OPENAI_BASE_URL", srv.URL+"/v1")
	t.Setenv("OPENAI_API_KEY", "test-key")

	out := filepath.Join(t.TempDir(), "preview.wav")
	if err := runOpenAITest("hello", "", out); err != nil {
		t.Fatalf("runOpenAITest: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != string(audio) {
		t.Fatalf("written file = %q, err %v", got, err)
	}
}

func TestRunElevenLabsTest(t *testing.T) {
	audio := []byte("fake-mp3-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("xi-api-key"); got != "el-key" {
			t.Errorf("xi-api-key = %q", got)
		}
		switch r.URL.Path {
		case "/v1/voices":
			w.Write([]byte(`{"voices":[{"voice_id":"abc123","name":"Rachel"}]}`))
		case "/v1/text-to-speech/abc123":
			w.Write(audio)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	t.Setenv("ELEVENLABS_BASE_URL", srv.URL)
	t.Setenv("ELEVENLABS_API_KEY", "el-key")

	out := filepath.Join(t.TempDir(), "preview.mp3")
	if err := runElevenLabsTest("hello", "", out); err != nil {
		t.Fatalf("runElevenLabsTest: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != string(audio) {
		t.Fatalf("written file = %q, err %v", got, err)
	}

	// Unknown voice surfaces a clear error instead of a synthesis attempt.
	if err := runElevenLabsTest("hello", "Nobody", out); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected voice-not-found error, got %v", err)
	}
}
