package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"api-z/app"
	"api-z/httpclient"
)

const (
	rowCollection = iota
	rowFolder
	rowRequest
)

type treeRow struct {
	kind   int
	depth  int
	id     string
	name   string
	method string
	url    string
	open   bool
}

type dialogKind int

const (
	dialogNone dialogKind = iota
	dialogRename
	dialogDelete
)

type Root struct {
	win    *mygo.Window
	paths  app.Paths
	store  *app.Store
	client *httpclient.Client

	tree     app.Tree
	expanded map[string]bool
	treeList ui.ListState
	search   string
	selected string

	activeCollection string
	colMRU           []string

	tabs   []*Editor
	active int

	sessions map[string]*app.Session

	env      app.EnvState
	envName  string
	envNames []string

	envMenuOpen bool
	envGroup    int

	sidebarWidth   float32
	requestPercent float32
	status         string
	err            string

	dialog     dialogKind
	dialogOpen bool
	dlgKind    string
	dlgID      string
	dlgName    string
	dlgErr     string

	mru         []string
	searchOpen  bool
	globalQuery string
	globalList  ui.ListState

	envOpen  bool
	envDraft app.EnvState
	envSel   int
	envErr   string

	moveOpen       bool
	moveKind       string
	moveID         string
	moveName       string
	moveCollection string
	moveFolder     string
	moveErr        string

	settingsOpen bool
	settingsTab  int

	nodeSettingsOpen     bool
	nodeSettingsKind     string
	nodeSettingsID       string
	nodeSettingsSection  int
	nodeSettingsAuth     app.Auth
	nodeSettingsSource   string
	nodeSettingsRootMode int
	nodeSettingsEnvKey   string
	nodeSettingsErr      string
	appIcon              ui.ImageSource

	themePrefs  app.ThemePrefs
	themePath   string
	themeKey    string
	sessionPath string
	baseTheme   *ui.Theme
	accentHex   string
	fontInput   string

	keymap      map[string]string
	keymapPath  string
	recording   string
	recordMsg   string
	settingsErr string

	openAPIOpen   bool
	openAPIName   string
	openAPIURL    string
	openAPIMode   int
	openAPIEnvKey string
	openAPIBusy   bool
	openAPIErr    string

	curlOpen bool
	curlText string
	curlErr  string
}

func NewRoot(paths app.Paths, store *app.Store, tree app.Tree, env app.EnvState) *Root {
	r := &Root{
		paths:          paths,
		store:          store,
		client:         httpclient.NewClient(),
		tree:           tree,
		expanded:       map[string]bool{},
		sessions:       map[string]*app.Session{},
		env:            env.Normalize(),
		sidebarWidth:   300,
		requestPercent: 50,
		themePath:      app.ThemePath(paths.ConfigDir),
		keymapPath:     app.KeymapPath(paths.ConfigDir),
		sessionPath:    app.SessionPath(paths.ConfigDir),
	}
	for _, c := range tree.Collections {
		r.expanded["c:"+c.ID] = true
	}
	if a := env.Active(); a != nil {
		r.envName = a.Name
	}
	r.envNames = envNames(env)
	r.themePrefs = app.LoadThemePrefs(r.themePath)
	r.accentHex = r.themePrefs.Accent
	r.fontInput = r.themePrefs.Font
	r.keymap = mergedKeymap(app.LoadKeymap(r.keymapPath))
	r.ensureActiveCollection()
	r.restoreLastRequest()
	r.status = fmt.Sprintf("%d 个集合 · %d 个请求", len(tree.Collections), len(tree.Requests))
	return r
}

func (r *Root) Attach(win *mygo.Window) {
	r.win = win
	win.OnClose(func(*mygo.CloseEvent) {
		for _, ed := range r.tabs {
			if ed.Dirty {
				_ = ed.Save()
			}
		}
	})
}

// SetAppIconPNG provides the icon drawn in the custom title bar.
func (r *Root) SetAppIconPNG(png []byte) {
	if src, err := ui.DecodeImage(png); err == nil {
		r.appIcon = src
	}
}

func (r *Root) View(c *ui.Context) {
	r.applyTheme(c)
	r.handleRecording(c)
	r.handleShortcuts(c)
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		r.titleBar(c)
		ui.Split(c, &r.sidebarWidth, func() {
			r.sidebar(c)
		}, func() {
			r.main(c)
		}).Grow(1)
		ui.Row(c).Padding(4, 12).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, r.status).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
			ui.Spacer(c)
			if r.err != "" {
				ui.Text(c, r.err).FontSize(11.5).TextColor(t.Danger).SingleLine().MaxWidth(500).Tooltip(r.err)
				ui.Spacer(c)
			}
			ui.Text(c, r.paths.DataDir).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
		})
	})
	r.dialogs(c)
	r.globalSearch(c)
	r.envManager(c)
	r.moveModal(c)
	r.settingsModal(c)
	r.nodeSettingsModal(c)
	r.openAPIDialog(c)
	r.curlDialog(c)
}

