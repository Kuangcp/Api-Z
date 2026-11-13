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
