package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"api-z/app"
	"api-z/httpclient"
)

type Editor struct {
	root *Root

	ID       string
	Name     string
	Method   string
	URL      string
	Headers  string
	Params   string
	Body     string
	MetaJSON string
	Auth     app.Auth

	Dirty  bool
	ReqTab int

	bodyEd  *editor.State
	bodyVer uint64

	paramsKV  kvState
	headersKV kvState

	StreamMode  int
	RespTab     int
	RawList     ui.ListState
	ModelList   ui.ListState
	AguiList    ui.ListState
	RequestList ui.ListState
	md          markdown.Options

	SelectedEvent int
	EventSplit    float32
	eventIdx      int
	eventEd       *editor.State
	agui          httpclient.AguiSnapshot
	aguiVer       uint64
	aguiRows      []aguiRow
	modelVer      uint64
	modelLines    []string

	jsonOpen       bool
	jsonSource     string
	jsonAutoSource string
	jsonEd         *editor.State

	autoOpen  bool
	autoField string
	autoStart int
	autoEnd   int

	saveMu          sync.Mutex
	saveGen         uint64
	autoSavePending bool

	hist              *app.HARResponse
	histLines         []string
	histList          ui.ListState
	histJSON          bool
	histJSONEd        *editor.State
	histStreamMode    int
	histModelLines    []string
	histModelList     ui.ListState
	histAguiRows      []aguiRow
	histAguiList      ui.ListState
	histEvents        []httpclient.SSEEvent
	histLineEvent     []int
	histSelectedEvent int
	histEventIdx      int
	histEventEd       *editor.State
	histEventSplit    float32
}

type aguiRow struct {
	kind  httpclient.AguiKind
	msg   httpclient.AguiMessage
	text  string
	role  string
	first bool
	last  bool
}

func NewEditor(root *Root, req app.Request) *Editor {
	ed := &Editor{
		root:              root,
		ID:                req.ID,
		Name:              req.Name,
		Method:            req.Method,
		URL:               req.URL,
		Headers:           req.HeadersText,
		Params:            req.ParamsText,
		Body:              req.BodyText,
		MetaJSON:          req.MetaJSON,
		Auth:              app.ParseAuth(req.MetaJSON),
		md:                markdown.Options{},
		SelectedEvent:     -1,
		EventSplit:        70,
		eventIdx:          -1,
		histSelectedEvent: -1,
		histEventIdx:      -1,
		histEventSplit:    70,
	}
	ed.paramsKV.Mode = 1
	ed.headersKV.Mode = 1
	ed.resetBodyEditor()
	return ed
}

func (ed *Editor) resetBodyEditor() {
	buf := editor.NewBuffer(ed.Body)
	buf.SetLanguage(editor.LanguageFor("body.json"))
	ed.bodyEd = &editor.State{Buffer: buf}
	ed.bodyEd.OnCompletion = ed.completeBody
	ed.bodyVer = buf.Version()
}

func (ed *Editor) bodyState() *editor.State {
	if ed.bodyEd == nil {
		ed.resetBodyEditor()
	}
	return ed.bodyEd
}

func (ed *Editor) syncBody() {
	if ed.bodyEd != nil {
		ed.Body = ed.bodyEd.Buffer.String()
	}
}

func (ed *Editor) snapshotRequest() (app.Request, uint64) {
	ed.syncBody()
	meta := app.MergeAuth(ed.MetaJSON, ed.Auth)
	req := app.Request{
		ID:          ed.ID,
		Name:        ed.Name,
		Method:      ed.Method,
		URL:         ed.URL,
		HeadersText: ed.Headers,
		ParamsText:  ed.Params,
		BodyText:    ed.Body,
		MetaJSON:    meta,
	}
	ed.saveMu.Lock()
	ed.saveGen++
	gen := ed.saveGen
	ed.saveMu.Unlock()
	return req, gen
}

// writeSnapshot writes req unless a newer snapshot has replaced it. Older
// background autosaves therefore never overwrite newer manual saves.
func (ed *Editor) writeSnapshot(req app.Request, gen uint64) error {
	ed.saveMu.Lock()
	defer ed.saveMu.Unlock()
	if gen != ed.saveGen {
		return nil
	}
	return ed.root.store.SaveRequest(req)
}