const titleBarHeight = 38

// titleBar is the frameless window's own title bar: drag anywhere on the
// strip, double-click to maximize, window controls on the right.
func (r *Root) titleBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(titleBarHeight).FillWidth().Padding(0, 12).Gap(8).
		AlignItems(ui.Center).DragWindow().Background(t.Surface).Children(func() {
		if r.appIcon != nil {
			ui.Image(c, r.appIcon).Width(20).Height(20)
		}
		ui.Text(c, "Api-Z").FontSize(12.5).Bold()
		ui.Spacer(c)
		if iconButtonPlain(c, iconTerminal, "从剪贴板导入 cURL").Clicked() {
			r.importCurlFromClipboard(c)
		}
		r.envMenu(c)
		iconMenuButton(c, iconDatabase, "数据目录", func(m *ui.Menu) {
			if m.Item("Push 到 data/").Chosen() {
				r.pushDataDir()
			}
			if m.Item("从 data/ Pull").Chosen() {
				r.pullDataDir()
			}
		})
		if iconButtonPlain(c, iconSettings, "设置").Clicked() {
			r.settingsOpen = true
		}
		ui.Box(c).Width(6)
		if r.win != nil {
			if iconButtonPlain(c, iconMinus, "最小化").Clicked() {
				r.win.Minimize()
			}
			if r.win.IsMaximized() {
				if iconButtonPlain(c, iconRestore, "还原").Clicked() {
					r.win.ToggleMaximize()
				}
			} else if iconButtonPlain(c, iconMaximize, "最大化").Clicked() {
				r.win.ToggleMaximize()
			}
			if iconButtonPlain(c, iconX, "关闭窗口").Clicked() {
				r.win.Close()
			}
		}
	})
}

func (r *Root) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(6).Children(func() {
		r.collectionTabBar(c)
		ui.Row(c).Gap(6).Children(func() {
			ui.TextInput(c, &r.search).Placeholder("搜索当前集合 / URL").Grow(1)
			iconButtonPlain(c, iconX, "清空搜索").Disabled(r.search == "").OnClick(func() { r.search = "" })
		})
		ui.Row(c).Gap(6).Children(func() {
			if iconButton(c, iconFolderPlus, "新建集合").Clicked() {
				r.createCollection()
			}
			iconMenuButton(c, iconUpload, "导入", func(m *ui.Menu) {
				if m.Item("Postman Collection…").Chosen() {
					r.importPostmanDialog()
				}
				if m.Item("OpenAPI…").Chosen() {
					r.openOpenAPIDialog()
				}
				if m.Item("cURL…").Chosen() {
					r.openCurlDialog(c)
				}
			})
			ui.Spacer(c)
			ui.Text(c, fmt.Sprintf("%d 个请求", len(r.tree.Requests))).FontSize(11).TextColor(t.TextMuted)
		})
		rows := r.visibleRows()
		ui.List(c, &r.treeList, len(rows), func(i int) {
			if i < 0 || i >= len(rows) {
				return
			}
			r.treeRow(c, rows[i])
		}).Grow(1).Children(func() {
			if len(rows) == 0 {
				msg := "没有匹配的请求"
				if r.search == "" {
					if len(r.tree.Collections) == 0 {
						msg = "还没有集合"
					} else {
						msg = "该集合还没有请求"
					}
				}
				ui.Text(c, msg).Padding(12).TextColor(t.TextMuted)
			}
		})
	})
}

