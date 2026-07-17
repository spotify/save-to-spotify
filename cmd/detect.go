package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/spotify/save-to-spotify/auth"
	"github.com/spotify/save-to-spotify/config"
)

// TTSEngineStatus represents the readiness of a TTS engine.
type TTSEngineStatus string

const (
	TTSReady       TTSEngineStatus = "ready"
	TTSInstallable TTSEngineStatus = "installable"
	TTSNeedsKey    TTSEngineStatus = "needs_key"

	kokoroVenvName   = "kokoro-env"
	kokoroMinPyMinor = 10
	kokoroMaxPyProbe = 20
)

// TTSEngine describes a detected TTS engine and its current status.
type TTSEngine struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Status      TTSEngineStatus `json:"status"`
	KeyEnvVar   string          `json:"key_env_var,omitempty"`
	InstallHint string          `json:"install_hint,omitempty"`
	IsDefault   bool            `json:"is_default,omitempty"`
	IsCustom    bool            `json:"is_custom,omitempty"`
}

// CheckStatus represents the result of a system prerequisite check.
type CheckStatus string

const (
	CheckOK      CheckStatus = "ok"
	CheckMissing CheckStatus = "missing"
	CheckExpired CheckStatus = "expired"
)

// SystemCheck is a single item in the doctor report.
type SystemCheck struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail"`
	Action string      `json:"action,omitempty"`
}

// DoctorReport aggregates all system checks and TTS engine detection.
type DoctorReport struct {
	Platform   string        `json:"platform"`
	Checks     []SystemCheck `json:"checks"`
	NextAction string        `json:"next_action,omitempty"`
	Engines    []TTSEngine   `json:"tts_engines,omitempty"`
}

// TTSConfig is the on-disk configuration for TTS engines.
type TTSConfig struct {
	Default string            `json:"default,omitempty"`
	Engines []CustomTTSEngine `json:"engines,omitempty"`
}

// CustomTTSEngine describes a user-added TTS engine.
type CustomTTSEngine struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	CheckCmd    string `json:"check_cmd"`
	KeyEnvVar   string `json:"key_env_var,omitempty"`
}

// ttsConfigPath returns the path to tts.json inside the config directory.
func ttsConfigPath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tts.json"), nil
}

// loadTTSConfig reads tts.json from disk. Returns an empty config if the file
// does not exist.
func loadTTSConfig() (*TTSConfig, error) {
	path, err := ttsConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &TTSConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read TTS config: %w", err)
	}

	var cfg TTSConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("corrupt TTS config: %w", err)
	}
	return &cfg, nil
}

// saveTTSConfig writes the TTS config to tts.json with restricted permissions.
func saveTTSConfig(cfg *TTSConfig) error {
	path, err := ttsConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal TTS config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write TTS config: %w", err)
	}
	return nil
}

// detectTTSEngines returns the built-in TTS engines plus any custom engines
// defined in tts.json, with their current status.
func detectTTSEngines() []TTSEngine {
	engines := []TTSEngine{
		detectAPIEngine("openai", "OpenAI TTS", "OPENAI_API_KEY"),
		detectAPIEngine("elevenlabs", "ElevenLabs", "ELEVENLABS_API_KEY"),
		detectKokoro(),
	}

	cfg, err := loadTTSConfig()
	if err == nil {
		for _, ce := range cfg.Engines {
			engines = append(engines, detectCustomEngine(ce))
		}
	}

	// Only an explicit tts.json entry is a default. Auto-promoting the
	// first ready engine silently made a paid cloud provider the default
	// for anyone with an API key exported — the provider-selection flow
	// must ask instead.
	if cfg != nil && cfg.Default != "" {
		for i := range engines {
			if engines[i].Name == cfg.Default {
				engines[i].IsDefault = true
			}
		}
	}

	return engines
}

func detectAPIEngine(name, displayName, envVar string) TTSEngine {
	e := TTSEngine{Name: name, DisplayName: displayName, KeyEnvVar: envVar}
	if os.Getenv(envVar) != "" {
		e.Status = TTSReady
	} else {
		e.Status = TTSNeedsKey
	}
	return e
}

