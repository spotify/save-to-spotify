package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spotify/save-to-spotify/auth"
	"github.com/spotify/save-to-spotify/config"
	"github.com/spotify/save-to-spotify/internal/httpx"
)

func printTTSUsage() {
	fmt.Printf(`Usage: %s tts <command>

Manage text-to-speech engines for audio content creation.

Commands:
  status              Show TTS engine status (default)
  setup [--engine X]  Install and configure a TTS engine
  voices [--engine X] List available voices for an engine
  test [--engine X] [--voice V]  Synthesize a test phrase
  default [name]      Get or set the default TTS engine
  add                 Register a custom TTS engine
  remove <name>       Remove a custom TTS engine
`, binName)
}

func handleTTS(args []string) error {
	if len(args) == 0 {
		return handleTTSStatus(nil)
	}
	switch args[0] {
	case "status":
		return handleTTSStatus(args[1:])
	case "setup":
		return handleTTSSetup(args[1:])
	case "voices":
		return handleTTSVoices(args[1:])
	case "test":
		return handleTTSTest(args[1:])
	case "default":
		return handleTTSDefault(args[1:])
	case "add":
		return handleTTSAdd(args[1:])
	case "remove":
		return handleTTSRemove(args[1:])
	case "-h", "--help", "help":
		printTTSUsage()
		return nil
	default:
		return fmt.Errorf("unknown tts subcommand: %s", args[0])
	}
}

// handleTTSStatus shows the current status of all TTS engines and ffmpeg.
func handleTTSStatus(args []string) error {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			fmt.Printf("Usage: %s tts status\n\nShow TTS engine and ffmpeg status.\n", binName)
			return nil
		default:
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	engines := detectTTSEngines()
	ffmpeg := detectFFmpeg()

	if config.JSONMode() {
		return printJSON(map[string]any{
			"engines":       engines,
			"ffmpeg":        ffmpeg,
			"default":       defaultEngineNameFrom(engines),
			"kokoro_python": kokoroPython(),
		})
	}

	fmt.Println("TTS Engines:")
	for _, e := range engines {
		prefix := "!"
		if e.Status == TTSReady {
			prefix = "✓"
		}
		fmt.Printf("  %s %-25s %s%s\n", prefix, e.DisplayName, e.Status, engineSuffix(e))
	}

	fmt.Println()
	fmt.Println("Dependencies:")
	prefix := "!"
	if ffmpeg.Status == CheckOK {
		prefix = "✓"
	}
	fmt.Printf("  %s %-25s %s\n", prefix, "ffmpeg", ffmpeg.Detail)

	return nil
}

// handleTTSSetup installs and configures a TTS engine.
func handleTTSSetup(args []string) error {
	engine := "kokoro"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Printf(`Usage: %s tts setup [--engine name]

Install and configure a TTS engine.

Flags:
  --engine <name>   Engine to set up (default: kokoro)
                    Options: kokoro, openai, elevenlabs
`, binName)
			return nil
		case "--engine":
			if i+1 >= len(args) {
				return fmt.Errorf("--engine requires a value")
			}
			i++
			engine = args[i]
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	switch engine {
	case "kokoro":
		return setupKokoro()
	case "openai":
		return setupAPIEngine("openai", "OPENAI_API_KEY")
	case "elevenlabs":
		return setupAPIEngine("elevenlabs", "ELEVENLABS_API_KEY")
	default:
		return fmt.Errorf("unknown engine: %s (options: kokoro, openai, elevenlabs)", engine)
	}
}

func isBuiltinEngine(name string) bool {
	return name == "openai" || name == "elevenlabs" || name == "kokoro"
}