func (r *Root) treeRow(c *ui.Context, row treeRow) {
	t := c.Theme()
	selected := r.selected == row.id
	btn := ui.ButtonBase(c).Label(rowLabel(row)).Justify(ui.Start).
		Padding(2, 6).PaddingStart(4 + float32(row.depth)*14).Radius(5)
	if selected {
		btn.Background(t.Accent.Alpha(0.14))
	} else if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	btn.Children(func() {
		switch row.kind {
		case rowCollection:
			chevron := iconChevronRight
			folder := iconFolder
			if row.open {
				chevron = iconChevronDown
				folder = iconFolderOpen
			}
			ui.Icon(c, chevron).FontSize(13).TextColor(t.TextMuted)
			ui.Icon(c, folder).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, row.name).FontSize(12.5).Bold().SingleLine().Grow(1)
		case rowFolder:
			chevron := iconChevronRight
			folder := iconFolder
			if row.open {
				chevron = iconChevronDown
				folder = iconFolderOpen
			}
			ui.Icon(c, chevron).FontSize(13).TextColor(t.TextMuted)
			ui.Icon(c, folder).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, row.name).FontSize(12.5).SingleLine().Grow(1)
		case rowRequest:
			if r.requestRunning(row.id) {
				ui.Spinner(c).FontSize(11).Label("请求中")
			} else {
				ui.Text(c, " ").FontSize(10)
			}
			ui.Text(c, row.method).FontSize(9.5).Bold().TextColor(methodColor(c, row.method)).Width(42)
			ui.Text(c, row.name).FontSize(12.5).SingleLine().Grow(1)
		}
	})
	if btn.Clicked() {
		r.selected = row.id
		switch row.kind {
		case rowCollection:
			r.expanded[row.id] = !r.expanded[row.id]
		case rowFolder:
			r.expanded[row.id] = !r.expanded[row.id]
		case rowRequest:
			r.openRequest(row.id)
		}
	}
	if row.kind == rowRequest && btn.DoubleClicked() {
		r.beginRename(app.NodeRequest, row.id, row.name)
	}
	btn.ContextMenu(func(m *ui.Menu) {
		r.treeContextMenu(c, m, row)
	})
	kind, id := r.nodeIdentity(row)
	attachTreeDrag(btn, kind, id)
	r.attachTreeDrop(c, btn, row)
}

func rowLabel(row treeRow) string {
	if row.kind == rowRequest {
		return row.method + " " + row.name
	}
	return row.name
}

func methodColor(c *ui.Context, method string) ui.Color {
	t := c.Theme()
	switch strings.ToUpper(method) {
	case "GET":
		return t.Success
	case "POST":
		return t.Accent
	case "PUT", "PATCH":
		return t.Warning
	case "DELETE":
		return t.Danger
	default:
		return t.TextMuted
	}
}

func (r *Root) visibleRows() []treeRow {
	search := strings.ToLower(strings.TrimSpace(r.search))
	col, ok := r.tree.Collection(r.activeCollection)
	if !ok {
		return nil
	}
	rows, matched := r.collectionRows(col, 0, search)
	if search != "" && !matched {
		return nil
	}
	return rows
}

func (r *Root) collectionRows(col app.Collection, depth int, search string) ([]treeRow, bool) {
	key := "c:" + col.ID
	row := treeRow{kind: rowCollection, depth: depth, id: key, name: "根目录", open: r.expanded[key]}
	rows := []treeRow{row}
	// The collection name lives in the tab strip; the root row stands for the
	// collection in search results only through its children.
	matched := false
	if r.expanded[key] || search != "" {
		for _, child := range r.folderChildren(col.ID, "") {
			childRows, ok := r.folderRows(col.ID, child, depth+1, search)
			if ok {
				rows = append(rows, childRows...)
				matched = true
			}
		}
		for _, req := range r.requestsIn(col.ID, "") {
			if !matches(search, req.Name, req.URL+" "+req.Method) {
				continue
			}
			matched = true
			rows = append(rows, treeRow{kind: rowRequest, depth: depth + 1, id: req.ID, name: req.Name, method: req.Method, url: req.URL})
		}
	}
	return rows, matched
}

func (r *Root) folderRows(collectionID string, f app.Folder, depth int, search string) ([]treeRow, bool) {
	key := "f:" + f.ID
	row := treeRow{kind: rowFolder, depth: depth, id: key, name: f.Name, open: r.expanded[key]}
	rows := []treeRow{row}
	matched := matches(search, f.Name, "")
	if r.expanded[key] || search != "" {
		for _, child := range r.folderChildren(collectionID, f.ID) {
			childRows, ok := r.folderRows(collectionID, child, depth+1, search)
			if ok {
				rows = append(rows, childRows...)
				matched = true
			}
		}
		for _, req := range r.requestsIn(collectionID, f.ID) {
			if !matches(search, req.Name, req.URL+" "+req.Method) {
				continue
			}
			matched = true
			rows = append(rows, treeRow{kind: rowRequest, depth: depth + 1, id: req.ID, name: req.Name, method: req.Method, url: req.URL})
		}
	}
	return rows, matched
}

func (r *Root) folderChildren(collectionID, parentID string) []app.Folder {
	var out []app.Folder
	for _, f := range r.tree.Folders {
		if f.CollectionID == collectionID && f.ParentID == parentID {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out
}

func (r *Root) requestsIn(collectionID, folderID string) []app.Request {
	var out []app.Request
	for _, req := range r.tree.Requests {
		if req.CollectionID == collectionID && req.FolderID == folderID {
			out = append(out, req)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SortOrder < out[j].SortOrder })
	return out
}

func matches(search, name, extra string) bool {
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), search) || strings.Contains(strings.ToLower(extra), search)
}

