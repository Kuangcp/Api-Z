package ui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestVisibleRowsCached(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	fid, _ := store.CreateFolder(cid, "", "Many")
	for i := 0; i < 300; i++ {
		id, _ := store.CreateRequest(cid, fid, fmt.Sprintf("R%03d", i))
		_ = store.SaveRequest(app.Request{ID: id, Name: fmt.Sprintf("R%03d", i), Method: "GET", URL: "https://example.com"})
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.expanded["f:"+fid] = true
	first := root.visibleRows()
	if len(first) < 300 {
		t.Fatalf("rows = %d", len(first))
	}
	second := root.visibleRows()
	if reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
		t.Fatal("second call rebuilt the row slice")
	}
	root.search = "R29"
	if n := len(root.visibleRows()); n == len(first) || n == 0 {
		t.Fatalf("search rows = %d", n)
	}
}

func TestTreeWheelScrollsThreeRows(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	fid, _ := store.CreateFolder(cid, "", "Many")
	for i := 0; i < 80; i++ {
		id, _ := store.CreateRequest(cid, fid, fmt.Sprintf("R%03d", i))
		_ = store.SaveRequest(app.Request{ID: id, Name: fmt.Sprintf("R%03d", i), Method: "GET", URL: "https://example.com"})
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.expanded["f:"+fid] = true
	tt := ui.NewTester(root.View, 1200, 600)
	tt.Frame()
	first, _ := root.treeList.Visible()
	tt.Wheel(60, 300, 0, 40)
	tt.Frame()
	after, _ := root.treeList.Visible()
	if after < first+2 {
		t.Fatalf("wheel moved from %d to %d rows", first, after)
	}
}
