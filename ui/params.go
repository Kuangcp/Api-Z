package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (ed *Editor) paramsSource() string {
	return ed.URL + "\x00" + ed.Params
}

// migrateParamsToURL folds legacy params_text into the URL so the address bar
// always carries the complete request URL. Disabled rows stay in params_text
// because they must not be sent.
func (ed *Editor) migrateParamsToURL() {
	if strings.TrimSpace(ed.Params) == "" {
		return
	}
	legacy, raw := parseKV(ed.Params)
	var enabled []app.Header
	var disabled []kvRow
	for _, row := range legacy {
		if row.Enabled {
			enabled = append(enabled, app.Header{Name: strings.TrimSpace(row.Key), Value: row.Value})
		} else {
			disabled = append(disabled, row)
		}
	}
	oldURL, oldParams := ed.URL, ed.Params
	if len(enabled) > 0 {
		params := app.ParseURLQuery(ed.URL)
		params = append(params, enabled...)
		ed.URL = app.SetURLQuery(ed.URL, params)
	}
	ed.Params = formatDisabledParams(disabled, raw)
	if ed.URL != oldURL || ed.Params != oldParams {
		ed.Dirty = true
	}
	ed.paramsKV.Source = ""
}

func (ed *Editor) syncParams() {
	key := ed.paramsSource()
	if ed.paramsKV.Source == key {
		return
	}
	rows := make([]kvRow, 0, len(ed.paramsKV.Rows))
	for _, h := range app.ParseURLQuery(ed.URL) {
		rows = append(rows, kvRow{Enabled: true, Key: h.Name, Value: h.Value})
	}
	legacy, raw := parseKV(ed.Params)
	rows = append(rows, legacy...)
	ed.paramsKV.Rows = rows
	ed.paramsKV.Raw = raw
	ed.paramsKV.Source = key
}

func (ed *Editor) commitParams() {
	var enabled []app.Header
	var disabled []kvRow
	for _, row := range ed.paramsKV.Rows {
		key := strings.TrimSpace(row.Key)
		if key == "" {
			continue
		}
		if row.Enabled {
			enabled = append(enabled, app.Header{Name: key, Value: row.Value})
		} else {
			disabled = append(disabled, row)
		}
	}
	ed.URL = app.SetURLQuery(ed.URL, enabled)
	ed.Params = formatDisabledParams(disabled, ed.paramsKV.Raw)
	ed.paramsKV.Source = ed.paramsSource()
	ed.Dirty = true
}

func formatDisabledParams(rows []kvRow, raw []string) string {
	lines := make([]string, 0, len(rows)+len(raw))
	for _, row := range rows {
		key := strings.TrimSpace(row.Key)
		if key == "" {
			continue
		}
		lines = append(lines, "! "+key+": "+strings.TrimSpace(row.Value))
	}
	for _, line := range raw {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

// paramsEditor renders URL query parameters as a table. Edits rewrite the
// URL; disabled rows are kept in params_text and never sent.
func (ed *Editor) paramsEditor(c *ui.Context) {
	t := c.Theme()
	ed.syncParams()
	ui.Text(c, "与地址栏 URL 双向同步；禁用行只保存在 Params，不发送。地址栏可直接编辑完整原始 URL。").
		FontSize(10.5).TextColor(t.TextMuted).Padding(2, 2)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "启用").FontSize(10.5).TextColor(t.TextMuted).Width(34)
		ui.Text(c, "Key").FontSize(10.5).TextColor(t.TextMuted).Width(170)
		ui.Text(c, "Value").FontSize(10.5).TextColor(t.TextMuted).Grow(1)
		ui.Box(c).Width(28)
	})
	deleteAt := -1
	ui.Scroll(c).Grow(1).Children(func() {
		for i := range ed.paramsKV.Rows {
			i := i
			row := &ed.paramsKV.Rows[i]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				enabled := ui.Checkbox(c, &row.Enabled, "").Label("启用该行")
				key := ui.TextInput(c, &row.Key).Placeholder("Key").Width(170)
				value := ui.TextInput(c, &row.Value).Placeholder("Value").Grow(1)
				del := iconButtonPlain(c, iconX, "删除该行")
				if enabled.Changed() || key.Changed() || value.Changed() {
					ed.commitParams()
				}
				if del.Clicked() {
					deleteAt = i
				}
			})
		}
		if len(ed.paramsKV.Rows) == 0 {
			ui.Text(c, "URL 中没有查询参数").FontSize(12).TextColor(t.TextMuted).Padding(8)
		}
		ui.Row(c).Children(func() {
			if ui.Button(c, "添加一行").Clicked() {
				ed.paramsKV.Rows = append(ed.paramsKV.Rows, kvRow{Enabled: true})
			}
		})
	})
	if deleteAt >= 0 && deleteAt < len(ed.paramsKV.Rows) {
		ed.paramsKV.Rows = append(ed.paramsKV.Rows[:deleteAt], ed.paramsKV.Rows[deleteAt+1:]...)
		ed.commitParams()
	}
}
