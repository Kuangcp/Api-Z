package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (r *Root) openCurlDialog(c *ui.Context) {
	r.curlOpen = true
	r.curlErr = ""
	if strings.TrimSpace(r.curlText) == "" {
		r.curlText = strings.TrimSpace(c.ReadClipboard())
	}
}

// importCurlFromClipboard creates a request from the latest clipboard cURL
// command, under the selected collection/folder or the active collection root.
func (r *Root) importCurlFromClipboard(c *ui.Context) {
	text := strings.TrimSpace(c.ReadClipboard())
	if text == "" {
		c.Toast("剪贴板为空")
		return
	}
	collectionID, folderID, afterID, label, ok := r.selectedCurlTarget()
	if !ok {
		c.Toast("请先创建集合")
		return
	}
	id, err := r.createCurlRequestAt(collectionID, folderID, afterID, text)
	if err != nil {
		c.Toast("导入 cURL 失败: " + err.Error())
		r.err = "导入 cURL 失败: " + err.Error()
		return
	}
	r.openRequest(id)
	r.status = "已从剪贴板创建请求 → " + label
	r.err = ""
	c.Toast("已从剪贴板创建请求")
}

// selectedCurlTarget honors a selected request by inserting the new
// request right below it; otherwise it uses a selected collection/folder or
// the active collection root.
func (r *Root) selectedCurlTarget() (collectionID, folderID, afterID, label string, ok bool) {
	if req, found := r.tree.Request(r.selected); found {
		label = req.Name + " 下方"
		return req.CollectionID, req.FolderID, req.ID, label, true
	}
	collectionID, folderID, label, ok = r.selectedDirectoryCurlTarget()
	return collectionID, folderID, "", label, ok
}

func (r *Root) createCurlRequestAt(collectionID, folderID, afterID, text string) (string, error) {
	if afterID != "" {
		beforeID := r.nextSiblingID(app.NodeRequest, collectionID, folderID, afterID)
		id, err := r.createCurlRequest(collectionID, folderID, text)
		if err != nil {
			return "", err
		}
		if err := r.store.MoveNodeBefore(app.NodeRequest, id, collectionID, folderID, beforeID); err != nil {
			return "", err
		}
		r.reloadTree()
		r.selected = id
		return id, nil
	}
	return r.createCurlRequest(collectionID, folderID, text)
}

// importCurlAt creates a request under an explicit collection or folder,
// used by the tree context menu.
func (r *Root) importCurlAt(c *ui.Context, kind, id string) {
	text := strings.TrimSpace(c.ReadClipboard())
	if text == "" {
		c.Toast("剪贴板为空")
		return
	}
	collectionID, folderID, label, ok := r.curlTargetForNode(kind, id)
	if !ok {
		c.Toast("目标目录不存在")
		return
	}
	requestID, err := r.createCurlRequest(collectionID, folderID, text)
	if err != nil {
		c.Toast("解析 cURL 失败: " + err.Error())
		r.err = "解析 cURL 失败: " + err.Error()
		return
	}
	r.openRequest(requestID)
	r.status = "已从剪贴板创建请求 → " + label
	r.err = ""
	c.Toast("已从剪贴板创建请求")
}

func (r *Root) createCurlRequest(collectionID, folderID, text string) (string, error) {
	parsed, err := app.ParseCurl(text)
	if err != nil {
		return "", err
	}
	name := app.CurlRequestName(parsed)
	id, err := r.store.CreateRequest(collectionID, folderID, name)
	if err != nil {
		return "", err
	}
	req, ok, err := r.store.GetRequest(id)
	if err != nil || !ok {
		return "", fmt.Errorf("创建请求失败")
	}
	req.Name = name
	req.Method = parsed.Method
	req.URL = parsed.URL
	req.HeadersText = parsed.HeadersText
	req.ParamsText = parsed.ParamsText
	req.BodyText = parsed.BodyText
	if err := r.store.SaveRequest(req); err != nil {
		return "", err
	}
	r.expandPath(collectionID, folderID)
	r.reloadTree()
	r.selected = id
	return id, nil
}