func detectKokoro() TTSEngine {
	e := TTSEngine{
		Name:        "kokoro",
		DisplayName: "Kokoro (local, free)",
		InstallHint: "save-to-spotify tts setup",
	}

	// Ready requires both the model weight files and the package
	// (importable via the venv python, falling back to system python3) —
	// the package alone cannot synthesize anything. Check the cheap glob
	// first: it short-circuits the Python process spawn on fresh installs.
	if kokoroModelsPresent() {
		pythonBin := kokoroPython()
		if exec.Command(pythonBin, "-c", "import kokoro_onnx").Run() == nil {
			e.Status = TTSReady
			return e
		}
	}
	e.Status = TTSInstallable
	return e
}

// kokoroModelPatterns are the version-tolerant file globs that constitute a
// working Kokoro install — any version found locally counts. This is the
// single source of truth for what the install looks like on disk; downloads
// resolve concrete filenames from the upstream release at runtime.
var kokoroModelPatterns = []string{"kokoro-v*.onnx", "voices-v*.bin"}

// kokoroSearchDirs returns the directories searched for kokoro model files.
func kokoroSearchDirs() []string {
	dir, err := config.ConfigDir()
	if err != nil {
		return []string{"."}
	}
	return []string{filepath.Join(dir, kokoroVenvName), dir}
}

// kokoroModelPath returns the path of the newest file matching pattern in
// the kokoro search directories, or "" when absent.
func kokoroModelPath(pattern string) string {
	for _, d := range kokoroSearchDirs() {
		if hits, _ := filepath.Glob(filepath.Join(d, pattern)); len(hits) > 0 {
			return hits[len(hits)-1]
		}
	}
	return ""
}

// kokoroModelsPresent reports whether every Kokoro model weight file exists
// in some search directory.
func kokoroModelsPresent() bool {
	for _, pattern := range kokoroModelPatterns {
		if kokoroModelPath(pattern) == "" {
			return false
		}
	}
	return true
}

// venvPython returns the path to the python executable inside a venv,
// accounting for the platform layout: bin/python3 on Unix, Scripts\python.exe
// on Windows.
func venvPython(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "python.exe")
	}
	return filepath.Join(venvDir, "bin", "python3")
}

// venvPip returns the path to the pip executable inside a venv, accounting
// for the platform layout like venvPython.
func venvPip(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "pip.exe")
	}
	return filepath.Join(venvDir, "bin", "pip")
}

// kokoroPython returns the python binary inside the kokoro venv if it exists,
// otherwise the best system python3 via findPython3.
func kokoroPython() string {
	if dir, err := config.ConfigDir(); err == nil {
		venvPy := venvPython(filepath.Join(dir, kokoroVenvName))
		if _, err := os.Stat(venvPy); err == nil {
			return venvPy
		}
	}
	p, _ := findPython3()
	return p
}

var (
	findPython3Once   sync.Once
	findPython3Result string
	findPython3Err    error
)

// findPython3 returns the path to a Python 3.10+ binary suitable for Kokoro.
// It checks versioned binaries first (python3.13 down to python3.10), then
// falls back to unversioned names — python3 on Unix; the py launcher, python,
// and python3 on Windows — if they're 3.10+. Returns an error when no
// suitable binary can be found. The result is cached for the process
// lifetime: PATH doesn't change mid-invocation, and the probe spawns
// subprocesses.
func findPython3() (string, error) {
	findPython3Once.Do(func() {
		findPython3Result, findPython3Err = findPython3Uncached()
	})
	return findPython3Result, findPython3Err
}

func findPython3Uncached() (string, error) {
	for v := kokoroMaxPyProbe; v >= kokoroMinPyMinor; v-- {
		name := fmt.Sprintf("python3.%d", v)
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	candidates := []string{"python3"}
	if runtime.GOOS == "windows" {
		// py (the official launcher) first — python3 is often the
		// Microsoft Store stub, and python may be Python 2.
		candidates = []string{"py", "python", "python3"}
	}
	// firstFound remembers the first interpreter located on PATH so the
	// error can name a concrete binary even when every candidate is too old.
	firstFound := ""
	for _, name := range candidates {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if firstFound == "" {
			firstFound = p
		}
		if pythonVersionOK(p) {
			return p, nil
		}
	}
	if firstFound != "" {
		return firstFound, fmt.Errorf("python found at %s but is older than 3.%d", firstFound, kokoroMinPyMinor)
	}
	fallback := "python3"
	if runtime.GOOS == "windows" {
		fallback = "python"
	}
	return fallback, fmt.Errorf("%s not found on PATH", fallback)
}

func pythonVersionOK(bin string) bool {
	out, err := exec.Command(bin, "-c",
		"import sys; print(sys.version_info.major, sys.version_info.minor)").Output()
	if err != nil {
		return false
	}
	var major, minor int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%d %d", &major, &minor); err != nil {
		return false
	}
	return major > 3 || (major == 3 && minor >= kokoroMinPyMinor)
}

