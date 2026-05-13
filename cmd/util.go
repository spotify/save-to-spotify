package cmd

import (
	"fmt"
	"os"
)

// isSuccessStatus reports whether the HTTP status code is in the 2xx range.
func isSuccessStatus(code int) bool {
	return code >= 200 && code < 300
}

// isHelp returns true if s is a help flag or subcommand.
func isHelp(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

// SilentError exits with a non-zero code without printing an error message.
type SilentError struct{ Code int }

func (e *SilentError) Error() string { return fmt.Sprintf("exit %d", e.Code) }

// isTerminal reports whether f is connected to a terminal.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