// selectedDirectoryCurlTarget only accepts a selected collection or folder.
// A selected request (or nothing) falls back to the active collection root.
func (r *Root) selectedDirectoryCurlTarget() (collectionID, folderID, label string, ok bool) {
	sel := r.selected
	switch {
	case strings.HasPrefix(sel, "c:"):
		id := strings.TrimPrefix(sel, "c:")
		if col, found := r.tree.Collection(id); found {
			return id, "", col.Name + "（根）", true
		}
	case strings.HasPrefix(sel, "f:"):
		if f, found := r.folderByID(strings.TrimPrefix(sel, "f:")); found {
			return f.CollectionID, f.ID, f.Name, true
		}
	}
	if col, found := r.tree.Collection(r.activeCollection); found {
		return col.ID, "", col.Name + "（根）", true
	}
	if len(r.tree.Collections) > 0 {
		col := r.tree.Collections[0]
		return col.ID, "", col.Name + "（根）", true
	}
	return "", "", "请先创建一个集合", false
}

func (r *Root) curlTargetForNode(kind, id string) (collectionID, folderID, label string, ok bool) {
	switch kind {
	case app.NodeCollection:
		if col, found := r.tree.Collection(id); found {
			return id, "", col.Name + "（根）", true
		}
	case app.NodeFolder:
		if f, found := r.folderByID(id); found {
			return f.CollectionID, f.ID, f.Name, true
		}
	}
	return "", "", "", false
}

func (r *Root) curlTarget() (collectionID, folderID, label string, ok bool) {
	sel := r.selected
	switch {
	case strings.HasPrefix(sel, "c:"):
		id := strings.TrimPrefix(sel, "c:")
		if col, found := r.tree.Collection(id); found {
			return id, "", col.Name + "（根）", true
		}
	case strings.HasPrefix(sel, "f:"):
		if f, found := r.folderByID(strings.TrimPrefix(sel, "f:")); found {
			return f.CollectionID, f.ID, f.Name, true
		}
	case sel != "":
		if req, found := r.tree.Request(sel); found {
			label := "请求根"
			if f, ok := r.folderByID(req.FolderID); ok {
				label = f.Name
			}
			return req.CollectionID, req.FolderID, label, true
		}
	}
	if col, found := r.tree.Collection(r.activeCollection); found {
		return col.ID, "", col.Name + "（根）", true
	}
	if len(r.tree.Collections) > 0 {
		col := r.tree.Collections[0]
		return col.ID, "", col.Name + "（根）", true
	}
	return "", "", "请先创建一个集合", false
}

func (r *Root) curlDialog(c *ui.Context) {
	if !r.curlOpen {
		return
	}
	t := c.Theme()
	_, _, label, ok := r.curlTarget()
	ui.Modal(c, &r.curlOpen, func() {
		ui.Text(c, "导入 cURL").FontSize(15).Bold()
		ui.Text(c, "粘贴 sh 风格 curl 命令；将创建到："+label).FontSize(11.5).TextColor(t.TextMuted)
		area := ui.TextArea(c, &r.curlText).Height(200).Font("monospace")
		_ = area
		if r.curlErr != "" {
			ui.Text(c, r.curlErr).FontSize(12).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "从剪贴板").Clicked() {
				r.curlText = strings.TrimSpace(c.ReadClipboard())
				r.curlErr = ""
			}
			if ui.Button(c, "取消").Clicked() {
				r.curlOpen = false
			}
			if ui.PrimaryButton(c, "创建").Disabled(!ok || strings.TrimSpace(r.curlText) == "").Clicked() {
				id, err := r.importCurlText(r.curlText)
				if err != nil {
					r.curlErr = "导入失败: " + err.Error()
					return
				}
				r.curlOpen = false
				r.openRequest(id)
				r.status = "已从 cURL 创建请求"
			}
		})
	}).Width(760).Height(440)
}

func (r *Root) importCurlText(text string) (string, error) {
	collectionID, folderID, _, ok := r.curlTarget()
	if !ok {
		return "", fmt.Errorf("请先选择集合或文件夹")
	}
	return r.createCurlRequest(collectionID, folderID, text)
}
