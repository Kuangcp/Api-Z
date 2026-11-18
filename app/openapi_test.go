package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const openAPIFixture = `{
  "openapi": "3.0.0",
  "info": {"title": "Demo", "version": "1"},
  "servers": [{"url": "http://api.test"}],
  "paths": {
    "/pets/{id}": {
      "get": {
        "tags": ["Pets"],
        "summary": "Get Pet",
        "parameters": [
          {"name": "limit", "in": "query", "example": 10},
          {"name": "X-Trace", "in": "header", "example": "t"}
        ]
      }
    },
    "/pets": {
      "post": {
        "tags": ["Pets"],
        "summary": "Create Pet",
        "operationId": "createPet",
        "requestBody": {"content": {"application/json": {"schema": {
          "type": "object",
          "properties": {"name": {"type": "string"}, "age": {"type": "integer"}}
        }}}}
      }
    },
    "/health": {"get": {"summary": "Health"}}
  }
}`

func TestParseOpenAPI(t *testing.T) {
	p, err := ParseOpenAPI(openAPIFixture, "http://api.test/v3/api-docs", "Demo", "coll-1", &OpenAPIRoot{Mode: OpenAPIRootBaseURL})
	if err != nil {
		t.Fatal(err)
	}
	if p.Requests != 3 || p.Folders != 2 {
		t.Fatalf("result = %d requests, %d folders", p.Requests, p.Folders)
	}
	if p.BaseURL != "http://api.test" {
		t.Fatalf("base = %q", p.BaseURL)
	}
	if len(p.Portable.Folders) != 2 || p.Portable.Folders[0].Name != "Pets" || p.Portable.Folders[1].Name != "Default" {
		t.Fatalf("folders = %#v", p.Portable.Folders)
	}
	var get, post PortableRequest
	for _, r := range p.Portable.Folders[0].Requests {
		if r.Method == "GET" {
			get = r
		}
		if r.Method == "POST" {
			post = r
		}
	}
	if get.Method != "GET" || get.URL != "http://api.test/pets/{id}" || get.Name != "Get Pet" {
		t.Fatalf("get = %#v", get)
	}
	if !strings.Contains(get.ParamsText, "limit: 10") || !strings.Contains(get.HeadersText, "X-Trace: t") {
		t.Fatalf("get params/headers = %q / %q", get.ParamsText, get.HeadersText)
	}
	if !strings.Contains(get.MetaJSON, `"syncKey":"GET /pets/{}"`) {
		t.Fatalf("get meta = %s", get.MetaJSON)
	}
	if post.Method != "POST" || !strings.Contains(post.HeadersText, "Content-Type: application/json") ||
		!strings.Contains(post.BodyText, `"age":0`) || !strings.Contains(post.BodyText, `"name":""`) {
		t.Fatalf("post = %#v", post)
	}
	if post.Auth == nil || post.Auth.Type != "inherit" {
		t.Fatalf("post auth = %#v", post.Auth)
	}

	// Deterministic IDs keep refresh stable.
	p2, _ := ParseOpenAPI(openAPIFixture, "http://api.test/v3/api-docs", "Demo", "coll-1", nil)
	var get2 PortableRequest
	for _, r := range p2.Portable.Folders[0].Requests {
		if r.Method == "GET" {
			get2 = r
		}
	}
	if p.Portable.Folders[0].ID != p2.Portable.Folders[0].ID || get.ID != get2.ID {
		t.Fatal("IDs are not deterministic")
	}

	env, _ := ParseOpenAPI(openAPIFixture, "http://api.test/v3/api-docs", "Demo", "coll-1",
		&OpenAPIRoot{Mode: OpenAPIRootEnv, EnvKey: "host"})
	var envGet PortableRequest
	for _, r := range env.Portable.Folders[0].Requests {
		if r.Method == "GET" {
			envGet = r
		}
	}
	if envGet.URL != "{{host}}/pets/{id}" {
		t.Fatalf("env root URL = %q", envGet.URL)
	}
}

func TestNormalizeOpenAPIPath(t *testing.T) {
	cases := map[string]string{
		"/pets/{id}":      "/pets/{}",
		"pets/{petId}/x/": "/pets/{}/x",
		"//a//{x}//":      "/a/{}",
		"/":               "/",
	}
	for in, want := range cases {
		if got := normalizeOpenAPIPath(in); got != want {
			t.Fatalf("normalize %q = %q, want %q", in, got, want)
		}
	}
}

