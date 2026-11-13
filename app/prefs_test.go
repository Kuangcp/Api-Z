package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestThemePrefsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := ThemePath(dir)
	p := ThemePrefs{Mode: 2, Accent: "#22c55e", Font: "Noto Sans CJK SC", Scale: 110,
		TreeFont: "Inter", TreeFontSize: 12, BodyFont: "JetBrains Mono", BodyFontSize: 14}
	if err := SaveThemePrefs(path, p); err != nil {
		t.Fatal(err)
	}
	got := LoadThemePrefs(path)
	if got != p {
		t.Fatalf("got %#v", got)
	}
	if n := NormalizeThemePrefs(ThemePrefs{Scale: 10}); n.Scale != MinThemeScale {
		t.Fatalf("low scale = %d", n.Scale)
	}
	if n := NormalizeThemePrefs(ThemePrefs{Scale: 500, Font: "  "}); n.Scale != MaxThemeScale || n.Font != "" {
		t.Fatalf("high scale = %#v", n)
	}
	if n := NormalizeThemePrefs(ThemePrefs{TreeFontSize: 99, BodyFontSize: -1, TreeFont: "  "}); n.TreeFontSize != 0 || n.BodyFontSize != 0 || n.TreeFont != "" {
		t.Fatalf("font normalize = %#v", n)
	}
	if err := SaveThemePrefs(path, ThemePrefs{Scale: 500}); err != nil {
		t.Fatal(err)
	}
	if got := LoadThemePrefs(path); got.Scale != MaxThemeScale {
		t.Fatalf("clamped scale = %d", got.Scale)
	}
	if err := SaveThemePrefs(path, ThemePrefs{Mode: 1, Accent: "red"}); err == nil {
		t.Fatal("invalid accent accepted")
	}
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadThemePrefs(path); got.Mode != 0 || got.Accent != "" || got.Scale != DefaultThemeScale {
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

func TestRecentRequestsRoundTrip(t *testing.T) {
	path := RecentPath(t.TempDir())
	var ids []string
	for i := 0; i < 60; i++ {
		ids = append(ids, fmt.Sprintf("r%02d", i))
	}
	ids = append(ids, "r00", "")
	if err := SaveRecentRequests(path, ids); err != nil {
		t.Fatal(err)
	}
	got := LoadRecentRequests(path)
	if len(got) != MaxRecentRequests || got[0] != "r00" || got[len(got)-1] != "r49" {
		t.Fatalf("recent = %#v", got)
	}
}
