package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestMoveRequestToFolderUI(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a, _ := store.CreateCollection("A")
	b, _ := store.CreateCollection("B")
	f1, _ := store.CreateFolder(a, "", "F1")
	f2, _ := store.CreateFolder(a, "", "F2")
	_ = f1
	r1, _ := store.CreateRequest(a, "", "R1")
	_ = store.SaveRequest(app.Request{ID: r1, Name: "R1", Method: "GET", URL: "https://example.com/r1"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("GET R1"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("移动到文件夹…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("移动「R1」") || !tt.HasText("A / F2") {
		t.Fatalf("move modal = %v", tt.Texts())
	}
	if err := tt.Click("A / F2"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("移动"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tree, _ = store.LoadTree()
	req, ok := tree.Request(r1)
	if !ok || req.FolderID != f2 || req.CollectionID != a {
		t.Fatalf("moved request = %#v", req)
	}
	if root.moveOpen {
		t.Fatal("move modal still open")
	}
	_ = b
}

func TestMoveFolderHidesSubtreeUI(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	a, _ := store.CreateCollection("A")
	parent, _ := store.CreateFolder(a, "", "Parent")
	child, _ := store.CreateFolder(a, parent, "Child")
	root := NewRoot(paths, store, app.Tree{}, app.EnvState{Version: 1})
	root.tree, _ = store.LoadTree()
	root.beginMove(app.NodeFolder, parent, "Parent")
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if !root.moveOpen {
		t.Fatal("move modal not open")
	}
	if tt.HasText("A / Parent") {
		t.Fatal("current folder should not be a destination")
	}
	if tt.HasText("A / Parent / Child") {
		t.Fatal("descendant folder should not be a destination")
	}
	if tt.HasText("A / Child") {
		t.Fatalf("descendant leaked into destinations: %v", tt.Texts())
	}
	_ = child
}