func TestMergeOpenAPIPreservesLocalEdits(t *testing.T) {
	paths := PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, _ := OpenStore(paths.DBPath)
	defer store.Close()
	collectionID, _ := store.CreateCollection("Demo")
	p1, err := ParseOpenAPI(openAPIFixture, "http://api.test/v3/api-docs", "Demo", collectionID, &OpenAPIRoot{Mode: OpenAPIRootBaseURL})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeOpenAPICollection(collectionID, p1.Portable); err != nil {
		t.Fatal(err)
	}
	tree, _ := store.LoadTree()
	var getID string
	for _, r := range tree.Requests {
		if r.Name == "Get Pet" {
			getID = r.ID
		}
	}
	if getID == "" {
		t.Fatal("Get Pet not imported")
	}
	req, _ := tree.Request(getID)
	req.URL = "http://localhost/local"
	req.HeadersText = "X-Local: 1"
	req.BodyText = `{"local":true}`
	if err := store.SaveRequest(req); err != nil {
		t.Fatal(err)
	}

	changed := strings.Replace(openAPIFixture, `"summary": "Get Pet"`, `"summary": "Get Pet v2"`, 1)
	changed = strings.Replace(changed, `"/health": {"get": {"summary": "Health"}}`, `"/new": {"get": {"summary": "New"}}`, 1)
	p2, _ := ParseOpenAPI(changed, "http://api.test/v3/api-docs", "Demo", collectionID, nil)
	stats, err := store.MergeOpenAPICollection(collectionID, p2.Portable)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Added != 1 || stats.Updated != 2 {
		t.Fatalf("stats = %#v", stats)
	}
	tree, _ = store.LoadTree()
	if len(tree.Requests) != 4 {
		t.Fatalf("requests = %d, want 4 (removed endpoint kept)", len(tree.Requests))
	}
	req, _ = tree.Request(getID)
	if req.Name != "Get Pet v2" || req.URL != "http://localhost/local" ||
		req.HeadersText != "X-Local: 1" || req.BodyText != `{"local":true}` {
		t.Fatalf("merged request = %#v", req)
	}
	meta := tree.Collections[0].MetaJSON
	source, root := CollectionOpenAPIInfo(meta)
	if source != "http://api.test/v3/api-docs" || root == nil {
		t.Fatalf("collection meta = %s", meta)
	}
}

func TestRefreshOpenAPICollection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, openAPIFixture)
	}))
	defer srv.Close()
	paths := PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, _ := OpenStore(paths.DBPath)
	defer store.Close()
	parsed, _ := ParseOpenAPI(openAPIFixture, srv.URL, "Demo", "temp", &OpenAPIRoot{Mode: OpenAPIRootBaseURL})
	id, _, err := store.CreateOpenAPICollection("Demo", parsed.Portable)
	if err != nil {
		t.Fatal(err)
	}
	// Re-parse with the real collection id so refresh can merge by ID.
	parsed, _ = ParseOpenAPI(openAPIFixture, srv.URL, "Demo", id, &OpenAPIRoot{Mode: OpenAPIRootBaseURL})
	if _, err := store.MergeOpenAPICollection(id, parsed.Portable); err != nil {
		t.Fatal(err)
	}
	// Change the local URL, then refresh from the server.
	tree, _ := store.LoadTree()
	var req Request
	for _, r := range tree.Requests {
		if r.Name == "Get Pet" {
			req = r
		}
	}
	req.URL = "http://local/edited"
	_ = store.SaveRequest(req)
	if _, err := store.RefreshOpenAPICollection(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	tree, _ = store.LoadTree()
	for _, r := range tree.Requests {
		if r.ID == req.ID && r.URL != "http://local/edited" {
			t.Fatalf("refresh overwrote local URL: %#v", r)
		}
	}
}

