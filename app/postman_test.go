package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const postmanFixture = `{
  "info": {
    "name": "Demo",
    "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
    "_postman_id": "pm-1",
    "_api_x_id": "ax-1",
    "description": "collection desc"
  },
  "variable": [{"key": "host", "value": "http://localhost"}],
  "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "{{token}}"}]},
  "item": [
    {
      "name": "Folder A",
      "description": "folder desc",
      "auth": {"type": "basic", "basic": [{"key": "username", "value": "u"}, {"key": "password", "value": "p"}]},
      "item": [
        {
          "id": "req-1",
          "name": "Echo",
          "description": {"content": "request desc"},
          "auth": {"type": "apikey", "apikey": [{"key": "X-Key", "value": "k"}, {"key": "in", "value": "header"}]},
          "request": {
            "method": "post",
            "header": [
              {"key": "Content-Type", "value": "application/json"},
              {"key": "X-Off", "value": "1", "disabled": true}
            ],
            "body": {"mode": "raw", "raw": "{\"a\":1}"},
            "url": {
              "raw": "{{host}}/v1/echo?q=1&x=2#frag",
              "query": [{"key": "q", "value": "1"}, {"key": "off", "value": "2", "disabled": true}]
            }
          }
        }
      ]
    },
    {
      "name": "Root Req",
      "request": {
        "method": "GET",
        "header": [],
        "url": "https://example.com/a?b=c"
      }
    }
  ]
}`

func TestPostmanImportExport(t *testing.T) {
	portable, err := ParsePostmanCollection([]byte(postmanFixture))
	if err != nil {
		t.Fatal(err)
	}
	if portable.Name != "Demo" || portable.ID != "ax-1" || portable.Auth == nil || portable.Auth.Type != "bearer" {
		t.Fatalf("collection = %#v", portable)
	}
	if len(portable.Folders) != 1 || len(portable.Requests) != 1 {
		t.Fatalf("items = %d folders, %d requests", len(portable.Folders), len(portable.Requests))
	}
	folder := portable.Folders[0]
	if folder.Name != "Folder A" || folder.Auth == nil || folder.Auth.Type != "basic" || len(folder.Requests) != 1 {
		t.Fatalf("folder = %#v", folder)
	}
	req := folder.Requests[0]
	if req.Method != "POST" || req.URL != "{{host}}/v1/echo#frag" {
		t.Fatalf("request url = %q method = %q", req.URL, req.Method)
	}
	if !strings.Contains(req.HeadersText, "! X-Off: 1") {
		t.Fatalf("headers = %q", req.HeadersText)
	}
	if !strings.Contains(req.ParamsText, "q: 1") || !strings.Contains(req.ParamsText, "! off: 2") {
		t.Fatalf("params = %q", req.ParamsText)
	}
	if req.BodyText != `{"a":1}` || req.Auth == nil || req.Auth.Type != "apikey" {
		t.Fatalf("request = %#v", req)
	}

	paths := PathsAt(t.TempDir(), t.TempDir())
	if err := paths.Ensure(); err != nil {
		t.Fatal(err)
	}
	store, err := OpenStore(paths.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	collID, err := store.ImportPortableCollection(portable)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := store.LoadTree()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Collections) != 1 || len(tree.Folders) != 1 || len(tree.Requests) != 2 {
		t.Fatalf("tree = %d/%d/%d", len(tree.Collections), len(tree.Folders), len(tree.Requests))
	}
	if got := tree.Collections[0].Name; got != "Demo" {
		t.Fatalf("collection name = %q", got)
	}
	if auth := ParseAuth(tree.Collections[0].MetaJSON); auth.Type != "bearer" {
		t.Fatalf("collection auth = %#v", auth)
	}
	var echo Request
	for _, r := range tree.Requests {
		if r.Name == "Echo" {
			echo = r
		}
	}
	if echo.ID == "" || echo.Method != "POST" || echo.URL != "{{host}}/v1/echo#frag" {
		t.Fatalf("echo = %#v", echo)
	}
	if auth := ParseAuth(echo.MetaJSON); auth.Type != "apikey" {
		t.Fatalf("echo auth = %#v", auth)
	}
	if eff := tree.EffectiveAuth(echo.ID); eff.Type != "apikey" {
		t.Fatalf("effective auth = %#v", eff)
	}

	exported, err := store.ExportPortableCollection(collID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := PostmanJSON(exported)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	info := root["info"].(map[string]any)
	if info["_api_x_id"] != collID || info["name"] != "Demo" {
		t.Fatalf("export info = %#v", info)
	}
	if _, ok := root["variable"]; !ok {
		t.Fatalf("variable missing: %s", data)
	}
	round, err := ParsePostmanCollection(data)
	if err != nil {
		t.Fatal(err)
	}
	if round.Name != "Demo" || len(round.Folders) != 1 || len(round.Requests) != 1 {
		t.Fatalf("round trip = %#v", round)
	}
	if round.Folders[0].Requests[0].BodyText != `{"a":1}` {
		t.Fatalf("round body = %q", round.Folders[0].Requests[0].BodyText)
	}
	if !strings.Contains(round.Folders[0].Requests[0].HeadersText, "! X-Off: 1") {
		t.Fatalf("round headers = %q", round.Folders[0].Requests[0].HeadersText)
	}

	path := filepath.Join(t.TempDir(), "out.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestPostmanRejectsOpenAPI(t *testing.T) {
	_, err := ParsePostmanCollection([]byte(`{"info":{"name":"x","schema":"https://spec.openapis.org/oas/v3.0.0"},"paths":{}}`))
	if err == nil || !strings.Contains(err.Error(), "OpenAPI") {
		t.Fatalf("err = %v", err)
	}
}
