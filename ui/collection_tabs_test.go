package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func collectionTestStore(t *testing.T) (*app.Store, app.Paths) {
	t.Helper()
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, paths
}

func TestCollectionTabsSwitch(t *testing.T) {
	store, paths := collectionTestStore(t)
	a, _ := store.CreateCollection("Alpha")
	b, _ := store.CreateCollection("Beta")
	ra, _ := store.CreateRequest(a, "", "AReq")
	rb, _ := store.CreateRequest(b, "", "BReq")
	_ = store.SaveRequest(app.Request{ID: ra, Name: "AReq", Method: "GET", URL: "https://example.com/a"})
	_ = store.SaveRequest(app.Request{ID: rb, Name: "BReq", Method: "POST", URL: "https://example.com/b"})
	tree, err := store.LoadTree()
	if err != nil {
		t.Fatal(err)
	}
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	if root.activeCollection != a {
		t.Fatalf("active collection = %q, want %q", root.activeCollection, a)
	}
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("AReq") || tt.HasText("BReq") {
		t.Fatalf("initial tree shows wrong collection: %v", tt.Texts())
	}
	if err := tt.Click("Beta"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.activeCollection != b {
		t.Fatalf("active collection = %q, want %q", root.activeCollection, b)
	}
	if !tt.HasText("BReq") || tt.HasText("AReq") {
		t.Fatalf("tree after switching = %v", tt.Texts())
	}
}

func TestCollectionTabsOverflowKeepsRecent(t *testing.T) {
	store, paths := collectionTestStore(t)
	names := []string{"Collection-One-Long-Name", "Collection-Two-Long-Name", "Collection-Three-Long-Name",
		"Collection-Four-Long-Name", "Collection-Five-Long-Name", "Collection-Six-Long-Name"}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		id, err := store.CreateCollection(name)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.sidebarWidth = 180
	root.activateCollection(ids[5])

	visible, hidden := root.collectionTabs()
	activeVisible := false
	for _, col := range visible {
		if col.ID == ids[5] {
			activeVisible = true
		}
	}
	if !activeVisible {
		t.Fatalf("active collection not visible: visible=%v hidden=%v", visible, hidden)
	}
	if len(hidden) == 0 {
		t.Fatalf("expected overflow, visible=%d", len(visible))
	}
	pick := hidden[0].ID
	root.activateCollection(pick)
	visible, hidden = root.collectionTabs()
	found := false
	for _, col := range visible {
		if col.ID == pick {
			found = true
		}
	}
	if !found {
		t.Fatalf("recently used collection %q hidden after activation: visible=%v hidden=%v", pick, visible, hidden)
	}
	if len(visible) > 2 {
		t.Fatalf("sidebar overflows: %d tabs in 180px", len(visible))
	}
}
