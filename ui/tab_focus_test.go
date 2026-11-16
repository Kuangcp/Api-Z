package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestTabSwitchFollowsTree(t *testing.T) {
	store, paths := collectionTestStore(t)
	a, _ := store.CreateCollection("Alpha")
	folder, _ := store.CreateFolder(a, "", "Nested")
	ra, _ := store.CreateRequest(a, folder, "AlphaReq")
	_ = store.SaveRequest(app.Request{ID: ra, Name: "AlphaReq", Method: "GET", URL: "https://example.com/a"})
	b, _ := store.CreateCollection("Beta")
	rb, _ := store.CreateRequest(b, "", "BetaReq")
	_ = store.SaveRequest(app.Request{ID: rb, Name: "BetaReq", Method: "GET", URL: "https://example.com/b"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(ra)
	root.openRequest(rb)
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if root.activeCollection != b || root.selected != rb {
		t.Fatalf("initial collection=%q selected=%q", root.activeCollection, root.selected)
	}

	if err := tt.Click("AlphaReq"); err != nil {
		t.Fatalf("click Alpha tab: %v", err)
	}
	tt.Frame()
	if root.activeCollection != a || root.selected != ra || root.tabs[root.active].ID != ra {
		t.Fatalf("after tab click collection=%q selected=%q active=%q", root.activeCollection, root.selected, root.tabs[root.active].ID)
	}
	if !root.expanded["f:"+folder] {
		t.Fatalf("folder not expanded: %v", root.expanded)
	}

	tt.Key(ui.Ctrl, ui.KeyTab)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if root.activeCollection != b || root.selected != rb || root.tabs[root.active].ID != rb {
		t.Fatalf("after Ctrl+Tab collection=%q selected=%q", root.activeCollection, root.selected)
	}
}
