package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestSettingsThemeAndKeymap(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !root.settingsOpen || !tt.HasText("主题") || !tt.HasText("快捷键") {
		t.Fatalf("settings not shown: %v", tt.Texts())
	}
	if err := tt.Click("深色"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.themePrefs.Mode != 2 {
		t.Fatalf("mode = %d", root.themePrefs.Mode)
	}
	if got := app.LoadThemePrefs(root.themePath); got.Mode != 2 {
		t.Fatalf("persisted theme = %#v", got)
	}

	if err := tt.Click("快捷键"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	root.recording = "search"
	tt.KeyRepeat(ui.Ctrl|ui.Shift, ui.KeyK)
	tt.Frame()
	if got := root.keymap["search"]; got != "Ctrl+Shift+K" {
		t.Fatalf("search chord = %q", got)
	}
	if root.recording != "" {
		t.Fatal("recording did not finish")
	}
	if got := app.LoadKeymap(root.keymapPath); got["search"] != "Ctrl+Shift+K" {
		t.Fatalf("persisted keymap = %#v", got)
	}

	if err := tt.Click("关闭"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.settingsOpen {
		t.Fatal("settings still open")
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyK)
	tt.Frame()
	if !root.searchOpen {
		t.Fatalf("custom shortcut did not open search: %v", tt.Texts())
	}
}

func TestThemeResetsToSystem(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	root := NewRoot(paths, store, app.Tree{}, app.EnvState{Version: 1})
	root.themePrefs = app.ThemePrefs{Mode: 2, Accent: "#22c55e"}
	root.accentHex = root.themePrefs.Accent
	root.saveThemePrefs()
	root.resetTheme()
	if root.themePrefs.Mode != 0 || root.themePrefs.Accent != "" {
		t.Fatalf("reset = %#v", root.themePrefs)
	}
	if got := app.LoadThemePrefs(root.themePath); got.Mode != 0 || got.Accent != "" {
		t.Fatalf("persisted reset = %#v", got)
	}
}
