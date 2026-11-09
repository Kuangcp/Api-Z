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
