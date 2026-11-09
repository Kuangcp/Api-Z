package ui

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func TestEnvironmentManager(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := app.OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	env := app.EnvState{
		Version:             1,
		ActiveEnvironmentID: "e1",
		Environments: []app.Environment{{
			ID: "e1", Name: "Local", Tag: "Local",
			Variables: []app.EnvVariable{{Key: "host", Value: "http://localhost"}},
		}},
	}
	if err := store.SaveEnvironments(env); err != nil {
		t.Fatal(err)
	}
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, env)
	tt := ui.NewTester(root.View, 1200, 800)
	tt.Frame()

	if err := tt.Click("环境管理…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("环境管理") || !tt.HasText("Local") {
		t.Fatalf("manager not shown: %v", tt.Texts())
	}
	if err := tt.Click("新增环境"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(root.envDraft.Environments) != 2 || root.envSel != 1 {
		t.Fatalf("draft = %d envs, sel=%d", len(root.envDraft.Environments), root.envSel)
	}
	root.envDraft.Environments[1].Name = "Prod"
	root.envDraft.Environments[1].Tag = "Prod"
	if err := tt.Click("添加变量"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	vars := root.envDraft.Environments[1].Variables
	if len(vars) != 1 {
		t.Fatalf("vars = %d", len(vars))
	}
	vars[0].Key = "host"
	vars[0].Value = "https://prod.example.com"
	if err := tt.Click("保存环境"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if root.envOpen {
		t.Fatal("manager still open")
	}
	saved, err := store.LoadEnvironments()
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Environments) != 2 || saved.Environments[1].Name != "Prod" {
		t.Fatalf("saved = %#v", saved)
	}
	if saved.Environments[1].Variables[0].Value != "https://prod.example.com" {
		t.Fatalf("saved vars = %#v", saved.Environments[1].Variables)
	}
	if root.envNames[0] != "Local" || root.envNames[1] != "Prod" {
		t.Fatalf("env names = %v", root.envNames)
	}
}

func TestEnvironmentManagerDeleteActive(t *testing.T) {
	paths := app.PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, _ := app.OpenStore(paths.DBPath)
	defer store.Close()
	env := app.EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []app.Environment{
		{ID: "e1", Name: "A"}, {ID: "e2", Name: "B"},
	}}
	_ = store.SaveEnvironments(env)
	root := NewRoot(paths, store, app.Tree{}, env)
	root.openEnvManager()
	root.deleteEnv()
	if root.envDraft.ActiveEnvironmentID != "e2" || len(root.envDraft.Environments) != 1 {
		t.Fatalf("draft after delete = %#v", root.envDraft)
	}
	root.saveEnvManager()
	saved, _ := store.LoadEnvironments()
	if saved.Active() == nil || saved.Active().Name != "B" {
		t.Fatalf("saved active = %#v", saved.Active())
	}
}