func setupKokoro() error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}

	venvDir := filepath.Join(dir, kokoroVenvName)
	venvPy := venvPython(venvDir)

	// The package must import AND the model weights must exist; the
	// package alone cannot synthesize.
	packageInstalled := false
	if _, err := os.Stat(venvPy); err == nil {
		packageInstalled = exec.Command(venvPy, "-c", "import kokoro_onnx").Run() == nil
	}

	if !packageInstalled {
		// Find a Python 3.10+ binary (required by onnxruntime).
		pythonBin, err := findPython3()
		if err != nil {
			return fmt.Errorf("%s — install a newer Python (e.g. brew install python@3.12) and try again", err)
		}

		// Create venv.
		info("Creating Python virtual environment (using %s)...\n", pythonBin)
		cmd := exec.Command(pythonBin, "-m", "venv", venvDir, "--clear")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to create venv: %w", err)
		}

		// Install kokoro-onnx and soundfile.
		info("Installing kokoro-onnx and soundfile...\n")
		pip := venvPip(venvDir)
		cmd = exec.Command(pip, "install", "kokoro-onnx", "soundfile")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install kokoro-onnx: %w", err)
		}

		// Install spaCy model.
		info("Installing spaCy language model...\n")
		cmd = exec.Command(pip, "install",
			"https://github.com/explosion/spacy-models/releases/download/en_core_web_sm-3.8.0/en_core_web_sm-3.8.0-py3-none-any.whl")
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to install spaCy model: %w", err)
		}
	}

	// Download the model weights — the pip package does not include them.
	if err := downloadKokoroModels(dir); err != nil {
		return err
	}

	if packageInstalled {
		info("Kokoro is ready at %s\n", venvDir)
	} else {
		info("Kokoro installed successfully.\n")
	}
	return setDefaultEngine("kokoro")
}

// kokoroReleasesAPIURL returns the GitHub API URL listing kokoro-onnx
// releases, overridable for proxies and air-gapped mirrors like the
// project's own release URLs.
func kokoroReleasesAPIURL() string {
	if v := os.Getenv("SAVE_TO_SPOTIFY_KOKORO_RELEASES_API_URL"); v != "" {
		return v
	}
	return "https://api.github.com/repos/thewh1teagle/kokoro-onnx/releases?per_page=30"
}

// kokoroFallbackAssets pins a known-good release for when the releases API
// is unreachable (rate limit, air gap without an API mirror).
var kokoroFallbackAssets = map[string]string{
	"kokoro-v*.onnx": "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/kokoro-v1.0.onnx",
	"voices-v*.bin":  "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/voices-v1.0.bin",
}

// kokoroAssetNames matches the canonical full-precision asset per model
// pattern. Anchored tightly on purpose: upstream releases also carry
// quantized variants (kokoro-v1.0.fp16.onnx, .int8) and language-specific
// models (kokoro-v1.1-zh.onnx) that must not be picked up.
var kokoroAssetNames = map[string]*regexp.Regexp{
	"kokoro-v*.onnx": regexp.MustCompile(`^kokoro-v\d+(\.\d+)*\.onnx$`),
	"voices-v*.bin":  regexp.MustCompile(`^voices-v\d+(\.\d+)*\.bin$`),
}

// resolveKokoroModelAssets maps each model pattern to the download URL of
// the matching asset in the newest upstream "model-files-*" release that
// carries all required files, so new model versions are picked up without a
// CLI change.
func resolveKokoroModelAssets() (map[string]string, error) {
	client := &http.Client{Timeout: 30 * time.Second, Transport: httpx.UserAgentTransport{UserAgent: cliUserAgent()}}
	resp, err := client.Get(kokoroReleasesAPIURL())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if !isSuccessStatus(resp.StatusCode) {
		return nil, fmt.Errorf("releases API returned %s", resp.Status)
	}

	var releases []struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	// Releases come newest-first; take the first model-files release that
	// carries every file we need.
	for _, rel := range releases {
		if !strings.HasPrefix(rel.TagName, "model-files-") {
			continue
		}
		assets := make(map[string]string, len(kokoroModelPatterns))
		for _, pattern := range kokoroModelPatterns {
			for _, a := range rel.Assets {
				if kokoroAssetNames[pattern].MatchString(a.Name) {
					assets[pattern] = a.BrowserDownloadURL
					break
				}
			}
		}
		if len(assets) == len(kokoroModelPatterns) {
			return assets, nil
		}
	}
	return nil, fmt.Errorf("no model-files release with all required assets found")
}

// downloadKokoroModels fetches missing model weights into the config
// directory (not the venv — it survives venv rebuilds). Files already
// present in any search directory, at any version, are kept as is.
func downloadKokoroModels(configDir string) error {
	if kokoroModelsPresent() {
		return nil
	}
	assets, err := resolveKokoroModelAssets()
	if err != nil {
		info("Could not resolve the latest Kokoro models (%v) — using the pinned fallback release.\n", err)
		assets = kokoroFallbackAssets
	}
	info("Downloading Kokoro model files (~340 MB)...\n")
	for _, pattern := range kokoroModelPatterns {
		if kokoroModelPath(pattern) != "" {
			continue
		}
		url, ok := assets[pattern]
		if !ok {
			return fmt.Errorf("no download source for %s", pattern)
		}
		dest := filepath.Join(configDir, path.Base(url))
		if err := downloadFile(dest, url); err != nil {
			return fmt.Errorf("failed to download %s: %w", path.Base(url), err)
		}
	}
	return nil
}

