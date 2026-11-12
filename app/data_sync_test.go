package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDataPushPullRoundTrip(t *testing.T) {
	paths := testPaths(t)
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collID, err := store.CreateCollection("Demo")
	if err != nil {
		t.Fatal(err)
	}
	folderID, err := store.CreateFolder(collID, "", "v1")
	if err != nil {
		t.Fatal(err)
	}
	reqID, err := store.CreateRequest(collID, folderID, "Ping")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRequest(Request{
		ID: reqID, Name: "Ping", Method: "POST", URL: "https://example.com/ping",
		HeadersText: "Content-Type: application/json", BodyText: `{"ok":true}`,
		MetaJSON: `{"auth":{"type":"bearer","bearer":[{"key":"token","value":"t","type":"string"}]}}`,
	}); err != nil {
		t.Fatal(err)
	}
	env := EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []Environment{{
		ID: "e1", Name: "Local", Tag: "dev",
		Variables: []EnvVariable{{Key: "host", Value: "example.com"}},
	}}}
	if err := store.SaveEnvironments(env); err != nil {
		t.Fatal(err)
	}

	push, err := store.PushDataDir(paths)
	if err != nil {
		t.Fatal(err)
	}
	if push.Collections != 1 || !push.EnvWritten {
		t.Fatalf("push = %#v", push)
	}
	collFile := filepath.Join(DataCollectionDir(paths), collID+".json")
	data, err := os.ReadFile(collFile)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	info, _ := root["info"].(map[string]any)
	if info["_api_x_id"] != collID {
		t.Fatalf("collection id not exported: %v", info)
	}
	envData, err := os.ReadFile(DataEnvFile(paths))
	if err != nil {
		t.Fatal(err)
	}
	var pushedEnv EnvState
	if err := json.Unmarshal(envData, &pushedEnv); err != nil || pushedEnv.Environments[0].Name != "Local" {
		t.Fatalf("pushed env = %#v, %v", pushedEnv, err)
	}

	// Pull with the collection present merges without duplicating rows.
	pull, err := store.PullDataDir(paths)
	if err != nil {
		t.Fatal(err)
	}
	if pull.Merged != 1 || pull.Created != 0 || len(pull.FileErrs) != 0 {
		t.Fatalf("pull = %#v", pull)
	}
	tree, _ := store.LoadTree()
	if len(tree.Collections) != 1 || len(tree.Folders) != 1 || len(tree.Requests) != 1 {
		t.Fatalf("tree after merge = %#v", tree)
	}

	// A deleted collection comes back with the same ids, so tabs and history
	// links keep working and the next pull merges instead of duplicating.
	if err := store.DeleteCollection(collID); err != nil {
		t.Fatal(err)
	}
	pull, err = store.PullDataDir(paths)
	if err != nil {
		t.Fatal(err)
	}
	if pull.Created != 1 || pull.Merged != 0 {
		t.Fatalf("pull after delete = %#v", pull)
	}
	tree, _ = store.LoadTree()
	if len(tree.Collections) != 1 || tree.Collections[0].ID != collID {
		t.Fatalf("collection id = %#v", tree.Collections)
	}
	if len(tree.Requests) != 1 || tree.Requests[0].ID != reqID {
		t.Fatalf("request ids = %#v", tree.Requests)
	}

	// Local edits are overwritten by a newer pull, and no rows are added.
	if err := store.RenameNode(NodeRequest, reqID, "Local Rename"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PullDataDir(paths); err != nil {
		t.Fatal(err)
	}
	req, ok, err := store.GetRequest(reqID)
	if err != nil || !ok || req.Name != "Ping" {
		t.Fatalf("request after pull = %#v, %v, %v", req, ok, err)
	}
}

func TestDataPullMergesEnvironmentsWithoutDelete(t *testing.T) {
	paths := testPaths(t)
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.CreateCollection("Demo"); err != nil {
		t.Fatal(err)
	}
	base := EnvState{Version: 1, ActiveEnvironmentID: "e1", Environments: []Environment{
		{ID: "e1", Name: "Local", Variables: []EnvVariable{{Key: "a", Value: "1"}}},
	}}
	if err := store.SaveEnvironments(base); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PushDataDir(paths); err != nil {
		t.Fatal(err)
	}

	// Keep a local-only environment and an updated active one; pull must
	// update e1, keep e2, and leave the local active choice alone.
	local := base.Clone()
	local.Environments[0].Name = "Renamed Locally"
	local.Environments = append(local.Environments, Environment{ID: "e2", Name: "Local Only"})
	local.ActiveEnvironmentID = "e2"
	if err := store.SaveEnvironments(local); err != nil {
		t.Fatal(err)
	}
	pull, err := store.PullDataDir(paths)
	if err != nil {
		t.Fatal(err)
	}
	if !pull.EnvMerged {
		t.Fatalf("env not merged: %#v", pull)
	}
	got, err := store.LoadEnvironments()
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Environments) != 2 {
		t.Fatalf("environments = %#v", got.Environments)
	}
	if got.Environments[0].Name != "Local" || got.ActiveEnvironmentID != "e2" {
		t.Fatalf("merged env = %#v", got)
	}
}
