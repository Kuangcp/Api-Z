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
	parsed, err := app.ParseCurl(text)
	if err != nil {
		return "", err
	}
	collectionID, folderID, _, ok := r.curlTarget()
	if !ok {
		return "", fmt.Errorf("请先选择集合或文件夹")
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
