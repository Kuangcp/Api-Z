package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

const nodeSettingsLabelWidth float32 = 140

// openNodeSettings starts editing a collection or folder's auth and, for a
// collection, its OpenAPI binding.
func (r *Root) openNodeSettings(kind, id string) {
	var meta string
	switch kind {
	case app.NodeCollection:
		col, ok := r.tree.Collection(id)
		if !ok {
			return
		}
		meta = col.MetaJSON
	case app.NodeFolder:
		folder, ok := r.folderByID(id)
		if !ok {
			return
		}
		meta = folder.MetaJSON
	default:
		return
	}
	r.nodeSettingsOpen = true
	r.nodeSettingsKind = kind
	r.nodeSettingsID = id
	r.nodeSettingsSection = 0
	r.nodeSettingsAuth = app.ParseAuth(meta)
	r.nodeSettingsErr = ""
	r.nodeSettingsSource = ""
	r.nodeSettingsRootMode = 0
	r.nodeSettingsEnvKey = ""
	if kind == app.NodeCollection {
		source, root := app.CollectionOpenAPIInfo(meta)
		r.nodeSettingsSource = source
		if root != nil && root.Mode == app.OpenAPIRootEnv {
			r.nodeSettingsRootMode = 1
			r.nodeSettingsEnvKey = root.EnvKey
		}
	}
	if r.nodeSettingsRootMode == 1 && r.nodeSettingsEnvKey == "" {
		if keys := r.envKeyOptions(); len(keys) > 0 {
			r.nodeSettingsEnvKey = keys[0]
		}
	}
}

func (r *Root) nodeSettingsModal(c *ui.Context) {
	if !r.nodeSettingsOpen {
		return
	}
	t := c.Theme()
	title := "集合设置"
	isCollection := r.nodeSettingsKind == app.NodeCollection
	if !isCollection {
		title = "文件夹设置"
	}
	ui.Modal(c, &r.nodeSettingsOpen, func() {
		ui.Text(c, title).FontSize(15).Bold()
		labels := []string{"认证 (Auth)"}
		if isCollection {
			labels = append(labels, "OpenAPI")
		}
		if r.nodeSettingsSection >= len(labels) {
			r.nodeSettingsSection = 0
		}
		ui.Tabs(c, &r.nodeSettingsSection, labels...)
		ui.Column(c).Grow(1).Padding(4).Children(func() {
			if r.nodeSettingsSection == 0 {
				r.nodeAuthEditor(c, t)
			} else if isCollection {
				r.nodeOpenAPIEditor(c, t)
			}
		})
		if r.nodeSettingsErr != "" {
			ui.Text(c, r.nodeSettingsErr).FontSize(12).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				r.nodeSettingsOpen = false
			}
			if ui.PrimaryButton(c, "保存").Clicked() {
				r.saveNodeSettings()
			}
		})
	}).Width(760).Height(520)
}

func (r *Root) nodeAuthEditor(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Gap(8).Children(func() {
		types := []string{"inherit", "noauth", "bearer", "basic", "apikey"}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "认证类型").Width(nodeSettingsLabelWidth).TextColor(t.TextMuted)
			if ui.Select(c, &r.nodeSettingsAuth.Type, types).Label("认证类型").Width(220).Changed() {
				r.nodeSettingsAuth = normalizeAuth(r.nodeSettingsAuth)
			}
		})
		switch r.nodeSettingsAuth.Type {
		case "inherit":
			ui.Text(c, "继承父级认证；集合根节点没有父级时不会附加认证。").
				FontSize(12).TextColor(t.TextMuted)
			if r.nodeSettingsKind == app.NodeFolder {
				if f, ok := r.folderByID(r.nodeSettingsID); ok {
					ui.Text(c, "父级当前生效："+authTypeLabel(r.parentAuthForFolder(f).Type)).
						FontSize(11.5).TextColor(t.TextMuted)
				}
			}
		case "noauth":
			ui.Text(c, "不附加认证，并阻止子节点继续继承。").
				FontSize(12).TextColor(t.TextMuted)
		case "bearer":
			authFieldInput(c, "Token", "token", &r.nodeSettingsAuth.Bearer, true)
			ui.Text(c, "发送时附加 Authorization: Bearer <token>。").FontSize(11).TextColor(t.TextMuted)
		case "basic":
			authFieldInput(c, "用户名", "username", &r.nodeSettingsAuth.Basic, false)
			authFieldInput(c, "密码", "password", &r.nodeSettingsAuth.Basic, true)
			ui.Text(c, "发送时附加 Authorization: Basic base64(username:password)。").
				FontSize(11).TextColor(t.TextMuted)
		case "apikey":
			authFieldInput(c, "Key", "key", &r.nodeSettingsAuth.APIKey, false)
			authFieldInput(c, "Value", "value", &r.nodeSettingsAuth.APIKey, true)
			in := authValue(r.nodeSettingsAuth.APIKey, "in")
			if in == "" {
				in = "header"
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "添加到").Width(nodeSettingsLabelWidth).TextColor(t.TextMuted)
				if ui.Select(c, &in, []string{"header", "query"}).Label("API Key 位置").Width(160).Changed() {
					setAuthValue(&r.nodeSettingsAuth.APIKey, "in", in)
				}
			})
		}
	})
}

