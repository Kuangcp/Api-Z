package app

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateFromApiXReadOnly(t *testing.T) {
	srcDir := t.TempDir()
	srcStore, err := OpenStore(filepath.Join(srcDir, "collections.db"))
	if err != nil {
		t.Fatal(err)
	}
	cid, err := srcStore.CreateCollection("Demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srcStore.db.Exec(`INSERT INTO folders(id, collection_id, parent_folder_id, name,
		sort_order, created_at, updated_at, meta_json) VALUES('f1', ?, NULL, 'Folder', 0, 1, 1, '{}')`, cid); err != nil {
		t.Fatal(err)
	}
	rid, err := srcStore.CreateRequest(cid, "", "Echo")
	if err != nil {
		t.Fatal(err)
	}
	if err := srcStore.SaveRequest(Request{
		ID:          rid,
		Name:        "Echo",
		Method:      "POST",
		URL:         "{{host}}/echo",
		HeadersText: "Content-Type: application/json",
		ParamsText:  "q: 1",
		BodyText:    `{"a":1}`,
		MetaJSON:    `{"auth":{"type":"bearer","bearer":[{"key":"token","value":"{{token}}"}]}}`,
	}); err != nil {
		t.Fatal(err)
	}
	srcStore.Close()
	_ = os.Remove(filepath.Join(srcDir, "collections.db-wal"))
	_ = os.Remove(filepath.Join(srcDir, "collections.db-shm"))

	env := `{"version":1,"activeEnvironmentId":"e1","environments":[{"id":"e1","name":"Local","variables":[{"key":"host","value":"http://localhost"}]}]}`
	if err := os.WriteFile(filepath.Join(srcDir, "environments.json"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	respDir := filepath.Join(srcDir, "request", rid, "response")
	if err := os.MkdirAll(respDir, 0o755); err != nil {
		t.Fatal(err)
	}
	har := `{"log":{"_apiX":{"savedAtEpochMs":123,"isSseResponse":true,"responseSseEventCountText":"Event 3"},"entries":[{"time":42,"response":{"status":200,"bodySize":5,"content":{"size":5}}}]}}`
	if err := os.WriteFile(filepath.Join(respDir, "123.har"), []byte(har), 0o600); err != nil {
		t.Fatal(err)
	}
	before := hashFile(t, filepath.Join(srcDir, "collections.db"))

	dstPaths := testPaths(t)
	dst, err := OpenStore(dstPaths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	stats, err := MigrateFromApiX(dst, dstPaths, srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Collections != 1 || stats.Folders != 1 || stats.Requests != 1 || stats.Artifacts != 1 || stats.Exchanges != 1 {
		t.Fatalf("stats = %#v", stats)
	}
	tree, err := dst.LoadTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Requests) != 1 || tree.Requests[0].URL != "{{host}}/echo" ||
		tree.Requests[0].ParamsText != "q: 1" || tree.Requests[0].MetaJSON == "" {
		t.Fatalf("request = %#v", tree.Requests)
	}
	if got := ParseAuth(tree.Requests[0].MetaJSON); got.Type != "bearer" {
		t.Fatalf("auth = %#v", got)
	}
	envState, err := dst.LoadEnvironments()
	if err != nil || len(envState.Environments) != 1 || envState.Active().Name != "Local" {
		t.Fatalf("env = %#v, err = %v", envState, err)
	}
	if _, err := os.Stat(filepath.Join(dstPaths.Artifacts, rid, "response", "123.har")); err != nil {
		t.Fatalf("artifact not copied: %v", err)
	}
	exchanges, err := dst.ListExchanges(rid, 10)
	if err != nil || len(exchanges) != 1 || exchanges[0].Status != 200 || exchanges[0].EventCount != 3 {
		t.Fatalf("exchanges = %#v, err = %v", exchanges, err)
	}
	if after := hashFile(t, filepath.Join(srcDir, "collections.db")); after != before {
		t.Fatalf("source database changed during migration")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(filepath.Join(srcDir, "collections.db"+suffix)); !os.IsNotExist(err) {
			t.Fatalf("migration created collections.db%s beside the source", suffix)
		}
	}

	// A second run is a no-op.
	stats2, err := MigrateFromApiX(dst, dstPaths, srcDir)
	if err != nil || !stats2.AlreadyDone {
		t.Fatalf("second migrate = %#v, %v", stats2, err)
	}
}

func TestMigrateRefusesNonEmptyDestination(t *testing.T) {
	srcDir := t.TempDir()
	srcStore, err := OpenStore(filepath.Join(srcDir, "collections.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srcStore.CreateCollection("Source"); err != nil {
		t.Fatal(err)
	}
	srcStore.Close()

	dstPaths := testPaths(t)
	dst, err := OpenStore(dstPaths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if _, err := dst.CreateCollection("Existing"); err != nil {
		t.Fatal(err)
	}
	if _, err := MigrateFromApiX(dst, dstPaths, srcDir); !errors.Is(err, ErrDestinationNotEmpty) {
		t.Fatalf("err = %v, want ErrDestinationNotEmpty", err)
	}
}

func hashFile(t *testing.T, path string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}
