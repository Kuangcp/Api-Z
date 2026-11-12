package ui

import (
	"testing"

	"api-z/app"
)

func TestEditorAutoSave(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Echo")
	_ = store.SaveRequest(app.Request{ID: rid, Name: "Echo", Method: "GET", URL: "https://example.com/a"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(rid)
	ed := root.tabs[0]

	ed.URL = "https://example.com/b"
	ed.Dirty = true
	if err := ed.autoSaveNow(); err != nil {
		t.Fatal(err)
	}
	if ed.Dirty {
		t.Fatal("dirty after save")
	}
	got, ok, err := store.GetRequest(rid)
	if err != nil || !ok || got.URL != "https://example.com/b" {
		t.Fatalf("saved request = %#v, %v, %v", got, ok, err)
	}

	// An older snapshot must not overwrite a newer one.
	ed.URL = "https://example.com/c"
	old, oldGen := ed.snapshotRequest()
	ed.URL = "https://example.com/d"
	newReq, newGen := ed.snapshotRequest()
	if err := ed.writeSnapshot(old, oldGen); err != nil {
		t.Fatal(err)
	}
	if err := ed.writeSnapshot(newReq, newGen); err != nil {
		t.Fatal(err)
	}
	got, _, _ = store.GetRequest(rid)
	if got.URL != "https://example.com/d" {
		t.Fatalf("stale save won: %q", got.URL)
	}
}