func detectCustomEngine(ce CustomTTSEngine) TTSEngine {
	e := TTSEngine{
		Name:        ce.Name,
		DisplayName: ce.DisplayName,
		KeyEnvVar:   ce.KeyEnvVar,
		IsCustom:    true,
	}

	// Check key first if required.
	if ce.KeyEnvVar != "" && os.Getenv(ce.KeyEnvVar) == "" {
		e.Status = TTSNeedsKey
		return e
	}

	// Run check command if provided.
	if ce.CheckCmd != "" {
		err := shellCommand(ce.CheckCmd).Run()
		if err == nil {
			e.Status = TTSReady
		} else {
			e.Status = TTSInstallable
		}
	} else {
		e.Status = TTSReady
	}
	return e
}

// shellCommand runs a user-supplied command line through the platform shell:
// sh on Unix, cmd on Windows (which has no sh).
func shellCommand(cmdStr string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", cmdStr)
	}
	return exec.Command("sh", "-c", cmdStr)
}

// detectFFmpeg checks whether ffmpeg and ffprobe are on the PATH.
func detectFFmpeg() SystemCheck {
	check := SystemCheck{Name: "ffmpeg"}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		check.Status = CheckMissing
		check.Detail = "ffmpeg not found in PATH"
		check.Action = "Install ffmpeg: https://ffmpeg.org/download.html"
		return check
	}

	if _, err := exec.LookPath("ffprobe"); err != nil {
		check.Status = CheckMissing
		check.Detail = "ffprobe not found in PATH (ffmpeg found at " + ffmpegPath + ")"
		check.Action = "Install ffmpeg with ffprobe: https://ffmpeg.org/download.html"
		return check
	}

	// Get version for detail.
	out, err := exec.Command("ffmpeg", "-version").Output()
	if err != nil {
		check.Status = CheckOK
		check.Detail = ffmpegPath
		return check
	}
	firstLine := strings.SplitN(string(out), "\n", 2)[0]
	check.Status = CheckOK
	check.Detail = firstLine
	return check
}

// detectAuth checks whether a valid auth token is available.
func detectAuth() SystemCheck {
	check := SystemCheck{Name: "auth"}

	// Environment variable token.
	if os.Getenv(config.EnvVarAuthToken) != "" {
		check.Status = CheckOK
		check.Detail = fmt.Sprintf("Authenticated via %s", config.EnvVarAuthToken)
		return check
	}

	token, err := config.LoadToken()
	if err != nil {
		check.Status = CheckMissing
		check.Detail = "Not authenticated"
		check.Action = fmt.Sprintf("Run `%s auth login`", binName)
		return check
	}

	if token.IsExpired() {
		check.Status = CheckExpired
		check.Detail = "Token expired (will auto-refresh on next use)"
		check.Action = fmt.Sprintf("Run `%s auth login` to re-authenticate", binName)
		return check
	}

	check.Status = CheckOK
	check.Detail = "Authenticated"
	return check
}

// detectPython reports whether a Kokoro-suitable Python 3.10+ is available.
// Only informational when a cloud engine is in use, but it is the first
// thing testers hit when Kokoro setup fails.
func detectPython() SystemCheck {
	check := SystemCheck{Name: "python"}
	p, err := findPython3()
	if err != nil {
		check.Status = CheckMissing
		check.Detail = err.Error()
		check.Action = "Install Python 3.10+ (brew install python@3.12 / apt install python3 / winget install Python.Python.3.12) — needed for the Kokoro voice engine"
		return check
	}
	check.Status = CheckOK
	check.Detail = p
	return check
}

