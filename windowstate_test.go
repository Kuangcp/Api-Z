package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

func TestWindowStateStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "window-state.json")
	store := windowStateStore{path: path}
	in := map[string]mygo.WindowState{
		"main": {X: 120, Y: 80, Width: 1280, Height: 800, Maximized: true},
	}
	if err := store.SaveWindowStates(in); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadWindowStates()
	if err != nil {
		t.Fatal(err)
	}
	if got["main"] != in["main"] {
		t.Fatalf("state = %#v", got)
	}
	if err := store.SaveWindowStates(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("empty save left file: %v", err)
	}
	if got, err := store.LoadWindowStates(); err != nil || got != nil {
		t.Fatalf("load missing = %#v, %v", got, err)
	}
}
