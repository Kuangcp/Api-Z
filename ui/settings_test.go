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

func TestFontSettings(t *testing.T) {
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
	if err := tt.Click("字体"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("界面字体") || !tt.HasText("界面缩放") {
		t.Fatalf("font settings not shown: %v", tt.Texts())
	}

	root.fontInput = "Noto Sans CJK SC"
	root.themePrefs.Font = root.fontInput
	root.saveThemePrefs()
	tt.Frame()
	if got := app.LoadThemePrefs(root.themePath); got.Font != "Noto Sans CJK SC" {
		t.Fatalf("persisted font = %q", got.Font)
	}

	if err := tt.Click("125%"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.themePrefs.Scale != 125 {
		t.Fatalf("scale = %d", root.themePrefs.Scale)
	}
	if got := app.LoadThemePrefs(root.themePath); got.Scale != 125 {
		t.Fatalf("persisted scale = %d", got.Scale)
	}
	if err := tt.Click("恢复默认字体与缩放"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.themePrefs.Font != "" || root.themePrefs.Scale != app.DefaultThemeScale {
		t.Fatalf("font reset = %#v", root.themePrefs)
	}
}

func TestResolveThemeFont(t *testing.T) {
	base := ui.DarkTheme()
	th := resolveTheme(base, app.ThemePrefs{Font: "Noto Sans", Scale: 100})
	if th.Font != "Noto Sans" {
		t.Fatalf("font = %q", th.Font)
	}
	th = resolveTheme(base, app.ThemePrefs{Mode: 2, Accent: "#22c55e", Font: "Serif", Scale: 100})
	if th.Font != "Serif" || th.Accent != ui.Hex("#22c55e") {
		t.Fatalf("theme = %#v", th)
	}
	for scale, want := range map[int]int{0: 1, 80: 0, 90: 0, 104: 1, 106: 2, 125: 3, 200: 3} {
		if got := themeScaleIndex(scale); got != want {
			t.Fatalf("scaleIndex(%d) = %d, want %d", scale, got, want)
		}
	}
}

func TestThemeResetsToSystem(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	root := NewRoot(paths, store, app.Tree{}, app.EnvState{Version: 1})
	root.themePrefs = app.NormalizeThemePrefs(app.ThemePrefs{Mode: 2, Accent: "#22c55e", Font: "Serif", Scale: 125})
	root.accentHex = root.themePrefs.Accent
	root.saveThemePrefs()
	root.resetTheme()
	if root.themePrefs.Mode != 0 || root.themePrefs.Accent != "" {
		t.Fatalf("reset = %#v", root.themePrefs)
	}
	if root.themePrefs.Font != "Serif" || root.themePrefs.Scale != 125 {
		t.Fatalf("reset touched font = %#v", root.themePrefs)
	}
	if got := app.LoadThemePrefs(root.themePath); got.Mode != 0 || got.Accent != "" {
		t.Fatalf("persisted reset = %#v", got)
	}
}
