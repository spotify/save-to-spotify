package cmd

import (
	"fmt"
	"strings"

	"github.com/spotify/save-to-spotify/config"
)

func printDoctorUsage() {
	fmt.Printf("Usage: %s doctor\n\nCheck system readiness — binary, auth, TTS, ffmpeg.\n", binName)
}

func handleDoctor(args []string) error {
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			printDoctorUsage()
			return nil
		default:
			return fmt.Errorf("unknown flag: %s", arg)
		}
	}

	report := buildDoctorReport()

	if config.JSONMode() {
		return printJSON(report)
	}

	for _, check := range report.Checks {
		icon := statusIcon(check.Status)
		fmt.Printf("  %s %s    %s\n", icon, check.Name, check.Detail)
	}

	fmt.Println()
	fmt.Println("TTS engines:")
	for _, engine := range report.Engines {
		switch engine.Status {
		case TTSReady:
			suffix := engineSuffix(engine)
			fmt.Printf("  ✓ %s    ready%s\n", engine.DisplayName, suffix)
		case TTSNeedsKey:
			fmt.Printf("  ! %s    needs key (set %s)\n", engine.DisplayName, engine.KeyEnvVar)
		case TTSInstallable:
			fmt.Printf("  ! %s    installable — run `%s`\n", engine.DisplayName, engine.InstallHint)
		}
	}

	fmt.Println()
	if report.NextAction != "" {
		fmt.Printf("Next: %s\n", report.NextAction)
	} else {
		fmt.Println("All systems ready.")
	}

	return nil
}

func engineSuffix(e TTSEngine) string {
	var tags []string
	if e.IsDefault {
		tags = append(tags, "default")
	}
	if e.IsCustom {
		tags = append(tags, "custom")
	}
	if len(tags) == 0 {
		return ""
	}
	return " (" + strings.Join(tags, ", ") + ")"
}

func statusIcon(s CheckStatus) string {
	switch s {
	case CheckOK:
		return "✓"
	case CheckMissing:
		return "✗"
	case CheckExpired:
		return "!"
	default:
		return "?"
	}
}