// detectBinary checks the CLI binary location and PATH availability.
func detectBinary() SystemCheck {
	check := SystemCheck{Name: "binary"}

	exe, err := os.Executable()
	if err != nil {
		check.Status = CheckMissing
		check.Detail = "Could not determine executable location"
		return check
	}

	pathExe, pathErr := exec.LookPath("save-to-spotify")
	if pathErr != nil {
		check.Status = CheckMissing
		check.Detail = fmt.Sprintf("Installed at %s but not found in PATH", exe)
		check.Action = fmt.Sprintf("Add %s to your PATH", filepath.Dir(exe))
		return check
	}

	check.Status = CheckOK
	check.Detail = pathExe
	return check
}

// isHeadless returns true when running in a non-interactive environment.
// Two-tier design: this is the entry point that short-circuits on the
// unambiguous remote signal (SSH), then delegates the nuanced per-platform
// detection to canOpenBrowserWith — the parameterized, unit-tested half.
func isHeadless() bool {
	// SSH_TTY is Unix-only; Windows OpenSSH sessions set SSH_CLIENT /
	// SSH_CONNECTION but not SSH_TTY. An SSH session with a forwarded X
	// display (ssh -X, DISPLAY set) CAN open a browser on the user's
	// screen, so only short-circuit when there is no display.
	if os.Getenv("DISPLAY") == "" {
		for _, v := range []string{"SSH_TTY", "SSH_CLIENT", "SSH_CONNECTION"} {
			if os.Getenv(v) != "" {
				return true
			}
		}
	}
	return !canOpenBrowser()
}

func canOpenBrowser() bool {
	return canOpenBrowserWith(runtime.GOOS, os.Getenv, exec.LookPath)
}

// canOpenBrowserWith holds the platform/environment decision behind browser
// launches. Parameterized (rather than the package-var swapping used by the
// HTTP test seams) so tests never mutate the real process environment or
// exec.LookPath behavior.
func canOpenBrowserWith(goos string, getenv func(string) string, lookPath func(string) (string, error)) bool {
	// CI runners have working `open`/`start` commands but no one watching a
	// browser — treat CI as headless on every platform. CI is a boolean:
	// CI=false / CI=0 explicitly opts out; any other non-empty value
	// (true, 1, yes, ...) means a CI environment.
	if v := getenv("CI"); v != "" {
		if b, err := strconv.ParseBool(v); err != nil || b {
			return false
		}
	}
	switch goos {
	case "darwin":
		// `open` always exists on macOS (no point probing for it) and
		// proves nothing about launchd daemons/agents, which have no
		// WindowServer access. Interactive sessions (terminals,
		// terminal-hosted agents) carry TERM or TERM_PROGRAM; launchd's
		// minimal environment carries neither. This may misclassify
		// GUI-launched embedders (no TERM) as headless — acceptable
		// because the auth flow degrades to the manual-URL mode, and
		// programmatic callers should pass --no-browser anyway.
		return getenv("TERM") != "" || getenv("TERM_PROGRAM") != ""
	case "windows":
		// Interactive sessions carry SESSIONNAME (Console, RDP-Tcp#N);
		// services and scheduled tasks don't.
		return getenv("SESSIONNAME") != ""
	case "linux":
		// An opener binary alone proves nothing in containers and
		// services — require a display session too.
		if getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
			return false
		}
		for _, opener := range auth.LinuxOpeners {
			if _, err := lookPath(opener); err == nil {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// buildDoctorReport runs all system checks and determines the next action.
func buildDoctorReport() *DoctorReport {
	report := &DoctorReport{Platform: runtime.GOOS + "/" + runtime.GOARCH}

	binaryCheck := detectBinary()
	authCheck := detectAuth()
	ffmpegCheck := detectFFmpeg()
	pythonCheck := detectPython()
	engines := detectTTSEngines()

	report.Checks = []SystemCheck{binaryCheck, authCheck, ffmpegCheck, pythonCheck}
	report.Engines = engines

	// Determine next action by priority.
	switch {
	case binaryCheck.Status != CheckOK:
		report.NextAction = binaryCheck.Action
	case authCheck.Status == CheckMissing:
		report.NextAction = authCheck.Action
	default:
		// Check if any TTS engine is ready.
		hasReady := false
		for _, e := range engines {
			if e.Status == TTSReady {
				hasReady = true
				break
			}
		}
		if !hasReady {
			report.NextAction = fmt.Sprintf("Run `%s tts setup` to install a TTS engine", binName)
		} else if ffmpegCheck.Status != CheckOK {
			report.NextAction = ffmpegCheck.Action
		}
		// All good — leave NextAction empty.
	}

	return report
}