func (ed *Editor) Save() error {
	req, gen := ed.snapshotRequest()
	if err := ed.writeSnapshot(req, gen); err != nil {
		return err
	}
	ed.MetaJSON = req.MetaJSON
	ed.Dirty = false
	return nil
}

// autoSaveNow saves pending edits synchronously (tests and close paths).
func (ed *Editor) autoSaveNow() error {
	if !ed.Dirty {
		return nil
	}
	req, gen := ed.snapshotRequest()
	if err := ed.writeSnapshot(req, gen); err != nil {
		return err
	}
	ed.MetaJSON = req.MetaJSON
	ed.Dirty = false
	return nil
}

// scheduleAutoSave writes edits 450ms after the last change, off the UI
// thread. The snapshot is taken on the UI thread; stale saves are skipped.
func (ed *Editor) scheduleAutoSave() {
	if !ed.Dirty || ed.autoSavePending || ed.root == nil || ed.root.win == nil {
		return
	}
	ed.autoSavePending = true
	win := ed.root.win
	time.AfterFunc(450*time.Millisecond, func() {
		win.Update(func() {
			ed.autoSavePending = false
			if !ed.Dirty {
				return
			}
			req, gen := ed.snapshotRequest()
			ed.MetaJSON = req.MetaJSON
			ed.Dirty = false
			app.SafeGo("ui", func() {
				if err := ed.writeSnapshot(req, gen); err != nil {
					win.Update(func() {
						ed.Dirty = true
						ed.root.err = "自动保存失败: " + err.Error()
					})
				}
			})
		})
	})
}

func (ed *Editor) requestPane(c *ui.Context) {
	defer ed.scheduleAutoSave()
	ui.Column(c).Fill().Children(func() {
		ed.requestHeader(c)
		ui.Tabs(c, &ed.ReqTab, "Body", "Params", "Headers", "Auth")
		switch ed.ReqTab {
		case 0:
			ed.bodyToolbar(c)
			st := ed.bodyState()
			st.Font = ed.root.bodyEditorFont()
			noBody := inferBodyKind(ed.Headers) == bodyNone
			st.ReadOnly = noBody
			view := editor.View(c, st).Grow(1).FillWidth()
			view.Disabled(noBody)
			if st.Buffer.Version() != ed.bodyVer {
				ed.Dirty = true
				ed.bodyVer = st.Buffer.Version()
			}
		case 1:
			ed.root.kvEditor(c, ed, "params", "每行 key: value；`! ` 前缀禁用；与 URL 中已有 query 用 & 合并。", &ed.Params, &ed.paramsKV)
		case 2:
			ed.root.kvEditor(c, ed, "headers", "每行 Name: Value；`! ` 前缀只编辑不发送。", &ed.Headers, &ed.headersKV)
		case 3:
			ed.authPane(c)
		}
	})
}

func (ed *Editor) requestHeader(c *ui.Context) {
	t := c.Theme()
	sess := ed.root.session(ed.ID)
	running := sess.Running()
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	ui.Row(c).Padding(6, 8).Gap(6).AlignItems(ui.Center).Children(func() {
		methodSel := ui.SelectBase(c, &ed.Method)
		methodSel.Trigger.Label("方法").Padding(2, 4).Radius(4).Justify(ui.Start)
		if methodSel.Trigger.Hovered() {
			methodSel.Trigger.Background(t.SurfaceHover)
		}
		methodSel.Trigger.Children(func() {
			ui.Text(c, ed.Method).FontSize(12).Bold().TextColor(methodColor(c, ed.Method))
			ui.Spacer(c)
			ui.Text(c, "▾").FontSize(9).TextColor(t.TextMuted)
		})
		methodSel.Popup(func(panel ui.Element) {
			panel.Padding(4).Background(t.Surface).Border(1, t.Border).Radius(6)
			for _, m := range methods {
				item := methodSel.Item(m).Padding(3, 8).Radius(4)
				if item.Highlighted() {
					item.Background(t.Accent.Alpha(0.14))
				}
				item.Children(func() {
					ui.Text(c, m).FontSize(12).TextColor(methodColor(c, m))
				})
			}
		})
		if methodSel.Trigger.Changed() {
			ed.Dirty = true
			ed.root.updateTreeRequestMethod(ed.ID, ed.Method)
		}
		url := ed.root.urlInput(c, &ed.URL)
		urlChanged := url.Changed()
		if urlChanged {
			ed.Dirty = true
		}
		ed.root.varAutocomplete(c, ed, "url", url, &ed.URL, urlChanged)
		if url.Submitted() {
			ed.send()
		}
		if !running {
			if iconButton(c, iconSend, "发送").Clicked() {
				ed.send()
			}
		} else {
			if ui.Button(c, "取消").Clicked() {
				sess.Cancel()
			}
		}
		if ed.Dirty {
			ui.Text(c, "●").FontSize(10).TextColor(t.Warning).Tooltip("有未保存的修改")
		}
	})
}

