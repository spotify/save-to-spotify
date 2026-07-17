package cmd

import (
	"errors"
	"testing"
)

func TestCanOpenBrowserWith(t *testing.T) {
	allBinaries := func(name string) (string, error) { return "/usr/bin/" + name, nil }
	noBinaries := func(name string) (string, error) { return "", errors.New("not found") }
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}

	tests := []struct {
		name     string
		goos     string
		env      map[string]string
		lookPath func(string) (string, error)
		want     bool
	}{
		{
			name: "darwin interactive terminal",
			goos: "darwin",
			env:  map[string]string{"TERM": "xterm-256color"},
			want: true,
		},
		{
			name: "darwin terminal-hosted agent",
			goos: "darwin",
			env:  map[string]string{"TERM_PROGRAM": "iTerm.app"},
			want: true,
		},
		{
			name: "darwin launchd daemon has no session markers",
			goos: "darwin",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "CI is headless on every platform",
			goos: "darwin",
			env:  map[string]string{"TERM": "xterm", "CI": "true"},
			want: false,
		},
		{
			name: "unparseable CI value counts as CI",
			goos: "darwin",
			env:  map[string]string{"TERM": "xterm", "CI": "yes"},
			want: false,
		},
		{
			name: "explicit CI=false is not headless",
			goos: "darwin",
			env:  map[string]string{"TERM": "xterm", "CI": "false"},
			want: true,
		},
		{
			name: "windows interactive session",
			goos: "windows",
			env:  map[string]string{"SESSIONNAME": "Console"},
			want: true,
		},
		{
			name: "windows service without session",
			goos: "windows",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "linux desktop with opener",
			goos: "linux",
			env:  map[string]string{"DISPLAY": ":0"},
			want: true,
		},
		{
			name: "linux container without display",
			goos: "linux",
			env:  map[string]string{},
			want: false,
		},
		{
			name:     "linux display but no opener binary",
			goos:     "linux",
			env:      map[string]string{"WAYLAND_DISPLAY": "wayland-0"},
			lookPath: noBinaries,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lp := tt.lookPath
			if lp == nil {
				lp = allBinaries
			}
			if got := canOpenBrowserWith(tt.goos, env(tt.env), lp); got != tt.want {
				t.Errorf("canOpenBrowserWith(%s) = %v, want %v", tt.goos, got, tt.want)
			}
		})
	}
}
