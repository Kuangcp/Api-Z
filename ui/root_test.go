package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestRootRequestLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cid, err := store.CreateCollection("Demo")
	if err != nil {
		t.Fatal(err)
	}
	rid, err := store.CreateRequest(cid, "", "Echo")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRequest(app.Request{
		ID: rid, Name: "Echo", Method: "GET", URL: srv.URL,
		HeadersText: "Accept: application/json",
	}); err != nil {
		t.Fatal(err)
	}
	tree, err := store.LoadTree()
	if err != nil {
		t.Fatal(err)
	}
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()
	if !tt.HasText("Api-Z") || !tt.HasText("Demo") || !tt.HasText("GET Echo") {
		t.Fatalf("initial texts = %v", tt.Texts())
	}
	if err := tt.Click("GET Echo"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("发送") {
		t.Fatalf("request pane not shown: %v", tt.Texts())
	}
	if err := tt.Click("发送"); err != nil {
		t.Fatal(err)
	}
	sess := root.session(rid)
	sess.Wait()
	tt.Frame()
	if !tt.HasText("200 OK") {
		t.Fatalf("status not shown: %v", tt.Texts())
	}
	if !tt.HasText(`{"ok":true}`) {
		t.Fatalf("body not shown: %v", tt.Texts())
	}
	if err := tt.Click("JSON"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(root.tabs) == 0 || root.tabs[0].jsonEd == nil {
		t.Fatal("json editor not built")
	}
	if got := root.tabs[0].jsonEd.Buffer.String(); !strings.Contains(got, `"ok": true`) {
		t.Fatalf("pretty JSON = %q", got)
	}
	if err := tt.Click("历史"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	menu := tt.Menu()
	if len(menu) == 0 {
		t.Fatal("history menu is empty")
	}
	if err := tt.ChooseMenuItem(menu[len(menu)-1]); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("返回实时") {
		t.Fatalf("history view not shown: %v", tt.Texts())
	}
	if err := tt.Click("返回实时"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("200 OK") {
		t.Fatalf("live view not restored: %v", tt.Texts())
	}
}

func TestRootAguiLargeStream(t *testing.T) {
	const chunk = "012345678901234567890123456789012345678901234567890123456789" // 60 bytes
	const want = 600_000
	big := strings.Repeat(chunk, want/len(chunk))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"RUN_STARTED\",\"runId\":\"r\",\"threadId\":\"t\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_START\",\"messageId\":\"m\",\"role\":\"assistant\"}\n\n")
		fl.Flush()
		fmt.Fprintf(w, "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"m\",\"delta\":%q}\n\n", big)
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_END\",\"messageId\":\"m\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\"}\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	paths := app.PathsAt(t.TempDir(), t.TempDir())
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Large")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "Large", Method: "GET", URL: srv.URL,
		HeadersText: "Accept: text/event-stream"})
	tree, err := store.LoadTree()
	if err != nil {
		t.Fatal(err)
	}
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1400, 900)
	tt.Frame()
	if err := tt.Click("GET Large"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("发送"); err != nil {
		t.Fatal(err)
	}
	sess := root.session(rid)
	sess.Wait()
	tt.Frame()
	if err := tt.Click("AGUI"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	agui := sess.AguiSnapshot()
	if len(agui.Messages) != 1 {
		t.Fatalf("messages = %d", len(agui.Messages))
	}
	if got := len(agui.Messages[0].Text); got != want {
		t.Fatalf("text bytes = %d, want %d", got, want)
	}

	times := make([]time.Duration, 0, 12)
	for range 12 {
		start := time.Now()
		tt.Frame()
		times = append(times, time.Since(start))
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	median := times[len(times)/2]
	t.Logf("AGUI frame median (600KB): %s, max: %s", median, times[len(times)-1])
	if median > 16*time.Millisecond {
		t.Fatalf("AGUI frame median %s exceeds 16ms budget", median)
	}
}
