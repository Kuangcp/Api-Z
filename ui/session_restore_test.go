package ui

import (
	"testing"

	"api-z/app"
)

func TestRestoreLastRequest(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	a, _ := store.CreateRequest(cid, "", "First")
	b, _ := store.CreateRequest(cid, "", "Second")
	_ = store.SaveRequest(app.Request{ID: a, Name: "First", Method: "GET", URL: "https://example.com/a"})
	_ = store.SaveRequest(app.Request{ID: b, Name: "Second", Method: "GET", URL: "https://example.com/b"})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	if len(root.tabs) != 0 {
		t.Fatalf("fresh root opened tabs: %#v", root.tabs)
	}
	root.openRequest(a)
	root.openRequest(b)
	root.active = 1
	root.rememberActiveRequest()
	if got := app.LoadSessionPrefs(app.SessionPath(paths.ConfigDir)).LastRequest; got != b {
		t.Fatalf("saved last request = %q", got)
	}

	tree, _ = store.LoadTree()
	again := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	if len(again.tabs) != 1 || again.tabs[0].ID != b || again.selected != b {
		t.Fatalf("restored tabs = %#v selected=%q", again.tabs, again.selected)
	}
	if again.activeCollection != cid {
		t.Fatalf("active collection = %q", again.activeCollection)
	}

	// Closing the last tab clears the memory, so the next start is blank.
	again.closeTabAt(0)
	tree, _ = store.LoadTree()
	blank := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	if len(blank.tabs) != 0 {
		t.Fatalf("tabs after close = %#v", blank.tabs)
	}

	// A remembered request that was deleted also falls back to the blank state.
	root2 := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root2.openRequest(a)
	if err := store.DeleteRequest(a); err != nil {
		t.Fatal(err)
	}
	tree, _ = store.LoadTree()
	missing := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	if len(missing.tabs) != 0 {
		t.Fatalf("missing request restored: %#v", missing.tabs)
	}
}

func TestThemeSchemeAndRadius(t *testing.T) {
	light := resolveTheme(uiLight(), app.ThemePrefs{Scheme: "ocean"})
	if light.Background != hexColor("#edf4fb") || light.Surface != hexColor("#ffffff") {
		t.Fatalf("ocean light = %#v", light)
	}
	dark := resolveTheme(uiLight(), app.ThemePrefs{Mode: 2, Scheme: "ocean"})
	if dark.Background != hexColor("#0f1822") {
		t.Fatalf("ocean dark = %#v", dark)
	}
	if got := WindowBackgroundColor(app.ThemePrefs{Scheme: "ocean"}); got != "light-dark(#edf4fb, #0f1822)" {
		t.Fatalf("window bg = %q", got)
	}
	if got := WindowCornerRadius(app.ThemePrefs{Radius: 8}); got != 12 {
		t.Fatalf("corner radius = %d", got)
	}
	if got := themeRadius(app.ThemePrefs{}); got != 6 {
		t.Fatalf("default radius = %v", got)
	}
}
