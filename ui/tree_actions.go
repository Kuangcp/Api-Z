package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (r *Root) treeContextMenu(c *ui.Context, m *ui.Menu, row treeRow) {
	kind, id := r.nodeIdentity(row)
	switch kind {
	case app.NodeCollection:
		if m.Item("新建文件夹").Chosen() {
			r.createFolder(id, "")
		}
		if m.Item("新建请求").Chosen() {
			r.createRequest(id, "")
		}
		if m.Item("导出 Postman…").Chosen() {
			r.exportPostmanDialog(id, r.store.NodeName(app.NodeCollection, id))
		}
		if col, ok := r.tree.Collection(id); ok {
			if source, _ := app.CollectionOpenAPIInfo(col.MetaJSON); source != "" {
				if m.Item("刷新 OpenAPI").Chosen() {
					r.refreshOpenAPIAsync(id)
				}
			}
		}
		m.Separator()
	case app.NodeFolder:
		folder, _ := r.folderByID(id)
		if m.Item("新建子文件夹").Chosen() {
			r.createFolder(folder.CollectionID, folder.ID)
		}
		if m.Item("新建请求").Chosen() {
			r.createRequest(folder.CollectionID, folder.ID)
		}
		if m.Item("移动到文件夹…").Chosen() {
			r.beginMove(app.NodeFolder, id, folder.Name)
		}
		m.Separator()
	case app.NodeRequest:
		req, _ := r.tree.Request(id)
		if m.Item("打开").Chosen() {
			r.openRequest(id)
		}
		if m.Item("复制请求").Chosen() {
			r.duplicateRequest(id)
		}
		if m.Item("复制 URL").Chosen() {
			c.WriteClipboard(req.URL)
			c.Toast("已复制 URL")
		}
		if m.Item("移动到文件夹…").Chosen() {
			r.beginMove(app.NodeRequest, id, req.Name)
		}
		m.Separator()
	}
	if m.Item("重命名…").Chosen() {
		r.beginRename(kind, id, r.store.NodeName(kind, id))
	}
	if m.Item("上移").Chosen() {
		r.moveNode(kind, id, -1)
	}
	if m.Item("下移").Chosen() {
		r.moveNode(kind, id, +1)
	}
	m.Separator()
	if m.Item("删除…").Chosen() {
		r.beginDelete(kind, id, r.store.NodeName(kind, id))
	}
}

func (r *Root) nodeIdentity(row treeRow) (string, string) {
	switch row.kind {
	case rowCollection:
		return app.NodeCollection, strings.TrimPrefix(row.id, "c:")
	case rowFolder:
		return app.NodeFolder, strings.TrimPrefix(row.id, "f:")
	default:
		return app.NodeRequest, row.id
	}
}

func (r *Root) folderByID(id string) (app.Folder, bool) {
	for _, f := range r.tree.Folders {
		if f.ID == id {
			return f, true
		}
	}
	return app.Folder{}, false
}

func (r *Root) expandPath(collectionID, folderID string) {
	if collectionID != "" {
		r.expanded["c:"+collectionID] = true
	}
	if folderID != "" {
		r.expanded["f:"+folderID] = true
	}
}

func (r *Root) createCollection() {
	id, err := r.store.CreateCollection("新集合")
	if err != nil {
		r.err = "新建集合失败: " + err.Error()
		return
	}
	r.expanded["c:"+id] = true
	r.reloadTree()
	r.selected = "c:" + id
	r.beginRename(app.NodeCollection, id, "新集合")
}

func (r *Root) createFolder(collectionID, parentID string) {
	id, err := r.store.CreateFolder(collectionID, parentID, "新文件夹")
	if err != nil {
		r.err = "新建文件夹失败: " + err.Error()
		return
	}
	r.expandPath(collectionID, parentID)
	r.reloadTree()
	r.selected = "f:" + id
	r.beginRename(app.NodeFolder, id, "新文件夹")
}

func (r *Root) createRequest(collectionID, folderID string) {
	id, err := r.store.CreateRequest(collectionID, folderID, "新请求")
	if err != nil {
		r.err = "新建请求失败: " + err.Error()
		return
	}
	r.expandPath(collectionID, folderID)
	r.reloadTree()
	r.openRequest(id)
	r.beginRename(app.NodeRequest, id, "新请求")
}

func (r *Root) duplicateRequest(id string) {
	newID, err := r.store.DuplicateRequest(id)
	if err != nil {
		r.err = "复制请求失败: " + err.Error()
		return
	}
	if req, ok := r.tree.Request(id); ok {
		r.expandPath(req.CollectionID, req.FolderID)
	}
	r.reloadTree()
	r.openRequest(newID)
}

func (r *Root) moveNode(kind, id string, delta int) {
	if err := r.store.MoveSibling(kind, id, delta); err != nil {
		r.err = "移动失败: " + err.Error()
		return
	}
	r.reloadTree()
	switch kind {
	case app.NodeCollection:
		r.selected = "c:" + id
	case app.NodeFolder:
		r.selected = "f:" + id
	default:
		r.selected = id
	}
}

