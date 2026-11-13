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

func TestSSERawTimestampsAndEventPanel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"ok\":true}\n\n")
		fl.Flush()
	}))
	defer srv.Close()
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "SSE")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "SSE", Method: "GET", URL: srv.URL})
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

	line := root.session(rid).Line(0)
	if !strings.Contains(line, " data: {\"ok\":true}") {
		t.Fatalf("timestamped line = %q", line)
	}
	if ed.SelectedEvent != -1 {
		t.Fatalf("event selected before click: %d", ed.SelectedEvent)
	}
	if err := tt.Click(`{"ok":true}`); err != nil {
		t.Fatalf("click event: %v", err)
	}
	tt.Frame()
	if ed.SelectedEvent != 0 || ed.eventEd == nil {
		t.Fatalf("selected=%d editor=%v", ed.SelectedEvent, ed.eventEd)
	}
	if got := ed.eventEd.Buffer.String(); !strings.Contains(got, "\"ok\": true") {
		t.Fatalf("event JSON = %q", got)
	}
	if !tt.HasText("Event #1") || !tt.HasText("复制 JSON") {
		t.Fatalf("event panel = %v", tt.Texts())
	}
}
