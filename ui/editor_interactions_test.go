package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func openEditorTest(t *testing.T, headers, body string) (*Root, *Editor, *ui.Tester) {
	t.Helper()
	store, paths := collectionTestStore(t)
	cid, _ := store.CreateCollection("Demo")
	rid, _ := store.CreateRequest(cid, "", "Echo")
	_ = store.SaveRequest(app.Request{
		ID: rid, Name: "Echo", Method: "POST", URL: "https://example.com/echo",
		HeadersText: headers, BodyText: body,
	})
	tree, _ := store.LoadTree()
	root := NewRoot(paths, store, tree, app.EnvState{Version: 1})
	root.openRequest(rid)
	tt := ui.NewTester(root.View, 1280, 760)
	tt.Frame()
	return root, root.tabs[0], tt
}

func TestRequestResponseSideBySide(t *testing.T) {
	_, _, tt := openEditorTest(t, "", "")
	send, ok := tt.Find("发送")
	if !ok {
		t.Fatal("send button not found")
	}
	response, ok := tt.Find("未发送")
	if !ok {
		t.Fatal("response pane not found")
	}
	if response.X <= send.X {
		t.Fatalf("response is not to the right: send=%v response=%v", send, response)
	}
	if response.Y > send.Y+80 {
		t.Fatalf("response is below the request: send=%v response=%v", send, response)
	}
}

func TestHeadersFormEdit(t *testing.T) {
	root, ed, tt := openEditorTest(t, "X-Test: v", "")
	if err := tt.Click("Headers"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("表单") || !tt.HasText("添加一行") {
		t.Fatalf("form headers not shown: %v", tt.Texts())
	}
	if len(ed.headersKV.Rows) != 1 || ed.headersKV.Rows[0].Key != "X-Test" || ed.headersKV.Rows[0].Value != "v" {
		t.Fatalf("form rows = %#v", ed.headersKV.Rows)
	}
	if err := tt.Click("添加一行"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(ed.headersKV.Rows) != 2 {
		t.Fatalf("rows after add = %#v", ed.headersKV.Rows)
	}
	if err := tt.Click("文本"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// Committing the extra empty row leaves the saved text unchanged.
	if ed.Headers != "X-Test: v" {
		t.Fatalf("headers after mode switch = %q", ed.Headers)
	}
	_ = root
}

func TestBodyTypeSelector(t *testing.T) {
	_, ed, tt := openEditorTest(t, "Content-Type: application/json", `{"a":1}`)
	if err := tt.Click("Body"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("类型") || !tt.HasText("JSON") || !tt.HasText("格式化 JSON") {
		t.Fatalf("body toolbar = %v", tt.Texts())
	}
	if err := tt.Click("格式化 JSON"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := ed.bodyState().Buffer.String(); !strings.Contains(got, "\"a\": 1") {
		t.Fatalf("formatted body = %q", got)
	}
	// Select opens an in-window popup; choose its option by label.
	if err := tt.Click("JSON"); err != nil {
		t.Fatalf("open body type: %v", tt.Texts())
	}
	tt.Frame()
	if err := tt.Click("XML"); err != nil {
		t.Fatalf("type options: %v", tt.Texts())
	}
	tt.Frame()
	if err := tt.Click("XML"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("表单"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !strings.Contains(ed.Headers, "Content-Type: application/x-www-form-urlencoded") {
		t.Fatalf("headers = %q", ed.Headers)
	}
}

func TestBodyKindHelpers(t *testing.T) {
	if got := setBodyContentType("A: b\nContent-Type: text/plain\n! Content-Type: x", "JSON"); got != "A: b\n! Content-Type: x\nContent-Type: application/json" {
		t.Fatalf("set json = %q", got)
	}
	if got := setBodyContentType("Content-Type: application/json", "无 Body"); got != "" {
		t.Fatalf("remove = %q", got)
	}
	if got := inferBodyKind("Content-Type: application/xml"); got != bodyXML {
		t.Fatalf("kind = %d", got)
	}
}

func TestAuthAPIKeyInteraction(t *testing.T) {
	_, ed, tt := openEditorTest(t, "", "")
	if err := tt.Click("Auth"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("inherit"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("apikey"); err != nil {
		t.Fatalf("auth options: %v", tt.Texts())
	}
	tt.Frame()
	if ed.Auth.Type != "apikey" || !tt.HasText("添加到") {
		t.Fatalf("auth = %#v texts=%v", ed.Auth, tt.Texts())
	}
	if err := tt.Click("header"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("query"); err != nil {
		t.Fatalf("location options: %v", tt.Texts())
	}
	tt.Frame()
	if got := authValue(ed.Auth.APIKey, "in"); got != "query" {
		t.Fatalf("apikey in = %q", got)
	}
}

func TestKVParseCommit(t *testing.T) {
	rows, raw := parseKV("A: 1\n! B: 2\nbad line\nC:")
	if len(rows) != 3 || rows[1].Enabled || rows[1].Key != "B" || rows[2].Value != "" {
		t.Fatalf("rows = %#v", rows)
	}
	if len(raw) != 1 || raw[0] != "bad line" {
		t.Fatalf("raw = %#v", raw)
	}
	kv := kvState{Rows: rows, Raw: raw}
	var out string
	kv.commit(&out)
	if out != "A: 1\n! B: 2\nC: \nbad line" {
		t.Fatalf("commit = %q", out)
	}
}

func TestNoBodyKeepsContentButDisablesAndSkipsSend(t *testing.T) {
	_, ed, tt := openEditorTest(t, "Content-Type: application/json", `{"a":1}`)
	if err := tt.Click("Body"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if ed.bodyState().ReadOnly {
		t.Fatal("JSON body should be editable")
	}
	if err := tt.Click("JSON"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("无 Body"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !ed.bodyState().ReadOnly {
		t.Fatal("no-body editor is still editable")
	}
	if ed.Body != `{"a":1}` || ed.bodyState().Buffer.String() != `{"a":1}` {
		t.Fatalf("body content changed: %q / %q", ed.Body, ed.bodyState().Buffer.String())
	}
	if spec := ed.buildSpec(); len(spec.Body) != 0 {
		t.Fatalf("no-body request sends %q", spec.Body)
	}
}

func TestMethodChangeUpdatesTree(t *testing.T) {
	root, ed, tt := openEditorTest(t, "", "")
	if err := tt.Click("方法"); err != nil {
		t.Fatalf("method select: %v", err)
	}
	tt.Frame()
	if err := tt.Click("PATCH"); err != nil {
		t.Fatalf("method option: %v", err)
	}
	tt.Frame()
	if ed.Method != "PATCH" {
		t.Fatalf("editor method = %q", ed.Method)
	}
	req, ok := root.tree.Request(ed.ID)
	if !ok || req.Method != "PATCH" {
		t.Fatalf("tree request = %#v", req)
	}
	for _, row := range root.visibleRows() {
		if row.id == ed.ID && row.method != "PATCH" {
			t.Fatalf("visible row method = %q", row.method)
		}
	}
}