func (r *Root) beginRename(kind, id, name string) {
	r.dialog = dialogRename
	r.dialogOpen = true
	r.dlgKind = kind
	r.dlgID = id
	r.dlgName = name
	r.dlgErr = ""
}

func (r *Root) beginDelete(kind, id, name string) {
	r.dialog = dialogDelete
	r.dialogOpen = true
	r.dlgKind = kind
	r.dlgID = id
	r.dlgName = name
	r.dlgErr = ""
}

func (r *Root) finishDialog() {
	r.dialog = dialogNone
	r.dialogOpen = false
	r.dlgErr = ""
	r.dlgKind = ""
	r.dlgID = ""
	r.dlgName = ""
}

func (r *Root) applyRename() {
	name := strings.TrimSpace(r.dlgName)
	if name == "" {
		r.dlgErr = "名称不能为空"
		return
	}
	if err := r.store.RenameNode(r.dlgKind, r.dlgID, name); err != nil {
		r.dlgErr = "重命名失败: " + err.Error()
		return
	}
	if r.dlgKind == app.NodeRequest {
		for _, ed := range r.tabs {
			if ed.ID == r.dlgID {
				ed.Name = name
			}
		}
	}
	kind := r.dlgKind
	id := r.dlgID
	r.finishDialog()
	r.reloadTree()
	r.selected = selectionID(kind, id)
	r.status = "已重命名为「" + name + "」"
}

func (r *Root) applyDelete() {
	var err error
	switch r.dlgKind {
	case app.NodeCollection:
		err = r.store.DeleteCollection(r.dlgID)
	case app.NodeFolder:
		err = r.store.DeleteFolder(r.dlgID)
	case app.NodeRequest:
		err = r.store.DeleteRequest(r.dlgID)
	}
	if err != nil {
		r.dlgErr = "删除失败: " + err.Error()
		return
	}
	name := r.dlgName
	if r.dlgKind == app.NodeRequest {
		r.closeRequestTab(r.dlgID)
	}
	r.finishDialog()
	r.reloadTree()
	r.closeTabsForRemoved()
	r.selected = ""
	r.status = "已删除「" + name + "」"
}

func selectionID(kind, id string) string {
	switch kind {
	case app.NodeCollection:
		return "c:" + id
	case app.NodeFolder:
		return "f:" + id
	default:
		return id
	}
}

func (r *Root) closeRequestTab(id string) {
	for i, ed := range r.tabs {
		if ed.ID != id {
			continue
		}
		if sess := r.sessions[id]; sess != nil {
			sess.Cancel()
			delete(r.sessions, id)
		}
		r.removeMRU(id)
		r.tabs = append(r.tabs[:i], r.tabs[i+1:]...)
		if r.active >= len(r.tabs) {
			r.active = len(r.tabs) - 1
		}
		if r.active < 0 {
			r.active = 0
		}
		return
	}
}

// closeTabsForRemoved drops tabs whose request no longer exists after a
// cascade delete.
func (r *Root) closeTabsForRemoved() {
	exists := make(map[string]bool, len(r.tree.Requests))
	for _, req := range r.tree.Requests {
		exists[req.ID] = true
	}
	kept := make([]*Editor, 0, len(r.tabs))
	for _, ed := range r.tabs {
		if exists[ed.ID] {
			kept = append(kept, ed)
			continue
		}
		if sess := r.sessions[ed.ID]; sess != nil {
			sess.Cancel()
			delete(r.sessions, ed.ID)
		}
		r.removeMRU(ed.ID)
	}
	r.tabs = kept
	if r.active >= len(r.tabs) {
		r.active = len(r.tabs) - 1
	}
	if r.active < 0 {
		r.active = 0
	}
}

func (r *Root) readAndImportPostman(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	portable, err := app.ParsePostmanCollection(data)
	if err != nil {
		return "", "", err
	}
	id, err := r.store.ImportPortableCollection(portable)
	if err != nil {
		return "", "", err
	}
	return id, portable.Name, nil
}

func (r *Root) importPostmanFile(path string) (string, string, error) {
	id, name, err := r.readAndImportPostman(path)
	if err != nil {
		r.err = "导入失败: " + err.Error()
		return "", "", err
	}
	r.expanded["c:"+id] = true
	r.reloadTree()
	r.selected = "c:" + id
	r.status = "已导入集合「" + name + "」"
	r.err = ""
	return id, name, nil
}

