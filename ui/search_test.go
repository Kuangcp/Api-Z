package ui

import (
	"fmt"
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
	fid, _ := store.CreateFolder(cid, "", "Folder")
	a, _ := store.CreateRequest(cid, fid, "Alpha")
	b, _ := store.CreateRequest(cid, "", "Beta")
	_ = store.SaveRequest(app.Request{ID: a, Name: "Alpha", Method: "GET", URL: "https://example.com/a"})
	_ = store.SaveRequest(app.Request{ID: b, Name: "Beta", Method: "GET", URL: "https://example.com/b"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.expanded["f:"+fid] = true
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
	if !root.recentOpen || !tt.HasText("最近请求") {
		t.Fatalf("recent overlay not open: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if root.tabs[root.active].ID != a {
		t.Fatalf("Ctrl+Tab active = %s, want Alpha", root.tabs[root.active].ID)
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
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
	if root.selected != a || !root.expanded["f:"+fid] {
		t.Fatalf("tree not revealed: selected=%q expanded=%v", root.selected, root.expanded)
	}
}

func TestGlobalSearchDefaultsToRecent(t *testing.T) {
	store, paths := collectionTestStore(t)
	a, _ := store.CreateCollection("Alpha")
	ra, _ := store.CreateRequest(a, "", "AlphaReq")
	_ = store.SaveRequest(app.Request{ID: ra, Name: "AlphaReq", Method: "GET", URL: "https://example.com/a"})
	b, _ := store.CreateCollection("Beta")
	rb, _ := store.CreateRequest(b, "", "BetaReq")
	_ = store.SaveRequest(app.Request{ID: rb, Name: "BetaReq", Method: "POST", URL: "https://example.com/b"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(ra)
	root.openRequest(rb)
	if root.mru[0] != rb {
		t.Fatalf("mru = %v", root.mru)
	}
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Frame()
	if !tt.HasText("最近请求") || !tt.HasText("BetaReq") || !tt.HasText("AlphaReq") {
		t.Fatalf("recent search = %v", tt.Texts())
	}

	tree, _ = store.LoadTree()
	again := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	if again.mru[0] != rb {
		t.Fatalf("persisted mru = %v", again.mru)
	}
}

func TestRecentOverlayCapsAtTwenty(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	var ids []string
	for i := 0; i < 25; i++ {
		id, _ := store.CreateRequest(cid, "", fmt.Sprintf("R%02d", i))
		_ = store.SaveRequest(app.Request{ID: id, Name: fmt.Sprintf("R%02d", i), Method: "GET", URL: "https://example.com"})
		ids = append(ids, id)
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.mru = ids
	items := root.recentRequests()
	if len(items) != 20 || items[0].ID != ids[0] || items[19].ID != ids[19] {
		t.Fatalf("recent items = %d", len(items))
	}
}

func TestRecentOverlayCyclesWithKeyboard(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	r0, _ := store.CreateRequest(cid, "", "R0")
	r1, _ := store.CreateRequest(cid, "", "R1")
	r2, _ := store.CreateRequest(cid, "", "R2")
	for _, id := range []string{r0, r1, r2} {
		_ = store.SaveRequest(app.Request{ID: id, Name: id, Method: "GET", URL: "https://example.com/" + id})
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(r0)
	root.openRequest(r1)
	root.openRequest(r2)
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if !root.recentOpen || root.recentSel != 1 {
		t.Fatalf("overlay = open %v sel %d", root.recentOpen, root.recentSel)
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if root.recentSel != 2 {
		t.Fatalf("Ctrl+Tab sel = %d, want 2", root.recentSel)
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyTab)
	tt.Frame()
	if root.recentSel != 1 {
		t.Fatalf("Shift+Ctrl+Tab sel = %d, want 1", root.recentSel)
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyTab)
	tt.Frame()
	if root.recentSel != 0 {
		t.Fatalf("Shift+Ctrl+Tab wrap sel = %d, want 0", root.recentSel)
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if root.recentSel != 1 || !root.recentOpen {
		t.Fatalf("Ctrl+Tab after wrap = open %v sel %d", root.recentOpen, root.recentSel)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if root.recentOpen || root.active < 0 || root.tabs[root.active].ID != r1 {
		t.Fatalf("Enter active = %d open=%v", root.active, root.recentOpen)
	}
}

func TestRecentOverlayOpensOnModifierRelease(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	r0, _ := store.CreateRequest(cid, "", "R0")
	r1, _ := store.CreateRequest(cid, "", "R1")
	r2, _ := store.CreateRequest(cid, "", "R2")
	for _, id := range []string{r0, r1, r2} {
		_ = store.SaveRequest(app.Request{ID: id, Name: id, Method: "GET", URL: "https://example.com/" + id})
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(r0)
	root.openRequest(r1)
	root.openRequest(r2)
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	tt.HoldModifiers(ui.Ctrl)
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if !root.recentOpen || root.recentSel != 1 {
		t.Fatalf("overlay = open %v sel %d", root.recentOpen, root.recentSel)
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	if root.recentSel != 2 {
		t.Fatalf("Ctrl+Tab sel = %d, want 2", root.recentSel)
	}

	// Releasing only Shift must not commit; the opening modifier is Ctrl.
	tt.HoldModifiers(ui.Ctrl | ui.Shift)
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyTab)
	tt.Frame()
	tt.HoldModifiers(ui.Ctrl)
	tt.Frame()
	if !root.recentOpen {
		t.Fatal("Shift release closed the switcher")
	}
	if root.recentSel != 1 {
		t.Fatalf("Shift+Ctrl+Tab sel = %d, want 1", root.recentSel)
	}

	tt.HoldModifiers(0)
	tt.Frame()
	if root.recentOpen {
		t.Fatal("Ctrl release did not close the switcher")
	}
	if root.active < 0 || root.tabs[root.active].ID != r1 {
		t.Fatalf("Ctrl release active = %d, want R1", root.active)
	}
}