// downloadFile fetches url into dest atomically (temp file + rename). The
// client bounds the connect and response-header phases so a stalled server
// fails fast, but sets no overall timeout — the global API timeout would cut
// off legitimate multi-minute body transfers.
func downloadFile(dest, url string) error {
	client := &http.Client{
		Transport: httpx.UserAgentTransport{
			UserAgent: cliUserAgent(),
			Base: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
				ResponseHeaderTimeout: 30 * time.Second,
				Proxy:                 http.ProxyFromEnvironment,
			},
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !isSuccessStatus(resp.StatusCode) {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), filepath.Base(dest)+".partial-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	var body io.Reader = resp.Body
	var progress *progressReader
	if resp.ContentLength > 0 {
		progress = newProgressReader(resp.Body, resp.ContentLength, filepath.Base(dest))
		progress.verb = "Downloading"
		body = progress
	}

	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		return err
	}
	if progress != nil {
		progress.finish()
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

func setupAPIEngine(name, envVar string) error {
	if os.Getenv(envVar) == "" {
		return fmt.Errorf("%s is not set — export it and try again", envVar)
	}
	info("%s API key detected.\n", envVar)
	return setDefaultEngine(name)
}

func setDefaultEngine(name string) error {
	cfg, err := loadTTSConfig()
	if err != nil {
		return err
	}
	cfg.Default = name
	if err := saveTTSConfig(cfg); err != nil {
		return err
	}
	info("Default TTS engine set to %s.\n", name)
	return nil
}

// Built-in voice lists per engine.
var builtinVoices = map[string][]string{
	"kokoro":     {"af_heart (American female, recommended)", "af_nicole (American female, soft — sleep content)", "af_alloy (American female)", "am_adam (American male)", "bf_emma (British female)", "bm_george (British male)"},
	"openai":     {"alloy", "echo", "fable", "onyx", "nova", "shimmer"},
	"elevenlabs": {"Amelia", "George", "Bella", "Rachel"},
}

// handleTTSVoices lists available voices for a TTS engine.
func handleTTSVoices(args []string) error {
	engine := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Printf("Usage: %s tts voices [--engine name]\n\nList available voices for a TTS engine.\n", binName)
			return nil
		case "--engine":
			if i+1 >= len(args) {
				return fmt.Errorf("--engine requires a value")
			}
			i++
			engine = args[i]
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	// Default to the configured default engine.
	if engine == "" {
		engine = defaultEngineName()
	}
	if engine == "" {
		return fmt.Errorf("no default engine configured — run `%s tts setup` first", binName)
	}

	voices, ok := builtinVoices[engine]
	if !ok {
		if config.JSONMode() {
			return printJSON(map[string]any{"engine": engine, "voices": []string{}})
		}
		fmt.Printf("No built-in voice list for engine %q.\n", engine)
		return nil
	}

	if config.JSONMode() {
		return printJSON(map[string]any{"engine": engine, "voices": voices})
	}

	fmt.Printf("Voices for %s:\n", engine)
	for _, v := range voices {
		fmt.Printf("  %s\n", v)
	}
	return nil
}

// handleTTSTest synthesizes a test phrase and prints its path and a
// file:// link; --play plays it inline instead.
func handleTTSTest(args []string) error {
	engine := ""
	voice := ""
	usePlay := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Printf("Usage: %s tts test [--engine name] [--voice name] [--play]\n\nSynthesize a test phrase and print a file:// link to the preview.\n\nFlags:\n  --play    Play the preview immediately instead of only printing the link\n", binName)
			return nil
		case "--engine":
			if i+1 >= len(args) {
				return fmt.Errorf("--engine requires a value")
			}
			i++
			engine = args[i]
		case "--voice":
			if i+1 >= len(args) {
				return fmt.Errorf("--voice requires a value")
			}
			i++
			voice = args[i]
		case "--play":
			usePlay = true
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if engine == "" {
		engine = defaultEngineName()
	}
	if engine == "" {
		return fmt.Errorf("no default engine configured — run `%s tts setup` first", binName)
	}

	testPhrase := "Hello! This is a voice test from Save to Spotify."

	previewDir := os.Getenv("SAVE_TO_SPOTIFY_PREVIEW_DIR")
	if previewDir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			cacheDir = os.TempDir()
		}
		previewDir = filepath.Join(cacheDir, "save-to-spotify")
	}
	os.MkdirAll(previewDir, 0o755)
	// Extension must match the bytes the engine produces: Kokoro and
	// OpenAI (response_format=wav) emit WAV; ElevenLabs emits MP3.
	ext := "wav"
	if engine == "elevenlabs" {
		ext = "mp3"
	}
	outPath := filepath.Join(previewDir, fmt.Sprintf("voice-preview-%s.%s", engine, ext))

	info("Synthesizing test phrase with %s...\n", engine)

	var synthErr error
	switch engine {
	case "kokoro":
		synthErr = runKokoroTest(testPhrase, voice, outPath)
	case "openai":
		synthErr = runOpenAITest(testPhrase, voice, outPath)
	case "elevenlabs":
		synthErr = runElevenLabsTest(testPhrase, voice, outPath)
	default:
		return fmt.Errorf("no test script for engine %q", engine)
	}
	if synthErr != nil {
		return previewWriteHint(synthErr, outPath)
	}

	if usePlay {
		return playPreview(outPath)
	}

	fmt.Printf("Preview saved: %s\n", outPath)
	fmt.Printf("Listen: file://%s\n", outPath)
	return nil
}

