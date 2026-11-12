package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestDataDirActionsUI(t *testing.T) {
	store, paths := collectionTestStore(t)
	collID, _ := store.CreateCollection("Sync")
	reqID, _ := store.CreateRequest(collID, "", "Ping")
	_ = store.SaveRequest(app.Request{ID: reqID, Name: "Ping", Method: "GET", URL: "https://example.com"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 900, 600)
	tt.Frame()

	root.pushDataDir()
	tt.Frame()
	if _, err := os.Stat(app.DataRoot(paths) + "/collection/" + collID + ".json"); err != nil {
		t.Fatalf("pushed file: %v", err)
	}
	if !strings.Contains(root.status, "已 Push") {
		t.Fatalf("push status = %q", root.status)
	}

	if err := store.DeleteCollection(collID); err != nil {
		t.Fatal(err)
	}
	root.reloadTree()
	root.pullDataDir()
	tt.Frame()
	if len(root.tree.Collections) != 1 || root.tree.Collections[0].ID != collID {
		t.Fatalf("collections after pull = %#v", root.tree.Collections)
	}
	if !strings.Contains(root.status, "拉取") {
		t.Fatalf("pull status = %q", root.status)
	}
}

func TestCopyRequestCodeUI(t *testing.T) {
	store, paths := collectionTestStore(t)
	collID, _ := store.CreateCollection("Demo")
	reqID, _ := store.CreateRequest(collID, "", "Echo")
	_ = store.SaveRequest(app.Request{
		ID: reqID, Name: "Echo", Method: "POST", URL: "https://example.com/echo",
		HeadersText: "Content-Type: application/json", BodyText: `{"a":1}`,
	})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1000, 700)
	tt.Frame()
	root.openRequest(reqID)
	tt.Frame()

	if err := tt.Click("代码"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("复制为 cURL"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	got := tt.Clipboard()
	if !strings.HasPrefix(got, "curl -X POST 'https://example.com/echo'") ||
		!strings.Contains(got, "--data-raw") || !strings.Contains(got, "Content-Type: application/json") {
		t.Fatalf("curl clipboard = %q", got)
	}

	if err := tt.Click("代码"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("复制为 Raw HTTP"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	got = tt.Clipboard()
	if !strings.HasPrefix(got, "POST /echo HTTP/1.1\nHost: example.com\n") ||
		!strings.HasSuffix(got, "\n\n{\"a\":1}") {
		t.Fatalf("raw http clipboard = %q", got)
	}
}