func (r *Root) importPostmanDialog() {
	if r.win == nil {
		return
	}
	win := r.win
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  win,
			Title:   "导入 Postman Collection",
			Filters: []mygo.FileFilter{{Name: "Postman Collection", Extensions: []string{"json"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		path := paths[0]
		data, err := os.ReadFile(path)
		if err != nil {
			win.Update(func() { r.err = "导入失败: " + err.Error() })
			return
		}
		portable, err := app.ParsePostmanCollection(data)
		if err != nil {
			win.Update(func() { r.err = "导入失败: " + err.Error() })
			return
		}
		id, err := r.store.ImportPortableCollection(portable)
		win.Update(func() {
			if err != nil {
				r.err = "导入失败: " + err.Error()
				return
			}
			r.expanded["c:"+id] = true
			r.reloadTree()
			r.selected = "c:" + id
			r.status = "已导入集合「" + portable.Name + "」"
			r.err = ""
		})
	}()
}

func (r *Root) exportPostmanFile(collectionID, path string) error {
	portable, err := r.store.ExportPortableCollection(collectionID)
	if err != nil {
		return err
	}
	data, err := app.PostmanJSON(portable)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (r *Root) exportPostmanDialog(collectionID, name string) {
	if r.win == nil {
		return
	}
	win := r.win
	go func() {
		portable, err := r.store.ExportPortableCollection(collectionID)
		if err != nil {
			win.Update(func() { r.err = "导出失败: " + err.Error() })
			return
		}
		data, err := app.PostmanJSON(portable)
		if err != nil {
			win.Update(func() { r.err = "导出失败: " + err.Error() })
			return
		}
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      win,
			Title:       "导出 Postman Collection v2.1",
			DefaultPath: safeFileName(name) + ".postman_collection.json",
			Filters:     []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}},
		})
		if err != nil || path == "" {
			return
		}
		if !strings.HasSuffix(strings.ToLower(path), ".json") {
			path += ".json"
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			win.Update(func() { r.err = "导出失败: " + err.Error() })
			return
		}
		win.Update(func() {
			r.status = "已导出: " + path
			r.err = ""
		})
	}()
}

func safeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "collection"
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "collection"
	}
	return out
}

func (r *Root) dialogs(c *ui.Context) {
	if r.dialog == dialogNone {
		return
	}
	if !r.dialogOpen {
		r.finishDialog()
		return
	}
	t := c.Theme()
	switch r.dialog {
	case dialogRename:
		ui.Modal(c, &r.dialogOpen, func() {
			ui.Text(c, "重命名").FontSize(15).Bold()
			input := ui.TextInput(c, &r.dlgName).AutoFocus()
			if input.Submitted() {
				r.applyRename()
				return
			}
			if r.dlgErr != "" {
				ui.Text(c, r.dlgErr).FontSize(12).TextColor(t.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					r.finishDialog()
				}
				if ui.PrimaryButton(c, "应用").Clicked() {
					r.applyRename()
				}
			})
		}).Width(440)
	case dialogDelete:
		ui.Modal(c, &r.dialogOpen, func() {
			ui.Text(c, "删除确认").FontSize(15).Bold()
			ui.Text(c, fmt.Sprintf("确定删除「%s」？", r.dlgName)).FontSize(13)
			warn := "此操作不可撤销。"
			if r.dlgKind == app.NodeCollection {
				warn = "集合下的文件夹与请求会一起删除，此操作不可撤销。"
			} else if r.dlgKind == app.NodeFolder {
				warn = "文件夹下的子文件夹与请求会一起删除，此操作不可撤销。"
			} else if r.dlgKind == app.NodeRequest {
				warn = "请求的响应历史与本地 artifacts 会一起删除，此操作不可撤销。"
			}
			ui.Text(c, warn).FontSize(12).TextColor(t.TextMuted)
			if r.dlgErr != "" {
				ui.Text(c, r.dlgErr).FontSize(12).TextColor(t.Danger)
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "取消").Clicked() {
					r.finishDialog()
				}
				if ui.Button(c, "删除").TextColor(t.Danger).Clicked() {
					r.applyDelete()
				}
			})
		}).Width(460)
	}
	if !r.dialogOpen {
		r.finishDialog()
	}
}

func (r *Root) newRequestFromSelection() {
	sel := r.selected
	switch {
	case strings.HasPrefix(sel, "c:"):
		r.createRequest(strings.TrimPrefix(sel, "c:"), "")
	case strings.HasPrefix(sel, "f:"):
		if f, ok := r.folderByID(strings.TrimPrefix(sel, "f:")); ok {
			r.createRequest(f.CollectionID, f.ID)
		}
	case sel != "":
		if req, ok := r.tree.Request(sel); ok {
			r.createRequest(req.CollectionID, req.FolderID)
		}
	default:
		if len(r.tree.Collections) > 0 {
			r.createRequest(r.tree.Collections[0].ID, "")
		} else {
			r.createCollection()
		}
	}
}

func (r *Root) closeTabAt(i int) {
	if i < 0 || i >= len(r.tabs) {
		return
	}
	ed := r.tabs[i]
	r.saveEditor(ed)
	if sess := r.sessions[ed.ID]; sess != nil {
		sess.Cancel()
		delete(r.sessions, ed.ID)
	}
	r.removeMRU(ed.ID)
	r.tabs = append(r.tabs[:i], r.tabs[i+1:]...)
	if r.active >= len(r.tabs) {
		r.active = len(r.tabs) - 1
	}
	if r.active < 0 {
		r.active = 0
	}
}
