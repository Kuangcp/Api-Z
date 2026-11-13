package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestCollectionSettingsDialog(t *testing.T) {
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	_ = store.UpdateNodeMeta(app.NodeCollection, cid,
		`{"color":"#123456","openapi":{"sourceUrl":"https://example.com/v3/api-docs","baseUrl":"https://api.example.com","root":{"mode":"baseUrl"}}}`)
	_ = store.SaveEnvironments(app.EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []app.Environment{{
		ID: "e1", Name: "Local", Tag: "Local", Variables: []app.EnvVariable{{Key: "host", Value: "http://localhost"}},
	}}})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	if err := tt.RightClick("Demo"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("设置…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("集合设置") || !tt.HasText("认证 (Auth)") || !tt.HasText("OpenAPI") {
		t.Fatalf("settings dialog = %v", tt.Texts())
	}
	if root.nodeSettingsSource != "https://example.com/v3/api-docs" || len(root.nodeSettingsAuth.Bearer) != 0 {
		t.Fatalf("draft = %#v source=%q", root.nodeSettingsAuth, root.nodeSettingsSource)
	}

	root.nodeSettingsAuth = app.Auth{Type: "bearer", Bearer: []app.AuthProperty{{Key: "token", Value: "secret", Type: "string"}}}
	root.nodeSettingsSource = "localhost:8080/v3/api-docs"
	root.nodeSettingsRootMode = 1
	root.nodeSettingsEnvKey = "host"
	root.saveNodeSettings()
	if root.nodeSettingsOpen {
		t.Fatal("settings still open")
	}
	tree, _ = store.LoadTree()
	col, ok := tree.Collection(cid)
	if !ok {
		t.Fatal("collection missing")
	}
	auth := app.ParseAuth(col.MetaJSON)
	if auth.Type != "bearer" || authValue(auth.Bearer, "token") != "secret" {
		t.Fatalf("saved auth = %#v", auth)
	}
	source, oaRoot := app.CollectionOpenAPIInfo(col.MetaJSON)
	if source != "localhost:8080/v3/api-docs" || oaRoot == nil || oaRoot.Mode != app.OpenAPIRootEnv || oaRoot.EnvKey != "host" {
		t.Fatalf("openapi = %q %#v", source, oaRoot)
	}
	if !strings.Contains(col.MetaJSON, `"color":"#123456"`) {
		t.Fatalf("color lost: %s", col.MetaJSON)
	}
}
