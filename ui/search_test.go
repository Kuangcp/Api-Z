package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestRecentSwitchAndGlobalSearch(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cid, _ := store.CreateCollection("Demo")
	a, _ := store.CreateRequest(cid, "", "Alpha")
	b, _ := store.CreateRequest(cid, "", "Beta")
	_ = store.SaveRequest(app.Request{ID: a, Name: "Alpha", Method: "GET", URL: "https://example.com/a"})
	_ = store.SaveRequest(app.Request{ID: b, Name: "Beta", Method: "GET", URL: "https://example.com/b"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	if err := tt.Click("GET Alpha"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("GET Beta"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.tabs[root.active].ID != b {
		t.Fatalf("active = %s, want Beta", root.tabs[root.active].ID)
	}

	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if root.tabs[root.active].ID != a {
		t.Fatalf("Ctrl+Tab active = %s, want Alpha", root.tabs[root.active].ID)
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if root.tabs[root.active].ID != b {
		t.Fatalf("Ctrl+Tab again active = %s, want Beta", root.tabs[root.active].ID)
	}

	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Frame()
	if !root.searchOpen || !tt.HasText("全局搜索") {
		t.Fatalf("global search not open: %v", tt.Texts())
	}
	root.globalQuery = "Alpha"
	tt.Frame()
	if !tt.HasText("1 个结果") {
		t.Fatalf("search results = %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if root.searchOpen {
		t.Fatal("search did not close on Enter")
	}
	if root.tabs[root.active].ID != a {
		t.Fatalf("search open active = %s, want Alpha", root.tabs[root.active].ID)
	}
}
