package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestCurlImportUI(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	cid, _ := store.CreateCollection("Demo")
	root := NewRoot(paths, store, app.Tree{}, app.EnvState{Version: 1})
	root.reloadTree()
	root.selected = "c:" + cid
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	cmd := `curl 'https://api.test/pets?x=1' -H 'Accept: application/json' -d 'a=1'`
	id, err := root.importCurlText(cmd)
	if err != nil {
		t.Fatal(err)
	}
	root.openRequest(id)
	tt.Frame()
	tree, _ := store.LoadTree()
	req, ok := tree.Request(id)
	if !ok || req.Method != "POST" || req.URL != "https://api.test/pets?x=1" ||
		req.HeadersText != "Accept: application/json" || req.BodyText != "a=1" {
		t.Fatalf("request = %#v", req)
	}
	if !tt.HasText("POST") {
		t.Fatalf("editor = %v", tt.Texts())
	}
}

func TestCurlDialog(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	root := NewRoot(paths, store, app.Tree{}, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("导入"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("cURL…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !root.curlOpen || !tt.HasText("导入 cURL") {
		t.Fatalf("dialog = %v", tt.Texts())
	}
	if !strings.Contains(root.curlErr, "") {
		t.Fatalf("unexpected err %q", root.curlErr)
	}
}

func TestCurlImportFromClipboardTitlebar(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	fid, _ := store.CreateFolder(cid, "", "Pets")
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.selected = "f:" + fid
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	tt.SetClipboard(`curl 'https://api.example.com/pets?x=1' -H 'Accept: application/json' -d 'a=1'`)
	if err := tt.Click("从剪贴板导入 cURL"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tree, _ = store.LoadTree()
	if len(tree.Requests) != 1 {
		t.Fatalf("requests = %#v", tree.Requests)
	}
	req := tree.Requests[0]
	if req.FolderID != fid || req.Method != "POST" || req.URL != "https://api.example.com/pets?x=1" ||
		req.HeadersText != "Accept: application/json" || req.BodyText != "a=1" {
		t.Fatalf("request = %#v", req)
	}
	if len(root.tabs) != 1 || root.tabs[0].ID != req.ID {
		t.Fatalf("tabs = %#v", root.tabs)
	}
}

func TestCurlImportFromClipboardRoot(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.selected = ""
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	tt.SetClipboard(`curl 'https://api.example.com/root'`)
	if err := tt.Click("从剪贴板导入 cURL"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tree, _ = store.LoadTree()
	if len(tree.Requests) != 1 || tree.Requests[0].FolderID != "" || tree.Requests[0].CollectionID != cid {
		t.Fatalf("requests = %#v", tree.Requests)
	}
}

func TestCurlImportFromFolderContextMenu(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	fid, _ := store.CreateFolder(cid, "", "Pets")
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	tt.SetClipboard(`curl 'https://api.example.com/ctx'`)
	if err := tt.RightClick("Pets"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("解析 cURL"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tree, _ = store.LoadTree()
	if len(tree.Requests) != 1 || tree.Requests[0].FolderID != fid {
		t.Fatalf("requests = %#v", tree.Requests)
	}
}

func TestCurlImportBelowSelectedRequest(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	r1, _ := store.CreateRequest(cid, "", "First")
	r2, _ := store.CreateRequest(cid, "", "Second")
	_ = store.SaveRequest(app.Request{ID: r1, Name: "First", Method: "GET", URL: "https://example.com/1"})
	_ = store.SaveRequest(app.Request{ID: r2, Name: "Second", Method: "GET", URL: "https://example.com/2"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.selected = r1
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	tt.SetClipboard(`curl 'https://api.example.com/imported'`)
	if err := tt.Click("从剪贴板导入 cURL"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tree, _ = store.LoadTree()
	var names []string
	for _, req := range tree.Requests {
		if req.CollectionID == cid && req.FolderID == "" {
			names = append(names, req.Name)
		}
	}
	if len(names) != 3 || names[0] != "First" || names[1] != "api.example.com/imported" || names[2] != "Second" {
		t.Fatalf("order = %v (requests %#v)", names, tree.Requests)
	}
}
