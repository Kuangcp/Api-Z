package ui

import (
	"strings"
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

	StreamMode int
	RawList    ui.ListState
	ModelList  ui.ListState
	AguiList   ui.ListState
	md         markdown.Options
	agui       httpclient.AguiSnapshot
	aguiVer    uint64
	aguiRows   []aguiRow
	modelVer   uint64
	modelLines []string

	jsonOpen   bool
	jsonSource string
	jsonEd     *editor.State

	autoOpen  bool
	autoField string
	autoStart int
	autoEnd   int

	hist       *app.HARResponse
	histLines  []string
	histList   ui.ListState
	histJSON   bool
	histJSONEd *editor.State
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
		root:     root,
		ID:       req.ID,
		Name:     req.Name,
		Method:   req.Method,
		URL:      req.URL,
		Headers:  req.HeadersText,
		Params:   req.ParamsText,
		Body:     req.BodyText,
		MetaJSON: req.MetaJSON,
		Auth:     app.ParseAuth(req.MetaJSON),
		md:       markdown.Options{},
	}
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

func (ed *Editor) Save() error {
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
	if err := ed.root.store.SaveRequest(req); err != nil {
		return err
	}
	ed.MetaJSON = meta
	ed.Dirty = false
	return nil
}

func (ed *Editor) requestPane(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		ed.requestHeader(c)
		ui.Tabs(c, &ed.ReqTab, "Params", "Headers", "Body", "Auth")
		switch ed.ReqTab {
		case 0:
			area := ui.TextArea(c, &ed.Params).Grow(1).Font("monospace")
			changed := area.Changed()
			if changed {
				ed.Dirty = true
			}
			ed.root.varAutocomplete(c, ed, "params", area, &ed.Params, changed)
		case 1:
			area := ui.TextArea(c, &ed.Headers).Grow(1).Font("monospace")
			changed := area.Changed()
			if changed {
				ed.Dirty = true
			}
			ed.root.varAutocomplete(c, ed, "headers", area, &ed.Headers, changed)
		case 2:
			st := ed.bodyState()
			editor.View(c, st).Grow(1).FillWidth()
			if st.Buffer.Version() != ed.bodyVer {
				ed.Dirty = true
				ed.bodyVer = st.Buffer.Version()
			}
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
		if ui.Select(c, &ed.Method, methods).Label("方法").Width(96).Changed() {
			ed.Dirty = true
		}
		url := ui.TextInput(c, &ed.URL).Placeholder("https://{{host}}/api").Grow(1)
		urlChanged := url.Changed()
		if urlChanged {
			ed.Dirty = true
		}
		ed.root.varAutocomplete(c, ed, "url", url, &ed.URL, urlChanged)
		if url.Submitted() {
			ed.send()
		}
		if !running {
			if ui.PrimaryButton(c, "发送").Clicked() {
				ed.send()
			}
		} else {
			if ui.Button(c, "取消").Clicked() {
				sess.Cancel()
			}
		}
		if ui.Button(c, "保存").Clicked() {
			ed.root.saveEditor(ed)
			c.Toast("已保存")
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
			ui.Text(c, "认证类型").Width(80).TextColor(t.TextMuted)
			if ui.Select(c, &ed.Auth.Type, types).Width(180).Changed() {
				ed.Dirty = true
			}
		})
		switch ed.Auth.Type {
		case "inherit":
			ui.Text(c, "继承集合 / 文件夹的认证；发送时解析。").TextColor(t.TextMuted)
		case "noauth":
			ui.Text(c, "不附加认证头。").TextColor(t.TextMuted)
		case "bearer":
			ed.authField(c, "Token", "token", &ed.Auth.Bearer, true)
		case "basic":
			ed.authField(c, "用户名", "username", &ed.Auth.Basic, false)
			ed.authField(c, "密码", "password", &ed.Auth.Basic, true)
		case "apikey":
			ed.authField(c, "Key", "key", &ed.Auth.APIKey, false)
			ed.authField(c, "Value", "value", &ed.Auth.APIKey, true)
			ed.authField(c, "位置（header/query）", "in", &ed.Auth.APIKey, false)
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

func (ed *Editor) send() {
	if ed.root == nil || ed.root.store == nil {
		return
	}
	if err := ed.Save(); err != nil {
		ed.root.err = err.Error()
		return
	}
	vars := ed.root.env.Map()
	auth := ed.Auth
	if strings.EqualFold(auth.Type, "inherit") || auth.Type == "" {
		auth = ed.root.tree.EffectiveAuth(ed.ID)
	}
	params := app.ParseParams(app.ApplyVariables(ed.Params, vars))
	params = append(params, app.ResolveAuthQuery(auth, vars)...)
	url := app.ApplyVariables(ed.URL, vars)
	url = app.EnsureDefaultScheme(app.MergeURLWithParams(url, params))

	headers := app.HeadersForSend(app.ParseHeaders(app.ApplyVariables(ed.Headers, vars)))
	headers = append(headers, app.ResolveAuth(auth, vars)...)
	body := []byte(app.ApplyVariables(ed.Body, vars))

	hdr := make([]httpclient.Header, 0, len(headers))
	for _, h := range headers {
		hdr = append(hdr, httpclient.Header{Name: h.Name, Value: h.Value})
	}
	spec := httpclient.Spec{
		Method:         ed.Method,
		URL:            url,
		Headers:        hdr,
		Body:           body,
		RequestTimeout: 120 * time.Second,
	}
	ed.root.err = ""
	sess := ed.root.session(ed.ID)
	sess.Start(spec)
	ed.root.status = "请求中: " + ed.Method + " " + app.RedactURL(url)
}
