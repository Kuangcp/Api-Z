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

	tabs   []*Editor
	active int

	sessions map[string]*app.Session

	env      app.EnvState
	envName  string
	envNames []string

	sidebarWidth  float32
	editorPercent float32
	status        string
	err           string
	migratedFrom  string

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
}

func NewRoot(paths app.Paths, store *app.Store, tree app.Tree, env app.EnvState) *Root {
	r := &Root{
		paths:         paths,
		store:         store,
		client:        httpclient.NewClient(),
		tree:          tree,
		expanded:      map[string]bool{},
		sessions:      map[string]*app.Session{},
		env:           env.Normalize(),
		sidebarWidth:  300,
		editorPercent: 52,
		migratedFrom:  store.Meta("migrated_from"),
	}
	for _, c := range tree.Collections {
		r.expanded["c:"+c.ID] = true
	}
	if a := env.Active(); a != nil {
		r.envName = a.Name
	}
	r.envNames = envNames(env)
	r.status = fmt.Sprintf("%d 个集合 · %d 个请求", len(tree.Collections), len(tree.Requests))
	return r
}

func (r *Root) Attach(win *mygo.Window) { r.win = win }

func (r *Root) View(c *ui.Context) {
	t := c.Theme()
	if c.Shortcut(ui.Ctrl, ui.KeyK) {
		r.searchOpen = true
		r.globalQuery = ""
	}
	if c.Shortcut(ui.Ctrl, ui.KeyTab) && !r.searchOpen && !r.dialogOpen {
		r.switchRecent()
	}
	ui.Column(c).Fill().Children(func() {
		r.topBar(c)
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
			if src := r.migratedFrom; src != "" {
				ui.Text(c, "数据迁移自 api-x").FontSize(11.5).TextColor(t.TextMuted).
					Tooltip(src)
			}
			ui.Text(c, r.paths.DataDir).FontSize(11.5).TextColor(t.TextMuted).SingleLine()
		})
	})
	r.dialogs(c)
	r.globalSearch(c)
}

func (r *Root) topBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(6, 12).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, "Api-Z").FontSize(15).Bold()
		ui.Text(c, "Go + mygo").FontSize(11).TextColor(t.TextMuted)
		ui.Box(c).Grow(1)
		ui.Text(c, "环境").FontSize(12).TextColor(t.TextMuted)
		names := r.envNames
		if len(names) == 0 {
			names = []string{"（无环境）"}
		}
		before := r.envName
		ui.Select(c, &r.envName, names).Label("环境").Width(220)
		if r.envName != before {
			r.activateEnv(r.envName)
		}
	})
}

func (r *Root) activateEnv(name string) {
	for _, e := range r.env.Environments {
		if e.Name == name {
			r.env.ActiveEnvironmentID = e.ID
			_ = r.store.SaveEnvironments(r.env)
			return
		}
	}
}

func (r *Root) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(6).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.TextInput(c, &r.search).Placeholder("搜索请求 / URL").Grow(1)
			ui.Button(c, "清空").Disabled(r.search == "").OnClick(func() { r.search = "" })
		})
		ui.Row(c).Gap(6).Children(func() {
			if ui.Button(c, "新建集合").Clicked() {
				r.createCollection()
			}
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
				ui.Text(c, "没有匹配的请求").Padding(12).TextColor(t.TextMuted)
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
			if row.open {
				ui.Text(c, "▾").FontSize(10).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "▸").FontSize(10).TextColor(t.TextMuted)
			}
			ui.Text(c, row.name).FontSize(12.5).Bold().SingleLine().Grow(1)
		case rowFolder:
			if row.open {
				ui.Text(c, "▾").FontSize(10).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "▸").FontSize(10).TextColor(t.TextMuted)
			}
			ui.Text(c, row.name).FontSize(12.5).SingleLine().Grow(1)
		case rowRequest:
			ui.Text(c, " ").FontSize(10)
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
	btn.ContextMenu(func(m *ui.Menu) {
		r.treeContextMenu(c, m, row)
	})
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
	var rows []treeRow
	collections := append([]app.Collection(nil), r.tree.Collections...)
	sort.SliceStable(collections, func(i, j int) bool { return collections[i].SortOrder < collections[j].SortOrder })
	for _, col := range collections {
		colRows, matched := r.collectionRows(col, 0, search)
		if search != "" && !matched {
			continue
		}
		rows = append(rows, colRows...)
	}
	return rows
}

func (r *Root) collectionRows(col app.Collection, depth int, search string) ([]treeRow, bool) {
	key := "c:" + col.ID
	row := treeRow{kind: rowCollection, depth: depth, id: key, name: col.Name, open: r.expanded[key]}
	rows := []treeRow{row}
	matched := matches(search, col.Name, "")
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
}

func (r *Root) openRequest(id string) {
	for i, ed := range r.tabs {
		if ed.ID == id {
			r.active = i
			r.selected = id
			r.touchMRU(id)
			return
		}
	}
	req, ok, err := r.store.GetRequest(id)
	if err != nil || !ok {
		r.err = fmt.Sprintf("读取请求失败: %v", err)
		return
	}
	ed := NewEditor(r, req)
	r.tabs = append(r.tabs, ed)
	r.active = len(r.tabs) - 1
	r.selected = id
	r.touchMRU(id)
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
		ui.SplitVerticalPercent(c, &r.editorPercent, func() {
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
			}
			btn.ContextMenu(func(m *ui.Menu) {
				if m.Item("关闭").Chosen() {
					r.saveEditor(ed)
					r.tabs = append(r.tabs[:i], r.tabs[i+1:]...)
					if r.active >= len(r.tabs) {
						r.active = len(r.tabs) - 1
					}
					if r.active < 0 {
						r.active = 0
					}
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
