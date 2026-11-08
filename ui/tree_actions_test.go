package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestTreeCreateRenameDelete(t *testing.T) {
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

	if err := tt.Click("新建集合"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("重命名") {
		t.Fatalf("rename dialog not shown: %v", tt.Texts())
	}
	root.dlgName = "Demo"
	tt.Frame()
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(root.tree.Collections) != 1 || root.tree.Collections[0].Name != "Demo" {
		t.Fatalf("collections = %#v", root.tree.Collections)
	}

	if err := tt.RightClick("Demo"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("新建请求"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("重命名") {
		t.Fatalf("request rename dialog not shown: %v", tt.Texts())
	}
	root.dlgName = "Req"
	tt.Frame()
	if err := tt.Click("应用"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(root.tree.Requests) != 1 || root.tree.Requests[0].Name != "Req" {
		t.Fatalf("requests = %#v", root.tree.Requests)
	}
	if len(root.tabs) != 1 || root.tabs[0].Name != "Req" {
		t.Fatalf("tabs = %#v", root.tabs)
	}

	if err := tt.RightClick("GET Req"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("删除…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("删除确认") {
		t.Fatalf("delete dialog not shown: %v", tt.Texts())
	}
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(root.tree.Requests) != 0 || len(root.tabs) != 0 {
		t.Fatalf("after delete requests=%d tabs=%d", len(root.tree.Requests), len(root.tabs))
	}
}
