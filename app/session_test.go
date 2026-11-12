package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api-z/httpclient"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	p := PathsAt(t.TempDir(), t.TempDir())
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSessionStreamsSSE(t *testing.T) {
	const n = 1000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"RUN_STARTED\",\"runId\":\"r\",\"threadId\":\"t\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_START\",\"messageId\":\"m\",\"role\":\"assistant\"}\n\n")
		fl.Flush()
		for i := range n {
			fmt.Fprintf(w, "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"m\",\"delta\":\"x%d\"}\n\n", i)
			if i%50 == 0 {
				fl.Flush()
			}
		}
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_END\",\"messageId\":\"m\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\"}\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	paths := testPaths(t)
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	s := NewSession("req-sse", paths, store, nil)
	s.Start(httpclient.Spec{
		Method:         "GET",
		URL:            srv.URL,
		Headers:        []httpclient.Header{{Name: "Accept", Value: "text/event-stream"}},
		RequestTimeout: 5 * time.Second,
	})
	s.Wait()

	snap := s.Snapshot()
	if !snap.Done || snap.Running {
		t.Fatalf("done = %v running = %v", snap.Done, snap.Running)
	}
	if snap.Status != 200 {
		t.Fatalf("status = %d (%s)", snap.Status, snap.Error)
	}
	if !snap.IsSSE || snap.Events != n+4 {
		t.Fatalf("sse = %v events = %d, want %d", snap.IsSSE, snap.Events, n+4)
	}
	if snap.LineCount < n*2 {
		t.Fatalf("lines = %d, want at least %d", snap.LineCount, n*2)
	}
	agui := s.AguiSnapshot()
	if len(agui.Messages) != 1 || !strings.HasPrefix(agui.Messages[0].Text, "x0") {
		t.Fatalf("agui messages = %#v", agui.Messages)
	}
	if !strings.Contains(s.ModelText(), "x0") {
		t.Fatalf("model text missing x0")
	}
	if _, err := os.Stat(snap.SpoolPath); err != nil {
		t.Fatalf("spool: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.ResponseDir("req-sse"))); err != nil {
		t.Fatal(err)
	}
	exchanges, err := store.ListExchanges("req-sse", 10)
	if err != nil || len(exchanges) != 1 {
		t.Fatalf("exchanges = %d, err = %v", len(exchanges), err)
	}
	if exchanges[0].EventCount != n+4 || exchanges[0].Status != 200 {
		t.Fatalf("exchange = %#v", exchanges[0])
	}
}

func TestSessionCancelDoesNotSave(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"RUN_STARTED\"}\n\n")
		fl.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
				fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"delta\":\"x\"}\n\n")
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	paths := testPaths(t)
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	s := NewSession("req-cancel", paths, store, nil)
	s.Start(httpclient.Spec{Method: "GET", URL: srv.URL})
	time.Sleep(60 * time.Millisecond)
	s.Cancel()
	s.Wait()
	snap := s.Snapshot()
	if !snap.Cancelled || !snap.Done {
		t.Fatalf("cancelled = %v done = %v", snap.Cancelled, snap.Done)
	}
	if n, _ := store.Count("exchanges"); n != 0 {
		t.Fatalf("cancel saved an exchange: %d", n)
	}
}

func TestSessionTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "late")
	}))
	defer srv.Close()

	paths := testPaths(t)
	s := NewSession("req-timeout", paths, nil, nil)
	s.Start(httpclient.Spec{Method: "GET", URL: srv.URL, RequestTimeout: 50 * time.Millisecond})
	s.Wait()
	snap := s.Snapshot()
	if !snap.Done || !snap.Failed {
		t.Fatalf("done = %v failed = %v err = %q", snap.Done, snap.Failed, snap.Error)
	}
}

func TestSessionManyEvents(t *testing.T) {
	const n = 10_000
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"type\":\"RUN_STARTED\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_START\",\"messageId\":\"m\",\"role\":\"assistant\"}\n\n")
		for i := range n {
			fmt.Fprintf(w, "data: {\"type\":\"TEXT_MESSAGE_CONTENT\",\"messageId\":\"m\",\"delta\":\"0123456789\"}\n\n")
			if i%200 == 0 {
				fl.Flush()
			}
		}
		fmt.Fprint(w, "data: {\"type\":\"TEXT_MESSAGE_END\",\"messageId\":\"m\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\"}\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	paths := testPaths(t)
	s := NewSession("req-many", paths, nil, nil)
	start := time.Now()
	s.Start(httpclient.Spec{Method: "GET", URL: srv.URL, RequestTimeout: 10 * time.Second})
	s.Wait()
	elapsed := time.Since(start)
	snap := s.Snapshot()
	if snap.Events != n+4 {
		t.Fatalf("events = %d, want %d", snap.Events, n+4)
	}
	if len(s.AguiSnapshot().Messages) != 1 {
		t.Fatalf("agui messages = %d", len(s.AguiSnapshot().Messages))
	}
	if got := len(s.ModelText()); got != n*10 {
		t.Fatalf("model text = %d bytes, want %d", got, n*10)
	}
	t.Logf("10k events parsed in %s", elapsed)
	if elapsed > 5*time.Second {
		t.Fatalf("10k events took %s", elapsed)
	}
}

func TestReadHARResponseTimings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist.har")
	doc := `{"log":{"version":"1.2","creator":{"name":"api-x","version":"1"},"_apiX":{"isSseResponse":true,"responseSseEventCountText":"Event 10","ttftText":"1.25s","tpotText":"120ms"},"entries":[{"time":2000,"request":{"method":"POST","url":"https://example.com/echo","headers":[{"name":"X-In","value":"1"}],"postData":{"mimeType":"application/json","text":"{\"a\":1}"}},"response":{"status":200,"statusText":"OK","headers":[],"content":{"size":4,"mimeType":"text/event-stream","text":"data"}}}]}}`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, err := ReadHARResponse(path)
	if err != nil {
		t.Fatal(err)
	}
	if resp.TTFTMS != 1250 || resp.TPOTMS != 120 || resp.EventCount != 10 {
		t.Fatalf("timings = %#v", resp)
	}
	if resp.RequestMethod != "POST" || resp.RequestURL != "https://example.com/echo" ||
		len(resp.RequestHeaders) != 1 || resp.RequestBody != `{"a":1}` {
		t.Fatalf("request snapshot = %#v", resp)
	}
	if got := parseDurationText("1.5s"); got != 1500 {
		t.Fatalf("parse 1.5s = %d", got)
	}
}

func TestBinaryFileName(t *testing.T) {
	cases := []struct {
		ct, cd, url, want string
	}{
		{"image/png", "", "https://example.com/img/logo", "logo.png"},
		{"application/json", "", "https://example.com/data", "data.json"},
		{"application/pdf", `attachment; filename="report"`, "https://example.com/x", "report.pdf"},
		{"image/jpeg", `attachment; filename*=UTF-8''na%C3%AFve.jpg`, "https://example.com/x", "naïve.jpg"},
		{"application/octet-stream", "", "https://example.com/", "response.bin"},
	}
	for _, tc := range cases {
		if got := BinaryFileName(tc.ct, tc.cd, tc.url); got != tc.want {
			t.Fatalf("BinaryFileName(%q,%q,%q) = %q, want %q", tc.ct, tc.cd, tc.url, got, tc.want)
		}
	}
}