// previewWriteHint enriches a synthesis failure on Windows when no output
// file was produced: the synthesis subprocess can end up with a different
// view of %LOCALAPPDATA% than the parent when the folder is virtualized
// (OneDrive Known Folder Move, MSIX containers, antivirus sandboxing), so
// the write fails with an opaque libsndfile/IO traceback.
func previewWriteHint(err error, outPath string) error {
	if runtime.GOOS != "windows" {
		return err
	}
	if _, statErr := os.Stat(outPath); statErr == nil {
		return err
	}
	return fmt.Errorf("%w\ncould not write the preview to %s — if that folder is virtualized (OneDrive Known Folder Move, MSIX, antivirus sandboxing), set SAVE_TO_SPOTIFY_PREVIEW_DIR to a different directory and retry", err, filepath.Dir(outPath))
}

func playPreview(path string) error {
	// Try command-line players that play inline (no GUI, no app launch).
	// ffplay first: cross-platform including Windows (ships with ffmpeg,
	// which this project already requires). afplay: macOS built-in.
	// aplay: Linux ALSA (wav only).
	players := []struct {
		bin  string
		args []string
	}{
		{"ffplay", []string{"-nodisp", "-autoexit", "-loglevel", "quiet", path}},
		{"afplay", []string{path}},
		{"aplay", []string{"-q", path}},
	}
	for _, p := range players {
		if bin, err := exec.LookPath(p.bin); err == nil {
			info("Playing preview...\n")
			cmd := exec.Command(bin, p.args...)
			return cmd.Run()
		}
	}
	// No inline player found — fall back to system open.
	info("Opening in default player...\n")
	if err := auth.OpenBrowser(path); err != nil {
		fmt.Printf("Preview saved: %s\n", path)
	}
	return nil
}

func runPythonTTSTest(pythonBin, script, engineName string) error {
	cmd := exec.Command(pythonBin, "-c", script)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s synthesis failed: %w", engineName, err)
	}
	return nil
}

func runKokoroTest(text, voice, outPath string) error {
	if voice == "" {
		voice = "af_heart"
	}

	// Resolve the model files in Go — the single search implementation —
	// and hand the script ready paths.
	model := kokoroModelPath("kokoro-v*.onnx")
	voices := kokoroModelPath("voices-v*.bin")
	if model == "" || voices == "" {
		return fmt.Errorf("kokoro model files not found — run `%s tts setup`", binName)
	}

	script := fmt.Sprintf(`
import kokoro_onnx, soundfile as sf
kokoro = kokoro_onnx.Kokoro(%q, %q)
samples, sr = kokoro.create(%q, voice=%q)
sf.write(%q, samples, sr)
`, model, voices, text, voice, outPath)

	return runPythonTTSTest(kokoroPython(), script, "kokoro")
}

// ttsHTTPClient bounds cloud synthesis calls: generous overall timeout for
// audio generation, standard header timeout.
var ttsHTTPClient = &http.Client{
	Timeout:   60 * time.Second,
	Transport: httpx.UserAgentTransport{UserAgent: cliUserAgent()},
}