func openAPITestStore(t *testing.T) (*Store, string) {
	t.Helper()
	paths := PathsAt(t.TempDir(), t.TempDir())
	_ = paths.Ensure()
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	id, err := store.CreateCollection("Demo")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ParseOpenAPI(openAPIFixture, "http://api.test/v3/api-docs", "Demo", id, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeOpenAPICollection(id, p.Portable); err != nil {
		t.Fatal(err)
	}
	return store, id
}

func openAPITestRequest(t *testing.T, tree Tree, name string) Request {
	t.Helper()
	for _, r := range tree.Requests {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("request %q not found", name)
	return Request{}
}

func openAPITestMerge(t *testing.T, store *Store, collectionID, fixture string) {
	t.Helper()
	p, err := ParseOpenAPI(fixture, "http://api.test/v3/api-docs", "Demo", collectionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MergeOpenAPICollection(collectionID, p.Portable); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshOpenAPIPreservesPathParamEdit(t *testing.T) {
	store, collectionID := openAPITestStore(t)
	tree, _ := store.LoadTree()
	req := openAPITestRequest(t, tree, "Get Pet")
	req.URL = "http://api.test/pets/42"
	if err := store.SaveRequest(req); err != nil {
		t.Fatal(err)
	}
	before := len(tree.Requests)

	openAPITestMerge(t, store, collectionID, openAPIFixture)
	tree, _ = store.LoadTree()
	if len(tree.Requests) != before {
		t.Fatalf("requests = %d, want %d; refresh must not add a server copy", len(tree.Requests), before)
	}
	got, ok := tree.Request(req.ID)
	if !ok || got.URL != "http://api.test/pets/42" {
		t.Fatalf("path param edit lost: %#v", got)
	}
	if !strings.Contains(got.MetaJSON, `"path":"/pets/{id}"`) {
		t.Fatalf("openapi identity lost: %s", got.MetaJSON)
	}
}

func TestMergeOpenAPIAfterRequestMovedAcrossCollections(t *testing.T) {
	store, collectionID := openAPITestStore(t)
	targetID, err := store.CreateCollection("Other")
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := store.LoadTree()
	req := openAPITestRequest(t, tree, "Get Pet")
	if err := store.MoveNodeTo(NodeRequest, req.ID, targetID, ""); err != nil {
		t.Fatal(err)
	}
	// An editor opened before the move still holds the old openapi metadata;
	// autosave must not reattach it.
	req.HeadersText = "X-Local: 1"
	req.URL = "http://api.test/pets/42"
	if err := store.SaveRequest(req); err != nil {
		t.Fatal(err)
	}
	tree, _ = store.LoadTree()
	moved, _ := tree.Request(req.ID)
	if strings.Contains(moved.MetaJSON, `"openapi"`) {
		t.Fatalf("moved request keeps openapi binding: %s", moved.MetaJSON)
	}
	if moved.HeadersText != "X-Local: 1" || moved.URL != "http://api.test/pets/42" {
		t.Fatalf("stale save did not keep local fields: %#v", moved)
	}

	openAPITestMerge(t, store, collectionID, openAPIFixture)
	tree, _ = store.LoadTree()
	if len(tree.Requests) != 4 {
		t.Fatalf("requests = %d, want 4 (3 restored + moved copy)", len(tree.Requests))
	}
	openAPITestMerge(t, store, collectionID, openAPIFixture)
	tree, _ = store.LoadTree()
	if len(tree.Requests) != 4 {
		t.Fatalf("second refresh duplicated requests: %d", len(tree.Requests))
	}
}

func TestMergeOpenAPIAfterFolderMovedAcrossCollections(t *testing.T) {
	store, collectionID := openAPITestStore(t)
	targetID, err := store.CreateCollection("Other")
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := store.LoadTree()
	var folder Folder
	for _, f := range tree.Folders {
		if f.Name == "Pets" {
			folder = f
		}
	}
	if folder.ID == "" {
		t.Fatal("Pets folder not imported")
	}
	if err := store.MoveNodeTo(NodeFolder, folder.ID, targetID, ""); err != nil {
		t.Fatal(err)
	}
	openAPITestMerge(t, store, collectionID, openAPIFixture)
	openAPITestMerge(t, store, collectionID, openAPIFixture)
	tree, _ = store.LoadTree()
	folders, requests := 0, 0
	for _, f := range tree.Folders {
		if f.CollectionID == collectionID {
			folders++
		}
	}
	for _, r := range tree.Requests {
		if r.CollectionID == collectionID {
			requests++
		}
	}
	if folders != 2 || requests != 3 {
		t.Fatalf("restored collection = %d folders / %d requests, want 2/3", folders, requests)
	}
}

func TestMergeOpenAPIDuplicateRequest(t *testing.T) {
	store, collectionID := openAPITestStore(t)
	tree, _ := store.LoadTree()
	req := openAPITestRequest(t, tree, "Get Pet")
	copyID, err := store.DuplicateRequest(req.ID)
	if err != nil {
		t.Fatal(err)
	}
	tree, _ = store.LoadTree()
	copied, _ := tree.Request(copyID)
	if strings.Contains(copied.MetaJSON, `"openapi"`) {
		t.Fatalf("duplicate keeps openapi binding: %s", copied.MetaJSON)
	}
	openAPITestMerge(t, store, collectionID, openAPIFixture)
	openAPITestMerge(t, store, collectionID, openAPIFixture)
	tree, _ = store.LoadTree()
	if len(tree.Requests) != 4 {
		t.Fatalf("requests = %d, want 4", len(tree.Requests))
	}
}

func TestSaveRequestKeepsRefreshedOpenAPIMeta(t *testing.T) {
	store, _ := openAPITestStore(t)
	tree, _ := store.LoadTree()
	req := openAPITestRequest(t, tree, "Get Pet")
	req.BodyText = `{"local":true}`
	req.MetaJSON = `{"auth":{"type":"bearer"}}`
	if err := store.SaveRequest(req); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetRequest(req.ID)
	if err != nil || !ok {
		t.Fatalf("get request: %v %v", ok, err)
	}
	if !strings.Contains(got.MetaJSON, `"openapi"`) || !strings.Contains(got.MetaJSON, `"syncKey":"GET /pets/{}"`) {
		t.Fatalf("refresh metadata was overwritten: %s", got.MetaJSON)
	}
	if ParseAuth(got.MetaJSON).Type != "bearer" {
		t.Fatalf("local auth lost: %s", got.MetaJSON)
	}
}

func TestSetCollectionOpenAPIBinding(t *testing.T) {
	meta := `{"auth":{"type":"bearer"},"color":"#123456","openapi":{"sourceUrl":"https://old","baseUrl":"https://api","root":{"mode":"baseUrl"}}}`
	got := SetCollectionOpenAPI(meta, "localhost:8080/v3/api-docs", &OpenAPIRoot{Mode: OpenAPIRootEnv, EnvKey: "host"})
	source, root := CollectionOpenAPIInfo(got)
	if source != "localhost:8080/v3/api-docs" || root == nil || root.Mode != OpenAPIRootEnv || root.EnvKey != "host" {
		t.Fatalf("binding = %q %#v", source, root)
	}
	if ParseAuth(got).Type != "bearer" || !strings.Contains(got, `"color":"#123456"`) {
		t.Fatalf("metadata lost: %s", got)
	}
	got = SetCollectionOpenAPI(got, "", nil)
	if source, root := CollectionOpenAPIInfo(got); source != "" || root != nil {
		t.Fatalf("unbind = %q %#v", source, root)
	}
	if ParseAuth(got).Type != "bearer" {
		t.Fatalf("auth lost on unbind: %s", got)
	}
}

func TestParseOpenAPIFormBody(t *testing.T) {
	doc := `{
  "openapi": "3.0.0",
  "info": {"title": "Form", "version": "1"},
  "paths": {
    "/login": {"post": {
      "requestBody": {"content": {"application/x-www-form-urlencoded": {"schema": {
        "type": "object",
        "properties": {"user": {"type": "string", "example": "a b"}, "age": {"type": "integer"}}
      }}}}
    }}
  }
}`
	p, err := ParseOpenAPI(doc, "http://api.test/openapi.json", "Form", "coll", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Portable.Folders) != 1 || len(p.Portable.Folders[0].Requests) != 1 {
		t.Fatalf("folders = %#v", p.Portable.Folders)
	}
	req := p.Portable.Folders[0].Requests[0]
	if req.BodyText != "age=0&user=a+b" {
		t.Fatalf("form body = %q", req.BodyText)
	}
	if !strings.Contains(req.HeadersText, "Content-Type: application/x-www-form-urlencoded") {
		t.Fatalf("headers = %q", req.HeadersText)
	}
}