func (r *Root) reloadTree() {
	tree, err := r.store.LoadTree()
	if err != nil {
		r.err = err.Error()
		return
	}
	r.tree = tree
	r.ensureActiveCollection()
}

func (r *Root) openRequest(id string) {
	for i, ed := range r.tabs {
		if ed.ID == id {
			r.active = i
			r.selected = id
			r.touchMRU(id)
			r.rememberActiveRequest()
			return
		}
	}
	req, ok, err := r.store.GetRequest(id)
	if err != nil || !ok {
		r.err = fmt.Sprintf("读取请求失败: %v", err)
		return
	}
	if meta, found := r.tree.Request(id); found {
		r.activateCollection(meta.CollectionID)
	}
	ed := NewEditor(r, req)
	r.tabs = append(r.tabs, ed)
	r.active = len(r.tabs) - 1
	r.selected = id
	r.touchMRU(id)
	r.rememberActiveRequest()
}

// restoreLastRequest reopens the request the previous session left active.
// A request that no longer exists leaves the app on the blank state.
func (r *Root) restoreLastRequest() {
	if r.sessionPath == "" {
		return
	}
	prefs := app.LoadSessionPrefs(r.sessionPath)
	if prefs.LastRequest == "" {
		return
	}
	if _, ok := r.tree.Request(prefs.LastRequest); !ok {
		_ = app.SaveSessionPrefs(r.sessionPath, app.SessionPrefs{})
		return
	}
	r.openRequest(prefs.LastRequest)
}

func (r *Root) rememberActiveRequest() {
	if r.sessionPath == "" {
		return
	}
	id := ""
	if r.active >= 0 && r.active < len(r.tabs) {
		id = r.tabs[r.active].ID
	}
	_ = app.SaveSessionPrefs(r.sessionPath, app.SessionPrefs{LastRequest: id})
}

func (r *Root) requestRunning(id string) bool {
	s := r.sessions[id]
	return s != nil && s.Running()
}

func (r *Root) session(id string) *app.Session {
	if s := r.sessions[id]; s != nil {
		return s
	}
	s := app.NewSession(id, r.paths, r.store, r.client)
	if r.win != nil {
		win := r.win
		s.SetNotify(func() { win.Update(func() {}) })
	}
	r.sessions[id] = s
	return s
}

func (r *Root) main(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Children(func() {
		if len(r.tabs) == 0 {
			ui.Text(c, "从左侧选择一个请求开始调试").FontSize(14).TextColor(t.TextMuted).Margin(ui.Auto)
			return
		}
		if r.active < 0 || r.active >= len(r.tabs) {
			r.active = 0
		}
		r.tabBar(c)
		ed := r.tabs[r.active]
		ui.SplitPercent(c, &r.requestPercent, func() {
			ed.requestPane(c)
		}, func() {
			ed.responsePane(c)
		}).Grow(1)
	})
}

func (r *Root) tabBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(2).Padding(4, 6).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		for i, ed := range r.tabs {
			i, ed := i, ed
			label := ed.Method + " " + ed.Name
			if ed.Dirty {
				label = "● " + label
			}
			btn := ui.ButtonBase(c).Label(label).Padding(3, 10).Radius(5)
			if i == r.active {
				btn.Background(t.Accent.Alpha(0.14))
			} else if btn.Hovered() {
				btn.Background(t.SurfaceHover)
			}
			btn.Children(func() {
				ui.Text(c, ed.Method).FontSize(10).Bold().TextColor(methodColor(c, ed.Method))
				ui.Text(c, ed.Name).FontSize(12).SingleLine().MaxWidth(180)
			})
			if btn.Clicked() {
				r.active = i
				r.selected = ed.ID
				r.touchMRU(ed.ID)
				r.rememberActiveRequest()
			}
			btn.ContextMenu(func(m *ui.Menu) {
				if m.Item("关闭").Chosen() {
					r.closeTabAt(i)
				}
				if m.Item("保存").Chosen() {
					r.saveEditor(ed)
				}
			})
		}
		ui.Spacer(c)
		ui.Text(c, fmt.Sprintf("%d 个标签", len(r.tabs))).FontSize(11).TextColor(t.TextMuted)
	})
}

func (r *Root) saveEditor(ed *Editor) {
	if ed == nil || !ed.Dirty {
		return
	}
	if err := ed.Save(); err != nil {
		r.err = err.Error()
		if r.win != nil {
			r.win.Update(func() {})
		}
		return
	}
	ed.Dirty = false
	r.reloadTree()
}

func envNames(st app.EnvState) []string {
	out := make([]string, 0, len(st.Environments))
	for _, e := range st.Environments {
		out = append(out, e.Name)
	}
	return out
}
