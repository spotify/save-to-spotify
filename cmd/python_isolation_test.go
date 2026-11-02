package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// A module in the working directory must not shadow the real one when we
// spawn Python.
func TestPythonCommandIgnoresWorkingDirModules(t *testing.T) {
	py, err := findPython3()
	if err != nil {
		t.Skip("no python3 available")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "hijacked")
	payload := fmt.Sprintf("open(%q, 'w').write('x')\n", filepath.ToSlash(marker))
	if err := os.WriteFile(filepath.Join(dir, "json.py"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := pythonCommand(py, "-c", "import json")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("python failed: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("working-directory json.py was imported; isolated mode not applied")
	}
}
