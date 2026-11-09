package ui

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (r *Root) openOpenAPIDialog() {
	r.openAPIOpen = true
	r.openAPIName = "新集合"
	r.openAPIURL = ""
	r.openAPIMode = 0
	r.openAPIEnvKey = ""
	if keys := r.envKeyOptions(); len(keys) > 0 {
		r.openAPIEnvKey = keys[0]
	}
	r.openAPIBusy = false
	r.openAPIErr = ""
}

func (r *Root) openAPIDialog(c *ui.Context) {
	if !r.openAPIOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &r.openAPIOpen, func() {
		ui.Text(c, "导入 OpenAPI").FontSize(15).Bold()
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "集合名称").Width(72).TextColor(t.TextMuted)
			ui.TextInput(c, &r.openAPIName).Grow(1)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "OpenAPI 地址").Width(72).TextColor(t.TextMuted)
			ui.TextInput(c, &r.openAPIURL).Placeholder("localhost:8080/v3/api-docs").Grow(1)
		})
		ui.Text(c, "请求 URL 的 root 来源").FontSize(12).TextColor(t.TextMuted)
		ui.Segmented(c, &r.openAPIMode, "主域名（servers / 源地址）", "环境变量").Label("root 来源")
		if r.openAPIMode == 1 {
			keys := r.envKeyOptions()
			if len(keys) == 0 {
				ui.Text(c, "当前没有环境变量，请先在环境管理器中添加。").FontSize(11.5).TextColor(t.Warning)
			} else {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "变量 key").Width(72).TextColor(t.TextMuted)
					ui.Select(c, &r.openAPIEnvKey, keys).Width(220)
					ui.Text(c, "生成 {{"+r.openAPIEnvKey+"}}/path").FontSize(11).TextColor(t.TextMuted)
				})
			}
		}
		ui.Text(c, "未填写协议时默认 http://；一个 OpenAPI 地址对应一个集合，tags 映射为文件夹。").
			FontSize(11).TextColor(t.TextMuted)
		if r.openAPIErr != "" {
			ui.Text(c, r.openAPIErr).FontSize(12).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Disabled(r.openAPIBusy).Clicked() {
				r.openAPIOpen = false
			}
			if ui.PrimaryButton(c, "创建").Disabled(r.openAPIBusy || strings.TrimSpace(r.openAPIURL) == "" || strings.TrimSpace(r.openAPIName) == "").Clicked() {
				r.startOpenAPIImport()
			}
		})
	}).Width(700).Height(480)
}

func (r *Root) openAPIRoot() *app.OpenAPIRoot {
	if r.openAPIMode == 1 {
		return &app.OpenAPIRoot{Mode: app.OpenAPIRootEnv, EnvKey: strings.TrimSpace(r.openAPIEnvKey)}
	}
	return &app.OpenAPIRoot{Mode: app.OpenAPIRootBaseURL}
}

func (r *Root) startOpenAPIImport() {
	name := strings.TrimSpace(r.openAPIName)
	if name == "" {
		name = "新集合"
	}
	source := app.NormalizeOpenAPIURL(r.openAPIURL)
	root := r.openAPIRoot()
	if root.Mode == app.OpenAPIRootEnv && root.EnvKey == "" {
		r.openAPIErr = "请选择环境变量 key"
		return
	}
	r.openAPIBusy = true
	r.openAPIErr = ""
	win := r.win
	if win == nil {
		r.openAPIBusy = false
		return
	}
	go func() {
		body, err := app.FetchOpenAPI(context.Background(), source)
		var id string
		var stats app.OpenAPIMergeStats
		if err == nil {
			id, stats, err = r.importOpenAPIText(string(body), source, name, root)
		}
		win.Update(func() {
			if err != nil {
				r.openAPIBusy = false
				r.openAPIErr = "导入失败: " + err.Error()
				return
			}
			r.openAPIBusy = false
			r.openAPIOpen = false
			r.expanded["c:"+id] = true
			r.reloadTree()
			r.selected = "c:" + id
			r.status = fmt.Sprintf("已导入 OpenAPI：%d 个接口 / %d 个文件夹", stats.Added, stats.Folders)
			r.err = ""
		})
	}()
}

// importOpenAPIText parses and stores a document without network access, for
// tests and the dialog goroutine.
func (r *Root) importOpenAPIText(text, sourceURL, name string, root *app.OpenAPIRoot) (string, app.OpenAPIMergeStats, error) {
	collectionID, err := r.store.CreateCollection(name)
	if err != nil {
		return "", app.OpenAPIMergeStats{}, err
	}
	parsed, err := app.ParseOpenAPI(text, sourceURL, name, collectionID, root)
	if err != nil {
		_ = r.store.DeleteCollection(collectionID)
		return "", app.OpenAPIMergeStats{}, err
	}
	stats, err := r.store.MergeOpenAPICollection(collectionID, parsed.Portable)
	if err != nil {
		_ = r.store.DeleteCollection(collectionID)
		return "", app.OpenAPIMergeStats{}, err
	}
	return collectionID, stats, nil
}

func (r *Root) refreshOpenAPICollection(collectionID string) (app.OpenAPIMergeStats, error) {
	return r.store.RefreshOpenAPICollection(context.Background(), collectionID)
}

func (r *Root) refreshOpenAPIAsync(collectionID string) {
	win := r.win
	if win == nil {
		if stats, err := r.refreshOpenAPICollection(collectionID); err == nil {
			r.afterOpenAPIRefresh(collectionID, stats, nil)
		} else {
			r.err = "OpenAPI 刷新失败: " + err.Error()
		}
		return
	}
	go func() {
		stats, err := r.refreshOpenAPICollection(collectionID)
		win.Update(func() { r.afterOpenAPIRefresh(collectionID, stats, err) })
	}()
}

func (r *Root) afterOpenAPIRefresh(collectionID string, stats app.OpenAPIMergeStats, err error) {
	if err != nil {
		r.err = "OpenAPI 刷新失败: " + err.Error()
		return
	}
	r.expanded["c:"+collectionID] = true
	r.reloadTree()
	r.selected = "c:" + collectionID
	r.status = fmt.Sprintf("OpenAPI 已刷新：新增 %d / 更新 %d 个接口", stats.Added, stats.Updated)
	r.err = ""
}
