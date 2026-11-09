package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestThemePrefsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := ThemePath(dir)
	p := ThemePrefs{Mode: 2, Accent: "#22c55e"}
	if err := SaveThemePrefs(path, p); err != nil {
		t.Fatal(err)
	}
	got := LoadThemePrefs(path)
	if got != p {
		t.Fatalf("got %#v", got)
	}
	if err := SaveThemePrefs(path, ThemePrefs{Mode: 1, Accent: "red"}); err == nil {
		t.Fatal("invalid accent accepted")
	}
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadThemePrefs(path); got.Mode != 0 || got.Accent != "" {
		t.Fatalf("corrupt prefs = %#v", got)
	}
	if !ValidHex("#a1B2c3") || ValidHex("#xyzxyz") || ValidHex("a1b2c3") {
		t.Fatal("ValidHex")
	}
}

func TestKeymapPrefsRoundTrip(t *testing.T) {
	path := KeymapPath(t.TempDir())
	in := KeymapPrefs{"search": "Ctrl+Shift+K"}
	if err := SaveKeymap(path, in); err != nil {
		t.Fatal(err)
	}
	got := LoadKeymap(path)
	if got["search"] != "Ctrl+Shift+K" {
		t.Fatalf("got %#v", got)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
}
