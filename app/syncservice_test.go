package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncPushPullSharedRepo(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if out, err := exec.Command("git", "init", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	pathsA := testPaths(t)
	storeA, err := OpenStore(pathsA.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()
	cid, _ := storeA.CreateCollection("Demo")
	fid, _ := storeA.CreateFolder(cid, "", "Nested")
	rid, _ := storeA.CreateRequest(cid, fid, "Ping")
	_ = storeA.SaveRequest(Request{ID: rid, Name: "Ping", Method: "GET", URL: "https://example.com/ping"})
	_ = storeA.SaveEnvironments(EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []Environment{{
		ID: "e1", Name: "Local", Variables: []EnvVariable{{Key: "host", Value: "localhost"}},
	}}})

	prefsA := filepath.Join(t.TempDir(), "sync.json")
	if err := SaveSyncPrefs(prefsA, SyncPrefs{RepoURL: remote, Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	svcA := NewSyncService(storeA, pathsA, prefsA)
	keyText, err := svcA.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	out := svcA.Run(context.Background(), "push")
	if !out.OK {
		t.Fatalf("push = %#v", out)
	}
	work := SyncWorkTree(prefsA)
	if _, err := os.Stat(filepath.Join(work, "api-z", "collections", cid+".enc")); err != nil {
		t.Fatalf("collection file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "README.md")); !os.IsNotExist(err) {
		t.Fatalf("sync created a root README: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "api-z", "key.kcv")); err != nil {
		t.Fatalf("kcv: %v", err)
	}

	pathsB := testPaths(t)
	storeB, err := OpenStore(pathsB.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()
	prefsB := filepath.Join(t.TempDir(), "sync.json")
	if err := SaveSyncPrefs(prefsB, SyncPrefs{RepoURL: remote, Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	svcB := NewSyncService(storeB, pathsB, prefsB)
	if err := svcB.ImportKey(keyText); err != nil {
		t.Fatal(err)
	}
	out = svcB.Run(context.Background(), "pull")
	if !out.OK {
		t.Fatalf("pull = %#v", out)
	}
	tree, err := storeB.LoadTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Collections) != 1 || tree.Collections[0].ID != cid || len(tree.Requests) != 1 || tree.Requests[0].ID != rid {
		t.Fatalf("imported tree = %#v", tree)
	}
	env, _ := storeB.LoadEnvironments()
	if len(env.Environments) != 1 || env.Environments[0].Name != "Local" {
		t.Fatalf("imported env = %#v", env)
	}

	// Pull again keeps local-only data and does not duplicate rows.
	extra, _ := storeB.CreateCollection("LocalOnly")
	out = svcB.Run(context.Background(), "pull")
	if !out.OK {
		t.Fatalf("second pull = %#v", out)
	}
	tree, _ = storeB.LoadTree()
	if len(tree.Collections) != 2 {
		t.Fatalf("collections after pull = %#v", tree.Collections)
	}
	if _, ok := tree.Collection(extra); !ok {
		t.Fatal("local-only collection removed")
	}
	var names []string
	for _, c := range tree.Collections {
		names = append(names, c.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "Demo") {
		t.Fatalf("remote collection missing: %v", names)
	}
}
