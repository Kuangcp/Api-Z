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

func TestLogsRequestsAndPanics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := InitLogs(dir); err != nil {
		t.Fatal(err)
	}
	defer CloseLogs()
	paths := testPaths(t)
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	s := NewSession("log-req", paths, store, nil)
	s.Start(httpclient.Spec{Method: "GET", URL: srv.URL + "/api?token=secret", RequestTimeout: 5 * time.Second})
	s.Wait()

	func() {
		defer RecoverPanic("ui")
		panic("boom")
	}()
	CloseLogs()

	var logText string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(path, ".log") {
			if b, readErr := os.ReadFile(path); readErr == nil {
				logText += string(b)
			}
		}
		return nil
	})
	for _, want := range []string{"request start", "method=GET", "request done", "recovered panic", "panic=boom", "url=" + RedactURL(srv.URL+"/api?token=secret")} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log missing %q:\n%s", want, logText)
		}
	}
	if strings.Contains(logText, "token=secret") {
		t.Fatalf("log leaked query:\n%s", logText)
	}
}