// openaiBaseURL and elevenlabsBaseURL follow the same override convention as
// the SDKs and the rest of this codebase's external URLs — needed for tests
// and proxies.
//
// OPENAI_BASE_URL is the COMPLETE API base including /v1 (the official SDK's
// semantics — its default is https://api.openai.com/v1), so callers append
// only the endpoint path, never /v1.
func openaiBaseURL() string {
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.openai.com/v1"
}

// ELEVENLABS_BASE_URL is scheme + host only (no /v1 suffix) — callers
// prepend /v1/<endpoint> themselves. This matches the ElevenLabs SDK
// convention and deliberately differs from OPENAI_BASE_URL above; do not
// "align" them.
func elevenlabsBaseURL() string {
	if v := os.Getenv("ELEVENLABS_BASE_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "https://api.elevenlabs.io"
}

// runOpenAITest synthesizes via the OpenAI speech API directly in Go — no
// Python dependency. WAV is requested explicitly so the preview extension
// matches the bytes.
func runOpenAITest(text, voice, outPath string) error {
	if voice == "" {
		voice = "alloy"
	}

	payload, err := json.Marshal(map[string]string{
		"model":           "tts-1",
		"voice":           voice,
		"input":           text,
		"response_format": "wav",
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, openaiBaseURL()+"/audio/speech", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("OPENAI_API_KEY"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ttsHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("openai synthesis failed: %w", err)
	}
	defer resp.Body.Close()
	if !isSuccessStatus(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("openai synthesis failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return writeResponseFile(outPath, resp.Body)
}

// runElevenLabsTest synthesizes via the ElevenLabs API directly in Go — no
// Python dependency. Output is MP3 (the API default); the caller assigns a
// matching .mp3 extension.
func runElevenLabsTest(text, voice, outPath string) error {
	if voice == "" {
		voice = "Rachel"
	}
	apiKey := os.Getenv("ELEVENLABS_API_KEY")

	// Resolve the voice name to an id.
	req, err := http.NewRequest(http.MethodGet, elevenlabsBaseURL()+"/v1/voices", nil)
	if err != nil {
		return err
	}
	req.Header.Set("xi-api-key", apiKey)
	resp, err := ttsHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("elevenlabs voice lookup failed: %w", err)
	}
	defer resp.Body.Close()
	if !isSuccessStatus(resp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("elevenlabs voice lookup failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var voicesResp struct {
		Voices []struct {
			VoiceID string `json:"voice_id"`
			Name    string `json:"name"`
		} `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&voicesResp); err != nil {
		return fmt.Errorf("elevenlabs voice lookup failed: %w", err)
	}
	voiceID := ""
	for _, v := range voicesResp.Voices {
		if v.Name == voice {
			voiceID = v.VoiceID
			break
		}
	}
	if voiceID == "" {
		return fmt.Errorf("voice %q not found — check available voices with `%s tts voices --engine elevenlabs`", voice, binName)
	}

	payload, err := json.Marshal(map[string]any{
		"text":           text,
		"voice_settings": map[string]float64{"stability": 0.5, "similarity_boost": 0.5},
	})
	if err != nil {
		return err
	}
	synthReq, err := http.NewRequest(http.MethodPost, elevenlabsBaseURL()+"/v1/text-to-speech/"+voiceID, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	synthReq.Header.Set("xi-api-key", apiKey)
	synthReq.Header.Set("Content-Type", "application/json")

	synthResp, err := ttsHTTPClient.Do(synthReq)
	if err != nil {
		return fmt.Errorf("elevenlabs synthesis failed: %w", err)
	}
	defer synthResp.Body.Close()
	if !isSuccessStatus(synthResp.StatusCode) {
		body, _ := io.ReadAll(io.LimitReader(synthResp.Body, 4096))
		return fmt.Errorf("elevenlabs synthesis failed: %s: %s", synthResp.Status, strings.TrimSpace(string(body)))
	}
	return writeResponseFile(outPath, synthResp.Body)
}

// writeResponseFile streams an HTTP body to outPath, capped well above any
// legitimate single-phrase preview so a broken API response can't fill the
// disk. Oversized responses are rejected with an error, never silently
// truncated into corrupt audio, and no partial file is left behind.
func writeResponseFile(outPath string, body io.Reader) error {
	const maxPreviewBytes = 100 << 20 // 100 MB
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	discard := func() {
		f.Close()
		os.Remove(outPath)
	}
	// Read one byte past the cap: reaching it proves the response is too big.
	n, err := io.Copy(f, io.LimitReader(body, maxPreviewBytes+1))
	if err != nil {
		discard()
		return err
	}
	if n > maxPreviewBytes {
		discard()
		return fmt.Errorf("preview response exceeded %d MB — refusing truncated audio", maxPreviewBytes>>20)
	}
	return f.Close()
}

// handleTTSDefault gets or sets the default TTS engine.
func handleTTSDefault(args []string) error {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Printf("Usage: %s tts default [name]\n\nGet or set the default TTS engine.\n", binName)
			return nil
		}
	}

	engines := detectTTSEngines()

	if len(args) == 0 {
		name := defaultEngineNameFrom(engines)
		if config.JSONMode() {
			return printJSON(map[string]string{"default": name})
		}
		if name == "" {
			fmt.Println("No default TTS engine configured.")
			return nil
		}
		fmt.Printf("Default TTS engine: %s\n", name)
		return nil
	}

	name := args[0]

	found := false
	for _, e := range engines {
		if e.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown engine %q — use `%s tts status` to see available engines", name, binName)
	}

	return setDefaultEngine(name)
}

// handleTTSAdd registers a custom TTS engine in tts.json.
func handleTTSAdd(args []string) error {
	name := ""
	checkCmd := ""
	keyEnvVar := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			fmt.Printf(`Usage: %s tts add --name X --check-cmd "cmd" [--key-env VAR]

Register a custom TTS engine.

Flags:
  --name       Engine name (required)
  --check-cmd  Command to verify the engine is installed (required)
  --key-env    Environment variable for the API key (optional)
`, binName)
			return nil
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
		case "--check-cmd":
			if i+1 >= len(args) {
				return fmt.Errorf("--check-cmd requires a value")
			}
			i++
			checkCmd = args[i]
		case "--key-env":
			if i+1 >= len(args) {
				return fmt.Errorf("--key-env requires a value")
			}
			i++
			keyEnvVar = args[i]
		default:
			return fmt.Errorf("unknown flag: %s", args[i])
		}
	}

	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if checkCmd == "" {
		return fmt.Errorf("--check-cmd is required")
	}

	if isBuiltinEngine(name) {
		return fmt.Errorf("%q is a built-in engine and cannot be added as custom", name)
	}

	// Verify the check command works.
	info("Verifying check command...\n")
	if err := shellCommand(checkCmd).Run(); err != nil {
		return fmt.Errorf("check command failed: %w", err)
	}

	cfg, err := loadTTSConfig()
	if err != nil {
		return err
	}

	newEngine := CustomTTSEngine{
		Name:        name,
		DisplayName: name,
		CheckCmd:    checkCmd,
		KeyEnvVar:   keyEnvVar,
	}

	replaced := false
	for i, e := range cfg.Engines {
		if e.Name == name {
			cfg.Engines[i] = newEngine
			replaced = true
			break
		}
	}
	if !replaced {
		cfg.Engines = append(cfg.Engines, newEngine)
	}

	if err := saveTTSConfig(cfg); err != nil {
		return err
	}

	info("Added custom engine %q.\n", name)
	return nil
}

// handleTTSRemove removes a custom TTS engine from tts.json.
func handleTTSRemove(args []string) error {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			fmt.Printf("Usage: %s tts remove <name>\n\nRemove a custom TTS engine.\n", binName)
			return nil
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("engine name is required")
	}
	name := args[0]

	if isBuiltinEngine(name) {
		return fmt.Errorf("%q is a built-in engine and cannot be removed", name)
	}

	cfg, err := loadTTSConfig()
	if err != nil {
		return err
	}

	origLen := len(cfg.Engines)
	filtered := make([]CustomTTSEngine, 0, origLen)
	for _, e := range cfg.Engines {
		if e.Name != name {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) == origLen {
		return fmt.Errorf("custom engine %q not found", name)
	}

	cfg.Engines = filtered

	// Clear default if it was the removed engine.
	if cfg.Default == name {
		cfg.Default = ""
	}

	if err := saveTTSConfig(cfg); err != nil {
		return err
	}

	info("Removed custom engine %q.\n", name)
	return nil
}

func defaultEngineNameFrom(engines []TTSEngine) string {
	for _, e := range engines {
		if e.IsDefault {
			return e.Name
		}
	}
	return ""
}

func defaultEngineName() string {
	return defaultEngineNameFrom(detectTTSEngines())
}
