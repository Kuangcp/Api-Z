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

func TestSSEEventSwitchWhilePanelOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"one\":1}\n\n")
		fmt.Fprint(w, "data: {\"two\":2}\n\n")
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

	ed.SelectedEvent = 0
	tt.Frame()
	if ed.eventEd == nil {
		t.Fatal("first event panel not built")
	}
	// Switching events while the JSON panel is open must not render a nil
	// editor panel mid-frame (it used to panic).
	if err := tt.Click(`{"two":2}`); err != nil {
		t.Fatalf("click second event: %v", err)
	}
	tt.Frame()
	if ed.SelectedEvent != 1 || ed.eventEd == nil {
		t.Fatalf("selected=%d editor=%v", ed.SelectedEvent, ed.eventEd)
	}
	if got := ed.eventEd.Buffer.String(); !strings.Contains(got, "\"two\": 2") {
		t.Fatalf("event JSON = %q", got)
	}
}

func TestHistorySSEViewsAndEventPanel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"m\",\"delta\":\"hello\"}\n\n")
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
	list, _ := store.ListExchanges(rid, 1)
	if len(list) != 1 {
		t.Fatalf("exchanges = %d", len(list))
	}
	ed.openHistory(list[0].HARPath)
	tt.Frame()
	if len(ed.histEvents) != 2 || len(ed.histAguiRows) == 0 {
		t.Fatalf("history parse events=%d agui=%d", len(ed.histEvents), len(ed.histAguiRows))
	}
	if !tt.HasText("历史响应视图") {
		t.Fatalf("history view selector missing: %v", tt.Texts())
	}
	ed.histSelectedEvent = 0
	tt.Frame()
	if ed.histEventEd == nil {
		t.Fatal("history event panel not built")
	}
	if !tt.HasText("Event #1") || !tt.HasText("复制 JSON") {
		t.Fatalf("history event panel = %v", tt.Texts())
	}
	ed.histSelectedEvent = 1
	tt.Frame()
	if ed.histEventEd == nil || !strings.Contains(ed.histEventEd.Buffer.String(), `"ok": true`) {
		t.Fatalf("second history event = %v", ed.histEventEd)
	}
	ed.histStreamMode = 1
	tt.Frame()
	if len(ed.histModelLines) == 0 || !tt.HasText("hello") {
		t.Fatalf("history model view = %v", tt.Texts())
	}
	ed.histStreamMode = 2
	tt.Frame()
	if len(ed.histAguiRows) == 0 || !tt.HasText("hello") {
		t.Fatalf("history AGUI view = %v", tt.Texts())
	}
}
