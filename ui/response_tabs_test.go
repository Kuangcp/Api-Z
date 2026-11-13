package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
	"api-z/httpclient"
)

func TestResponseTabsAndBinaryCard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Test", "yes")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Echo")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "Echo", Method: "POST", URL: srv.URL + "/echo",
		HeadersText: "X-Input: 1\nContent-Type: application/json", BodyText: `{"a":1}`})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(rid)
	ed := root.tabs[0]

	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("发送"); err != nil {
		t.Fatal(err)
	}
	root.session(rid).Wait()
	tt.Frame()

	ed.RespTab = 1
	tt.Frame()
	if !tt.HasText("X-Test") || !tt.HasText("yes") ||
		!tt.HasText("请求头") || !tt.HasText("X-Input") || !tt.HasText("响应头") {
		t.Fatalf("headers tab = %v", tt.Texts())
	}

	ed.RespTab = 2
	tt.Frame()
	if !tt.HasText("实际发送") || !tt.HasText(srv.URL+"/echo") {
		t.Fatalf("request tab = %v", tt.Texts())
	}
	if got := app.FormatHTTP(mustSpec(t, root, rid)); !strings.Contains(got, "X-Input: 1") || !strings.Contains(got, `{"a":1}`) {
		t.Fatalf("snapshot = %q", got)
	}

	// Binary card offers save/open and a derived filename.
	view := func(c *ui.Context) {
		ed.binaryView(c, app.SessionSnapshot{Binary: true, Bytes: 2048, ContentType: "image/png",
			FileName: "chart.png", SpoolPath: "/tmp/chart.raw"})
	}
	bt := ui.NewTester(view, 700, 400)
	bt.Frame()
	if !bt.HasText("二进制响应") || !bt.HasText("chart.png") || !bt.HasText("保存到文件") || !bt.HasText("打开") {
		t.Fatalf("binary card = %v", bt.Texts())
	}
}

func mustSpec(t *testing.T, root *Root, id string) httpclient.Spec {
	t.Helper()
	s, ok := root.session(id).Spec()
	if !ok {
		t.Fatal("spec not recorded")
	}
	return s
}

func TestNonJSONResponseStaysRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "plain text")
	}))
	defer srv.Close()
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Text")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "Text", Method: "GET", URL: srv.URL})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(rid)
	ed := root.tabs[0]
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if err := tt.Click("发送"); err != nil {
		t.Fatal(err)
	}
	root.session(rid).Wait()
	tt.Frame()
	if ed.jsonOpen || ed.jsonEd != nil {
		t.Fatal("non-JSON response opened the JSON view")
	}
	if !tt.HasText("plain text") {
		t.Fatalf("raw body = %v", tt.Texts())
	}
	if tt.HasText("AGUI") {
		t.Fatalf("non-SSE response showed the SSE view switch: %v", tt.Texts())
	}
}
