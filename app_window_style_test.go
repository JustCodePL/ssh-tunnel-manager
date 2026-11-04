package main

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"ssh-tunnel-manager/internal/prefs"
)

// The macOS title bar is painted from Go, so it has to follow the stylesheet.
func TestWindowBackgroundMatchesThemeSurface(t *testing.T) {
	css, err := os.ReadFile("frontend/src/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{prefs.ThemeTerminal, prefs.ThemeGraphite} {
		pattern := regexp.MustCompile(`\[data-theme="` + theme + `"\] \{[^}]*?--surface: #([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2});`)
		match := pattern.FindSubmatch(css)
		if match == nil {
			t.Fatalf("no --surface for theme %s", theme)
		}
		want := application.NewRGBA(hexByte(t, match[1]), hexByte(t, match[2]), hexByte(t, match[3]), 255)
		if got := windowBackground("darwin", theme); got != want {
			t.Errorf("theme %s: title bar %v, stylesheet %v", theme, got, want)
		}
	}
}

func TestWindowBackgroundStaysDarkOutsideMac(t *testing.T) {
	want := application.NewRGBA(10, 10, 10, 255)
	for _, goos := range []string{"windows", "linux"} {
		if got := windowBackground(goos, prefs.ThemeGraphite); got != want {
			t.Errorf("%s: %v, want %v", goos, got, want)
		}
	}
}

func hexByte(t *testing.T, value []byte) uint8 {
	t.Helper()
	parsed, err := strconv.ParseUint(string(value), 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	return uint8(parsed)
}