func (ed *Editor) authPane(c *ui.Context) {
	t := c.Theme()
	types := []string{"inherit", "noauth", "bearer", "basic", "apikey"}
	ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "认证类型").Width(140).TextColor(t.TextMuted)
			if ui.Select(c, &ed.Auth.Type, types).Width(180).Changed() {
				ed.Dirty = true
			}
		})
		switch ed.Auth.Type {
		case "inherit":
			eff := ed.root.tree.EffectiveAuth(ed.ID)
			label := authTypeLabel(eff.Type)
			if eff.Type == "" || eff.Type == "inherit" {
				ui.Text(c, "父级未配置认证，发送时不会附加认证头。").
					FontSize(12).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "继承自集合 / 文件夹："+label).FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, "发送时自动注入；切换到其他类型可覆盖。").
					FontSize(11).TextColor(t.TextMuted)
			}
		case "noauth":
			ui.Text(c, "不附加认证头，也不会继承父级认证。").TextColor(t.TextMuted)
		case "bearer":
			ed.authField(c, "Token", "token", &ed.Auth.Bearer, true)
			ui.Text(c, "发送时附加 Authorization: Bearer <token>。").
				FontSize(11).TextColor(t.TextMuted)
		case "basic":
			ed.authField(c, "用户名", "username", &ed.Auth.Basic, false)
			ed.authField(c, "密码", "password", &ed.Auth.Basic, true)
			ui.Text(c, "发送时附加 Authorization: Basic base64(username:password)。").
				FontSize(11).TextColor(t.TextMuted)
		case "apikey":
			ed.authField(c, "Key", "key", &ed.Auth.APIKey, false)
			ed.authField(c, "Value", "value", &ed.Auth.APIKey, true)
			in := authValue(ed.Auth.APIKey, "in")
			if in == "" {
				in = "header"
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "添加到").Width(140).TextColor(t.TextMuted)
				if ui.Select(c, &in, []string{"header", "query"}).Label("API Key 位置").Width(160).Changed() {
					setAuthValue(&ed.Auth.APIKey, "in", in)
					ed.Dirty = true
				}
			})
			ui.Text(c, "header 写入请求头；query 追加到 URL 查询参数。").
				FontSize(11).TextColor(t.TextMuted)
		}
	})
}

func (ed *Editor) authField(c *ui.Context, label, key string, list *[]app.AuthProperty, secret bool) {
	t := c.Theme()
	val := authValue(*list, key)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(140).TextColor(t.TextMuted)
		input := ui.TextInput(c, &val).Grow(1)
		if secret {
			input = input.Password()
		}
		if input.Changed() {
			setAuthValue(list, key, val)
			ed.Dirty = true
		}
	})
}

func authTypeLabel(typ string) string {
	switch typ {
	case "bearer":
		return "Bearer Token"
	case "basic":
		return "Basic Auth"
	case "apikey":
		return "API Key"
	case "noauth":
		return "无认证"
	default:
		return "Inherit"
	}
}

func authValue(list []app.AuthProperty, key string) string {
	for _, p := range list {
		if p.Key == key {
			return p.Value
		}
	}
	return ""
}

func setAuthValue(list *[]app.AuthProperty, key, value string) {
	for i := range *list {
		if (*list)[i].Key == key {
			(*list)[i].Value = value
			return
		}
	}
	*list = append(*list, app.AuthProperty{Key: key, Value: value, Type: "string"})
}

