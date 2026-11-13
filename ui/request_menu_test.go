package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestRequestContextMenu(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	_ = store.UpdateNodeMeta(app.NodeCollection, cid,
		`{"auth":{"type":"bearer","bearer":[{"key":"token","value":"secret","type":"string"}]}}`)
	rid, _ := store.CreateRequest(cid, "", "Echo")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "Echo", Method: "POST", URL: "https://example.com/echo",
		HeadersText: "Content-Type: application/json", BodyText: `{"a":1}`})
	_ = store.SaveEnvironments(app.EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []app.Environment{{
		ID: "e1", Name: "Local", Variables: []app.EnvVariable{{Key: "host", Value: "https://api.example.com"}},
	}}})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if err := tt.RightClick("POST Echo"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	menu := strings.Join(tt.Menu(), "\n")
	for _, want := range []string{"复制请求", "复制 cURL", "移动到文件夹…", "删除…"} {
		if !strings.Contains(menu, want) {
			t.Fatalf("menu missing %q: %s", want, menu)
		}
	}
	for _, gone := range []string{"打开", "复制 URL", "重命名…", "上移", "下移"} {
		if strings.Contains(menu, gone) {
			t.Fatalf("menu still has %q: %s", gone, menu)
		}
	}
	if err := tt.ChooseMenuItem("复制 cURL"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	got := tt.Clipboard()
	for _, want := range []string{"curl -X POST 'https://example.com/echo'", "Authorization: Bearer secret", "--data-raw"} {
		if !strings.Contains(got, want) {
			t.Fatalf("curl = %q", got)
		}
	}
}

func TestRequestDoubleClickRename(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Echo")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "Echo", Method: "GET", URL: "https://example.com"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	r, ok := tt.Find("GET Echo")
	if !ok {
		t.Fatal("request row not found")
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.ClickAt(x, y)
	tt.ClickAt(x, y)
	tt.Frame()
	if !root.dialogOpen || root.dlgKind != app.NodeRequest || root.dlgID != rid {
		t.Fatalf("rename dialog = open:%v kind:%q id:%q", root.dialogOpen, root.dlgKind, root.dlgID)
	}
	if !tt.HasText("重命名") {
		t.Fatalf("dialog texts = %v", tt.Texts())
	}
}

func TestRequestMenuParseCurlBelow(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	r1, _ := store.CreateRequest(cid, "", "First")
	r2, _ := store.CreateRequest(cid, "", "Second")
	_ = store.SaveRequest(app.Request{ID: r1, Name: "First", Method: "GET", URL: "https://example.com/1"})
	_ = store.SaveRequest(app.Request{ID: r2, Name: "Second", Method: "GET", URL: "https://example.com/2"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	tt.SetClipboard(`curl 'https://api.example.com/imported'`)
	if err := tt.RightClick("GET First"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("解析 cURL"); err != nil {
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
		t.Fatalf("order = %v", names)
	}
}

func TestErrorAutoClearSequence(t *testing.T) {
	root := &Root{}
	root.showError("boom")
	seq := root.errSeq
	if root.err != "boom" {
		t.Fatalf("err = %q", root.err)
	}
	root.expireError(seq)
	if root.err != "" {
		t.Fatalf("err not cleared: %q", root.err)
	}
	root.showError("newer")
	root.expireError(seq)
	if root.err != "newer" {
		t.Fatalf("stale timer cleared newer error: %q", root.err)
	}
}
