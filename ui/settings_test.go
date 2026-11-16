package ui

import (
	"strings"
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
	if !tt.HasText("界面字体") || !tt.HasText("界面缩放") ||
		!tt.HasText("左侧请求树") || !tt.HasText("请求 / 响应 Body") {
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

func TestThemeSchemeAndRadiusSettings(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	root := NewRoot(paths, store, app.Tree{}, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 900)
	tt.Frame()
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("海洋"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.themePrefs.Scheme != "ocean" {
		t.Fatalf("scheme = %q", root.themePrefs.Scheme)
	}
	if err := tt.Click("8"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.themePrefs.Radius != 8 {
		t.Fatalf("radius = %d", root.themePrefs.Radius)
	}
	got := app.LoadThemePrefs(root.themePath)
	if got.Scheme != "ocean" || got.Radius != 8 {
		t.Fatalf("persisted = %#v", got)
	}
	if bg := WindowBackgroundColor(root.themePrefs); !strings.Contains(bg, "#edf4fb") {
		t.Fatalf("window bg = %q", bg)
	}
}

func TestTreeAndBodyFontPreferences(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Echo")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "Echo", Method: "GET", URL: "https://example.com", BodyText: `{"a":1}`})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.themePrefs.TreeFont = "Inter"
	root.themePrefs.TreeFontSize = 16
	root.themePrefs.BodyFont = "JetBrains Mono"
	root.themePrefs.BodyFontSize = 15
	root.saveThemePrefs()
	got := app.LoadThemePrefs(root.themePath)
	if got.TreeFont != "Inter" || got.TreeFontSize != 16 || got.BodyFont != "JetBrains Mono" || got.BodyFontSize != 15 {
		t.Fatalf("persisted = %#v", got)
	}
	root.openRequest(rid)
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if root.treeTextSize(12.5) != 16 {
		t.Fatalf("tree size = %v", root.treeTextSize(12.5))
	}
	ed := root.tabs[0]
	if ed.bodyEd == nil || ed.bodyEd.Font.Family != "JetBrains Mono" || ed.bodyEd.Font.Size != 15 {
		t.Fatalf("body font = %#v", ed.bodyEd)
	}
}

func TestThemeAppliesEveryFrame(t *testing.T) {
	store, paths := collectionTestStore(t)
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	dark := true
	view := func(c *ui.Context) {
		root.View(c)
		dark = c.Theme().Dark
	}
	tt := ui.NewTester(view, 1000, 700)
	tt.Frame()
	root.settingsOpen = true
	root.settingsTab = 0
	tt.Frame()
	if err := tt.Click("浅色"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if dark {
		t.Fatal("light theme did not persist across frames")
	}
	if err := tt.Click("深色"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !dark {
		t.Fatal("dark theme did not persist across frames")
	}
}

func TestSyncSettingsUI(t *testing.T) {
	store, paths := collectionTestStore(t)
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 900)
	tt.Frame()
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("同步"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Git 同步") || !tt.HasText("仓库地址") || !tt.HasText("同步密钥") {
		t.Fatalf("sync settings = %v", tt.Texts())
	}
	root.syncPrefs.RepoURL = "git@example.com:me/sync.git"
	root.syncPrefs.Branch = "dev"
	root.syncPrefs.SSHKeyPath = "~/.ssh/id_test"
	root.saveSyncPrefs()
	got, warn := app.LoadSyncPrefs(root.syncPath)
	if warn != "" || got.RepoURL != root.syncPrefs.RepoURL || got.Branch != "dev" {
		t.Fatalf("saved sync prefs = %#v warn=%q", got, warn)
	}
}

func TestSyncBranchCanBeClearedInInput(t *testing.T) {
	store, paths := collectionTestStore(t)
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.syncPrefs.Branch = ""
	root.saveSyncPrefs()
	if root.syncPrefs.Branch != "" {
		t.Fatalf("input branch was forced to %q", root.syncPrefs.Branch)
	}
	got, _ := app.LoadSyncPrefs(root.syncPath)
	if got.Branch != "main" {
		t.Fatalf("persisted branch = %q", got.Branch)
	}
}

func TestRequestTimeoutSettings(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "SSE")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "SSE", Method: "GET", URL: "https://example.com"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 900)
	tt.Frame()
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("请求"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("首字节读超时") || !tt.HasText("完整请求超时") {
		t.Fatalf("request settings = %v", tt.Texts())
	}
	root.readTimeoutInput = "300"
	root.totalTimeoutInput = "900"
	if err := tt.Click("保存请求设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.requestPrefs.ReadTimeoutMS != 300000 || root.requestPrefs.TotalTimeoutMS != 900000 {
		t.Fatalf("prefs = %#v", root.requestPrefs)
	}
	got := app.LoadRequestPrefs(root.requestPrefsPath)
	if got.ReadTimeoutMS != 300000 || got.TotalTimeoutMS != 900000 {
		t.Fatalf("persisted = %#v", got)
	}
	root.openRequest(rid)
	ed := root.tabs[0]
	spec := ed.buildSpec()
	if spec.ReadTimeout.Seconds() != 300 || spec.RequestTimeout.Seconds() != 900 {
		t.Fatalf("spec timeouts = %v / %v", spec.ReadTimeout, spec.RequestTimeout)
	}
}
