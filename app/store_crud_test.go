package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreTreeCRUD(t *testing.T) {
	paths := PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	colA, err := store.CreateCollection("A")
	if err != nil {
		t.Fatal(err)
	}
	colB, err := store.CreateCollection("B")
	if err != nil {
		t.Fatal(err)
	}
	folder1, err := store.CreateFolder(colA, "", "F1")
	if err != nil {
		t.Fatal(err)
	}
	folder2, err := store.CreateFolder(colA, "", "F2")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := store.CreateFolder(colA, folder1, "F1-sub")
	if err != nil {
		t.Fatal(err)
	}
	r1, err := store.CreateRequest(colA, "", "R1")
	if err != nil {
		t.Fatal(err)
	}
	r2, err := store.CreateRequest(colA, "", "R2")
	if err != nil {
		t.Fatal(err)
	}
	r3, err := store.CreateRequest(colA, folder1, "R3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateRequest(colA, folder2, "R4"); err != nil {
		t.Fatal(err)
	}

	// Collections: B moves before A.
	if err := store.MoveSibling(NodeCollection, colB, -1); err != nil {
		t.Fatal(err)
	}
	tree, err := store.LoadTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Collections) != 2 || tree.Collections[0].ID != colB || tree.Collections[1].ID != colA {
		t.Fatalf("collections order = %#v", tree.Collections)
	}

	// Requests in the collection root: R2 moves before R1.
	if err := store.MoveSibling(NodeRequest, r2, -1); err != nil {
		t.Fatal(err)
	}
	tree, _ = store.LoadTree()
	var rootOrder []string
	for _, req := range tree.Requests {
		if req.CollectionID == colA && req.FolderID == "" {
			rootOrder = append(rootOrder, req.ID)
		}
	}
	if len(rootOrder) != 2 || rootOrder[0] != r2 || rootOrder[1] != r1 {
		t.Fatalf("request order = %v", rootOrder)
	}

	if err := store.RenameNode(NodeRequest, r1, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if got := store.NodeName(NodeRequest, r1); got != "Renamed" {
		t.Fatalf("renamed = %q", got)
	}

	dup, err := store.DuplicateRequest(r1)
	if err != nil {
		t.Fatal(err)
	}
	if dup == r1 {
		t.Fatal("duplicate reused id")
	}
	if n, _ := store.Count("requests"); n != 5 {
		t.Fatalf("requests after duplicate = %d, want 5", n)
	}
	if got := store.NodeName(NodeRequest, dup); got != "Renamed Copy" {
		t.Fatalf("duplicate name = %q", got)
	}

	// Deleting a request removes its artifacts and exchange rows.
	artifactDir := paths.RequestDir(r1)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactDir, "x.raw"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.AddExchange(Exchange{ID: "e1", RequestID: r1, SavedAt: 1, HARPath: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteRequest(r1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(artifactDir); !os.IsNotExist(err) {
		t.Fatalf("artifact dir still exists: %v", err)
	}
	if list, _ := store.ListExchanges(r1, 10); len(list) != 0 {
		t.Fatalf("exchanges after delete = %d", len(list))
	}

	// Deleting a folder cascades its subtree, exchanges and artifacts.
	if err := store.AddExchange(Exchange{ID: "e2", RequestID: r3, SavedAt: 2, HARPath: "y"}); err != nil {
		t.Fatal(err)
	}
	r3dir := paths.RequestDir(r3)
	if err := os.MkdirAll(r3dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r3dir, "y.raw"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteFolder(folder1); err != nil {
		t.Fatal(err)
	}
	tree, _ = store.LoadTree()
	for _, f := range tree.Folders {
		if f.ID == folder1 || f.ID == sub {
			t.Fatalf("deleted folder still present: %#v", f)
		}
	}
	if list, _ := store.ListExchanges(r3, 10); len(list) != 0 {
		t.Fatalf("folder exchanges after delete = %d", len(list))
	}
	if _, err := os.Stat(r3dir); !os.IsNotExist(err) {
		t.Fatalf("folder artifact dir still exists: %v", err)
	}
	if n, _ := store.Count("requests"); n != 3 {
		t.Fatalf("requests after folder delete = %d, want 3", n)
	}

	// Deleting the collection removes everything left under it.
	if err := store.DeleteCollection(colA); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.Count("requests"); n != 0 {
		t.Fatalf("requests after collection delete = %d", n)
	}
	if n, _ := store.Count("exchanges"); n != 0 {
		t.Fatalf("exchanges after collection delete = %d", n)
	}
}
