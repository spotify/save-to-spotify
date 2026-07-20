package cmd

import (
	"fmt"

	"github.com/spotify/save-to-spotify/config"
)

func printSetupUsage() {
	fmt.Printf(`Usage: %s setup [flags]

Set up auth and voice engine (first-run onboarding).

Flags:
  --no-browser     Don't open a browser (for headless/remote servers)
`, binName)
}

func handleSetup(args []string) error {
	noBrowser := false

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			printSetupUsage()
			return nil
		case "--no-browser":
			noBrowser = true
		default:
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	if config.JSONMode() {
		return printJSON(buildDoctorReport())
	}

	fmt.Println("Setting up save-to-spotify...")

	// 1/3 · Auth
	fmt.Println()
	fmt.Println("1/3 · Auth")
	authCheck := detectAuth()
	if authCheck.Status == CheckOK {
		fmt.Println("  ✓ Already authenticated.")
	} else {
		var loginArgs []string
		if noBrowser || isHeadless() {
			loginArgs = []string{"--no-browser"}
		}
		if err := handleLogin(loginArgs); err != nil {
			return err
		}
	}

	// 2/3 · Voice engine
	fmt.Println()
	fmt.Println("2/3 · Voice engine")
	fmt.Println("  Checking for TTS engines...")
	engines := detectTTSEngines()
	hasReady := false
	for _, e := range engines {
		if e.Status == TTSReady {
			hasReady = true
			break
		}
	}
	if hasReady {
		for _, e := range engines {
			if e.Status == TTSReady {
				fmt.Printf("  ✓ %s — ready to use.\n", e.DisplayName)
			}
		}
	} else {
		fmt.Println("  ! No TTS engine found.")
		fmt.Printf("    Install Kokoro (free, local, ~340 MB): %s tts setup\n", binName)
		fmt.Println("    Or set a cloud TTS API key (OPENAI_API_KEY or ELEVENLABS_API_KEY)")
	}

	// 3/3 · ffmpeg
	fmt.Println()
	fmt.Println("3/3 · ffmpeg")
	ffmpegCheck := detectFFmpeg()
	if ffmpegCheck.Status == CheckOK {
		fmt.Println("  ✓ ffmpeg found.")
	} else {
		fmt.Println("  ! ffmpeg not found. Install it: brew install ffmpeg (macOS), apt install ffmpeg (Linux), or winget install ffmpeg (Windows)")
	}

	fmt.Println()
	fmt.Printf("Setup complete. Run `%s doctor` to verify everything.\n", binName)
	return nil
}