func (ed *Editor) buildSpec() httpclient.Spec {
	ed.syncBody()
	auth := ed.Auth
	if strings.EqualFold(auth.Type, "inherit") || auth.Type == "" {
		auth = ed.root.tree.EffectiveAuth(ed.ID)
	}
	body := ed.Body
	if inferBodyKind(ed.Headers) == bodyNone {
		body = ""
	}
	spec := app.BuildSpec(ed.Method, ed.URL, ed.Headers, ed.Params, body, auth, ed.root.env.Map())
	readMS, totalMS := ed.root.requestTimeoutsMS()
	spec.ReadTimeout = time.Duration(readMS) * time.Millisecond
	spec.RequestTimeout = time.Duration(totalMS) * time.Millisecond
	return spec
}

func (ed *Editor) send() {
	if ed.root == nil || ed.root.store == nil {
		return
	}
	if err := ed.Save(); err != nil {
		ed.root.err = err.Error()
		return
	}
	spec := ed.buildSpec()
	ed.root.err = ""
	ed.SelectedEvent = -1
	ed.eventIdx = -1
	ed.eventEd = nil
	ed.jsonAutoSource = ""
	ed.root.touchMRU(ed.ID)
	sess := ed.root.session(ed.ID)
	sess.Start(spec)
	ed.root.status = "请求中: " + ed.Method + " " + app.RedactURL(spec.URL)
}

func (ed *Editor) bodyToolbar(c *ui.Context) {
	t := c.Theme()
	kind := inferBodyKind(ed.Headers)
	options := []string{"无 Body", "JSON", "表单", "XML"}
	cur := options[kind]
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(4, 6).BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).Children(func() {
		ui.Text(c, "类型").FontSize(11).TextColor(t.TextMuted)
		kindSel := ui.SelectBase(c, &cur)
		kindSel.Trigger.Label("Body 类型").Padding(2, 6).Radius(4)
		if kindSel.Trigger.Hovered() {
			kindSel.Trigger.Background(t.SurfaceHover)
		}
		kindSel.Trigger.Children(func() {
			ui.Text(c, cur).FontSize(12).TextColor(t.Text)
			ui.Box(c).Width(4)
			ui.Text(c, "▾").FontSize(9).TextColor(t.TextMuted)
		})
		kindSel.Popup(func(panel ui.Element) {
			panel.Padding(4).Background(t.Surface).Border(1, t.Border).Radius(6)
			for _, option := range options {
				item := kindSel.Item(option).Padding(3, 8).Radius(4)
				if item.Highlighted() {
					item.Background(t.Accent.Alpha(0.14))
				}
				item.Children(func() { ui.Text(c, option).FontSize(12) })
			}
		})
		if kindSel.Trigger.Changed() {
			ed.Headers = setBodyContentType(ed.Headers, cur)
			ed.headersKV.Source = ""
			ed.Dirty = true
		}
		ui.Spacer(c)
		if kind == 1 {
			if iconButtonPlain(c, iconFormat, "格式化 JSON").
				Disabled(strings.TrimSpace(ed.bodyState().Buffer.String()) == "").Clicked() {
				if formatted, ok := prettyJSON(ed.bodyState().Buffer.String()); ok {
					ed.Body = formatted
					ed.resetBodyEditor()
					ed.Dirty = true
				}
			}
		}
	})
}

const (
	bodyNone = iota
	bodyJSON
	bodyForm
	bodyXML
)

func inferBodyKind(headers string) int {
	for _, line := range strings.Split(headers, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "! ") {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(name), "Content-Type") {
			continue
		}
		ct := strings.ToLower(strings.TrimSpace(value))
		switch {
		case strings.Contains(ct, "json"):
			return bodyJSON
		case strings.Contains(ct, "x-www-form-urlencoded"):
			return bodyForm
		case strings.Contains(ct, "xml"):
			return bodyXML
		}
	}
	return bodyNone
}

func setBodyContentType(headers, label string) string {
	var ct string
	switch label {
	case "JSON":
		ct = "application/json"
	case "表单":
		ct = "application/x-www-form-urlencoded"
	case "XML":
		ct = "application/xml"
	}
	var lines []string
	for _, line := range strings.Split(headers, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		name, _, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(trimmed, "! ")), ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Type") &&
			!strings.HasPrefix(trimmed, "! ") {
			continue
		}
		lines = append(lines, line)
	}
	if ct != "" {
		lines = append(lines, "Content-Type: "+ct)
	}
	return strings.Join(lines, "\n")
}

func prettyJSON(text string) (string, bool) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(text), "", "  "); err != nil {
		return text, false
	}
	return buf.String(), true
}