func (r *Root) parentAuthForFolder(f app.Folder) app.Auth {
	parent := f.ParentID
	for parent != "" {
		for _, candidate := range r.tree.Folders {
			if candidate.ID != parent {
				continue
			}
			if a := app.ParseAuth(candidate.MetaJSON); a.Type != "inherit" {
				return a
			}
			parent = candidate.ParentID
			break
		}
	}
	if col, ok := r.tree.Collection(f.CollectionID); ok {
		if a := app.ParseAuth(col.MetaJSON); a.Type != "inherit" {
			return a
		}
	}
	return app.Auth{Type: "noauth"}
}

func (r *Root) nodeOpenAPIEditor(c *ui.Context, t *ui.Theme) {
	keys := r.envKeyOptions()
	ui.Column(c).Gap(8).Children(func() {
		ui.Text(c, "OpenAPI 地址").FontSize(12).TextColor(t.TextMuted)
		ui.TextInput(c, &r.nodeSettingsSource).
			Placeholder("http://localhost:8080/v3/api-docs").Grow(1)
		ui.Text(c, "留空表示解绑；绑定后可在树右键「刷新 OpenAPI」。未写协议默认 http://。").
			FontSize(11).TextColor(t.TextMuted)
		ui.Text(c, "请求 URL 的 root 来源").FontSize(12).TextColor(t.TextMuted)
		ui.Segmented(c, &r.nodeSettingsRootMode, "主域名（servers / 源地址）", "环境变量").
			Label("root 来源")
		if r.nodeSettingsRootMode == 1 {
			if len(keys) == 0 {
				ui.Text(c, "当前没有环境变量，请先在环境管理器中添加。").
					FontSize(11.5).TextColor(t.Warning)
			} else {
				if r.nodeSettingsEnvKey == "" || !containsString(keys, r.nodeSettingsEnvKey) {
					r.nodeSettingsEnvKey = keys[0]
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "变量 key").Width(80).TextColor(t.TextMuted)
					ui.Select(c, &r.nodeSettingsEnvKey, keys).Width(220)
					ui.Text(c, "生成 {{"+r.nodeSettingsEnvKey+"}}/path").
						FontSize(11).TextColor(t.TextMuted)
				})
			}
		}
	})
}

func (r *Root) saveNodeSettings() {
	var meta string
	switch r.nodeSettingsKind {
	case app.NodeCollection:
		col, ok := r.tree.Collection(r.nodeSettingsID)
		if !ok {
			r.nodeSettingsErr = "集合不存在"
			return
		}
		meta = col.MetaJSON
	case app.NodeFolder:
		folder, ok := r.folderByID(r.nodeSettingsID)
		if !ok {
			r.nodeSettingsErr = "文件夹不存在"
			return
		}
		meta = folder.MetaJSON
	}
	r.nodeSettingsAuth = normalizeAuth(r.nodeSettingsAuth)
	meta = app.MergeAuth(meta, r.nodeSettingsAuth)
	if r.nodeSettingsKind == app.NodeCollection {
		source := strings.TrimSpace(r.nodeSettingsSource)
		var root *app.OpenAPIRoot
		if r.nodeSettingsRootMode == 1 {
			if source != "" && strings.TrimSpace(r.nodeSettingsEnvKey) == "" {
				r.nodeSettingsErr = "请选择环境变量 key"
				return
			}
			root = &app.OpenAPIRoot{Mode: app.OpenAPIRootEnv, EnvKey: strings.TrimSpace(r.nodeSettingsEnvKey)}
		} else if source != "" {
			root = &app.OpenAPIRoot{Mode: app.OpenAPIRootBaseURL}
		}
		meta = app.SetCollectionOpenAPI(meta, source, root)
	}
	if err := r.store.UpdateNodeMeta(r.nodeSettingsKind, r.nodeSettingsID, meta); err != nil {
		r.nodeSettingsErr = "保存失败: " + err.Error()
		return
	}
	kind, id := r.nodeSettingsKind, r.nodeSettingsID
	r.nodeSettingsOpen = false
	r.reloadTree()
	r.selected = selectionID(kind, id)
	r.status = "设置已保存"
	r.err = ""
}

func normalizeAuth(a app.Auth) app.Auth {
	if a.Type == "" {
		a.Type = "inherit"
	}
	if a.Type == "apikey" {
		if authValue(a.APIKey, "in") == "" {
			setAuthValue(&a.APIKey, "in", "header")
		}
	}
	return a
}

func authFieldInput(c *ui.Context, label, key string, list *[]app.AuthProperty, secret bool) {
	t := c.Theme()
	val := authValue(*list, key)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(nodeSettingsLabelWidth).TextColor(t.TextMuted)
		input := ui.TextInput(c, &val).Grow(1)
		if secret {
			input = input.Password()
		}
		if input.Changed() {
			setAuthValue(list, key, val)
		}
	})
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
