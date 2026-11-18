package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func paramsTestRequest(t *testing.T, rawURL, params string) (*Editor, *ui.Tester) {
	t.Helper()
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Echo")
	if err := store.SaveRequest(app.Request{
		ID: rid, Name: "Echo", Method: "GET", URL: rawURL, ParamsText: params,
	}); err != nil {
		t.Fatal(err)
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(rid)
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	return root.tabs[0], tt
}

func TestParamsTableParsesURL(t *testing.T) {
	ed, tt := paramsTestRequest(t, "https://api.test/pets?limit=10&sort=asc#frag", "")
	if err := tt.Click("Params"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("添加一行") || !tt.HasText("Key") || !tt.HasText("Value") {
		t.Fatalf("params table not shown: %v", tt.Texts())
	}
	if len(ed.paramsKV.Rows) != 2 || ed.paramsKV.Rows[0].Key != "limit" || ed.paramsKV.Rows[0].Value != "10" {
		t.Fatalf("rows = %#v", ed.paramsKV.Rows)
	}
	if strings.Contains(ed.URL, "?limit=10&sort=asc") == false {
		t.Fatalf("URL changed on open: %q", ed.URL)
	}

	// Editing the table rewrites the complete URL and keeps its fragment.
	ed.paramsKV.Rows[0].Value = "20"
	ed.commitParams()
	tt.Frame()
	wantURL := "https://api.test/pets?limit=20&sort=asc#frag"
	if ed.URL != wantURL {
		t.Fatalf("URL = %q, want %q", ed.URL, wantURL)
	}
	if got := ed.buildSpec().URL; got != wantURL {
		t.Fatalf("sent URL = %q, want %q", got, wantURL)
	}

	// Disabling a row removes it from the URL but keeps it in Params.
	ed.paramsKV.Rows[1].Enabled = false
	ed.commitParams()
	if ed.URL != "https://api.test/pets?limit=20#frag" {
		t.Fatalf("disabled URL = %q", ed.URL)
	}
	if !strings.Contains(ed.Params, "! sort: asc") {
		t.Fatalf("disabled params = %q", ed.Params)
	}
	if got := ed.buildSpec().URL; got != "https://api.test/pets?limit=20#frag" {
		t.Fatalf("disabled sent URL = %q", got)
	}

	// Editing the address bar feeds the table again.
	ed.URL = "https://api.test/pets?limit=30&page=2#frag"
	tt.Frame()
	if len(ed.paramsKV.Rows) != 3 || ed.paramsKV.Rows[1].Key != "page" || ed.paramsKV.Rows[1].Value != "2" ||
		ed.paramsKV.Rows[2].Enabled || ed.paramsKV.Rows[2].Key != "sort" {
		t.Fatalf("rows after URL edit = %#v", ed.paramsKV.Rows)
	}
}

func TestParamsMigrationFoldsLegacyTextIntoURL(t *testing.T) {
	ed, tt := paramsTestRequest(t, "https://api.test/pets", "limit: 10\n! secret: x")
	if !strings.Contains(ed.URL, "?limit=10") {
		t.Fatalf("migrated URL = %q", ed.URL)
	}
	if ed.Params != "! secret: x" {
		t.Fatalf("disabled params = %q", ed.Params)
	}
	if !ed.Dirty {
		t.Fatal("migration did not mark the request dirty")
	}
	if err := tt.Click("Params"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(ed.paramsKV.Rows) != 2 || ed.paramsKV.Rows[0].Key != "limit" ||
		ed.paramsKV.Rows[1].Enabled || ed.paramsKV.Rows[1].Key != "secret" {
		// URL rows come first, then the disabled legacy row.
		t.Fatalf("rows = %#v", ed.paramsKV.Rows)
	}
	if got := ed.buildSpec().URL; got != "https://api.test/pets?limit=10" {
		t.Fatalf("sent URL = %q", got)
	}
}
