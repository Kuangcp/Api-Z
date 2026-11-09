package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"api-z/app"
)

func TestFindVarPrefixInsert(t *testing.T) {
	start, prefix, ok := findVarPrefix("https://{{ho", 12)
	if !ok || start != 8 || prefix != "ho" {
		t.Fatalf("start=%d prefix=%q ok=%v", start, prefix, ok)
	}
	if _, _, ok := findVarPrefix("{{host}}", 8); ok {
		t.Fatal("closed variable should not suggest")
	}
	if _, _, ok := findVarPrefix("plain", 5); ok {
		t.Fatal("plain text should not suggest")
	}

	got, caret := insertVariable("{{ho", 0, 4, "host")
	if got != "{{host}}" || caret != 8 {
		t.Fatalf("insert = %q caret=%d", got, caret)
	}
	got, caret = insertVariable("{{ho}}", 0, 4, "host")
	if got != "{{host}}" || caret != 6 {
		t.Fatalf("insert closed = %q caret=%d", got, caret)
	}
	got, caret = insertVariable("x {{", 2, 4, "host")
	if got != "x {{host}}" || caret != 10 {
		t.Fatalf("insert middle = %q caret=%d", got, caret)
	}
}

func TestVariableAutocompleteUI(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	env := app.EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []app.Environment{{
		ID: "e1", Name: "Local", Variables: []app.EnvVariable{
			{Key: "host", Value: "http://localhost"},
			{Key: "token", Value: "secret"},
		},
	}}}
	root := NewRoot(paths, store, app.Tree{}, env)
	ed := &Editor{root: root}
	text := ""
	view := func(c *ui.Context) {
		input := ui.TextInput(c, &text).AutoFocus()
		root.varAutocomplete(c, ed, "url", input, &text, input.Changed())
	}
	tt := ui.NewTester(view, 600, 300)
	tt.Frame()
	tt.Type("{{ho")
	tt.Frame()
	if !tt.HasText("host") || !tt.HasText("{{host}}") {
		t.Fatalf("suggestions = %v", tt.Texts())
	}
	if err := tt.Click("{{host}}"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if text != "{{host}}" {
		t.Fatalf("text = %q", text)
	}
}

func TestVariableAutocompleteTextArea(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	env := app.EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []app.Environment{{
		ID: "e1", Name: "Local", Variables: []app.EnvVariable{{Key: "host", Value: "http://localhost"}},
	}}}
	root := NewRoot(paths, store, app.Tree{}, env)
	ed := &Editor{root: root}
	text := ""
	view := func(c *ui.Context) {
		area := ui.TextArea(c, &text).AutoFocus()
		root.varAutocomplete(c, ed, "headers", area, &text, area.Changed())
	}
	tt := ui.NewTester(view, 600, 300)
	tt.Frame()
	tt.Type("{{ho")
	tt.Frame()
	if !tt.HasText("{{host}}") {
		t.Fatalf("textarea suggestions = %v", tt.Texts())
	}
	if err := tt.Click("{{host}}"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if text != "{{host}}" {
		t.Fatalf("textarea text = %q", text)
	}
}

func TestVariableAutocompleteBodyEditor(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	env := app.EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []app.Environment{{
		ID: "e1", Name: "Local", Variables: []app.EnvVariable{{Key: "host", Value: "http://localhost"}},
	}}}
	root := NewRoot(paths, store, app.Tree{}, env)
	ed := &Editor{root: root}
	ed.resetBodyEditor()
	view := func(c *ui.Context) {
		editor.View(c, ed.bodyState()).AutoFocus()
	}
	tt := ui.NewTester(view, 600, 300)
	tt.Frame()
	tt.Type("{{ho")
	tt.Frame()
	ed.completeBody(editor.CompletionRequest{Offset: 4, Prefix: "ho"})
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	got := ed.bodyEd.Buffer.String()
	if got != "{{host}}" {
		t.Fatalf("body = %q", got)
	}
}
