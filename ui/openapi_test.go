package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

const uiOpenAPIFixture = `{
  "openapi": "3.0.0",
  "info": {"title": "Demo"},
  "servers": [{"url": "http://api.test"}],
  "paths": {
    "/pets/{id}": {"get": {"tags": ["Pets"], "summary": "Get Pet"}},
    "/health": {"get": {"summary": "Health"}}
  }
}`

func TestOpenAPIImportAndRefreshUI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, uiOpenAPIFixture)
	}))
	defer srv.Close()

	paths := app.PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	root := NewRoot(paths, store, app.Tree{}, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	id, stats, err := root.importOpenAPIText(uiOpenAPIFixture, srv.URL, "Demo", &app.OpenAPIRoot{Mode: app.OpenAPIRootBaseURL})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 2 || stats.Folders != 2 {
		t.Fatalf("stats = %#v", stats)
	}
	root.reloadTree()
	root.expanded["c:"+id] = true
	for _, f := range root.tree.Folders {
		root.expanded["f:"+f.ID] = true
	}
	tt.Frame()
	if !tt.HasText("Demo") || !tt.HasText("Pets") || !tt.HasText("Get Pet") {
		t.Fatalf("tree = %v", tt.Texts())
	}

	// Local edits survive refresh.
	tree, _ := store.LoadTree()
	var req app.Request
	for _, r := range tree.Requests {
		if r.Name == "Get Pet" {
			req = r
		}
	}
	req.URL = "http://local/edited"
	if err := store.SaveRequest(req); err != nil {
		t.Fatal(err)
	}
	root.refreshOpenAPIAsync(id)
	tt.Frame()
	tree, _ = store.LoadTree()
	for _, r := range tree.Requests {
		if r.ID == req.ID && r.URL != "http://local/edited" {
			t.Fatalf("refresh overwrote URL: %#v", r)
		}
	}
	if source, _ := app.CollectionOpenAPIInfo(tree.Collections[0].MetaJSON); source != srv.URL {
		t.Fatalf("source = %q", source)
	}
}

func TestOpenAPIImportDialog(t *testing.T) {
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
	if err := tt.ChooseMenuItem("OpenAPI…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !root.openAPIOpen || !tt.HasText("导入 OpenAPI") || !tt.HasText("OpenAPI 地址") {
		t.Fatalf("dialog = %v", tt.Texts())
	}
	if !strings.Contains(root.openAPIName, "新集合") {
		t.Fatalf("default name = %q", root.openAPIName)
	}
}
