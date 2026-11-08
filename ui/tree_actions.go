package ui

import (
	"fmt"
	"strings"

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
		m.Separator()
	case app.NodeFolder:
		folder, _ := r.folderByID(id)
		if m.Item("新建子文件夹").Chosen() {
			r.createFolder(folder.CollectionID, folder.ID)
		}
		if m.Item("新建请求").Chosen() {
			r.createRequest(folder.CollectionID, folder.ID)
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
