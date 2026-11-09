package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestChordParseFormat(t *testing.T) {
	cases := []string{"Ctrl+Enter", "Ctrl+Shift+K", "Ctrl+Tab", "Ctrl+,", "Alt+F2"}
	for _, s := range cases {
		mods, key, ok := parseChord(s)
		if !ok {
			t.Fatalf("parse %q failed", s)
		}
		if got := formatChord(mods, key); got != s {
			t.Fatalf("format %q = %q", s, got)
		}
	}
	if _, _, ok := parseChord("Ctrl+Nope"); ok {
		t.Fatal("invalid key accepted")
	}
	if _, _, ok := parseChord(""); ok {
		t.Fatal("empty chord accepted")
	}
}

func TestResolveTheme(t *testing.T) {
	base := ui.LightTheme()
	got := resolveTheme(base, app.ThemePrefs{Mode: 2, Accent: "#22c55e"})
	if !got.Dark {
		t.Fatal("dark mode not applied")
	}
	if got.Accent != ui.Hex("#22c55e") {
		t.Fatalf("accent = %#v", got.Accent)
	}
	got = resolveTheme(base, app.ThemePrefs{Mode: 0})
	if got.Dark {
		t.Fatal("system mode should keep base")
	}
}
