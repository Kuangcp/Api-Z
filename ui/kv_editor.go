package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

type kvRow struct {
	Enabled bool
	Key     string
	Value   string
}

// kvState keeps the form view of a line-based key/value text (Params,
// Headers). Text stays the source of truth in the request; the rows are
// rebuilt whenever the text changes outside the form.
type kvState struct {
	Mode   int // 0 text, 1 form
	Rows   []kvRow
	Raw    []string
	Source string
}

func parseKV(text string) ([]kvRow, []string) {
	var rows []kvRow
	var raw []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		body := strings.TrimSpace(line)
		enabled := true
		if strings.HasPrefix(body, "! ") {
			enabled = false
			body = strings.TrimSpace(body[2:])
		}
		key, value, ok := strings.Cut(body, ":")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			raw = append(raw, line)
			continue
		}
		rows = append(rows, kvRow{Enabled: enabled, Key: key, Value: strings.TrimSpace(value)})
	}
	return rows, raw
}

func (kv *kvState) commit(dst *string) {
	lines := make([]string, 0, len(kv.Rows)+len(kv.Raw))
	for _, row := range kv.Rows {
		key := strings.TrimSpace(row.Key)
		if key == "" {
			continue
		}
		line := key + ": " + strings.TrimSpace(row.Value)
		if !row.Enabled {
			line = "! " + line
		}
		lines = append(lines, line)
	}
	for _, line := range kv.Raw {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	*dst = strings.Join(lines, "\n")
	kv.Source = *dst
}

// kvEditor renders the text/form toggle and the active editor. field names
// the autocomplete slot ("params"/"headers").
func (r *Root) kvEditor(c *ui.Context, ed *Editor, field, hint string, text *string, kv *kvState) {
	t := c.Theme()
	if kv.Source != *text {
		kv.Rows, kv.Raw = parseKV(*text)
		kv.Source = *text
	}
	mode := kv.Mode
	if ui.Segmented(c, &kv.Mode, "文本", "表单").Label("编辑模式").Changed() {
		kv.Source = ""
		if kv.Mode == 1 {
			kv.Rows, kv.Raw = parseKV(*text)
			kv.Source = *text
		}
	}
	if mode != kv.Mode && kv.Mode == 0 {
		// Text mode reads *text directly; drop the form snapshot next frame.
		kv.Source = ""
	}

	if kv.Mode == 0 {
		area := ui.TextArea(c, text).Grow(1).Font("monospace")
		changed := area.Changed()
		if changed {
			ed.Dirty = true
			kv.Source = ""
		}
		r.varAutocomplete(c, ed, field, area, text, changed)
		return
	}

	ui.Text(c, hint).FontSize(10.5).TextColor(t.TextMuted).Padding(2, 2)
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "启用").FontSize(10.5).TextColor(t.TextMuted).Width(34)
		ui.Text(c, "Key").FontSize(10.5).TextColor(t.TextMuted).Width(170)
		ui.Text(c, "Value").FontSize(10.5).TextColor(t.TextMuted).Grow(1)
		ui.Box(c).Width(28)
	})
	deleteAt := -1
	ui.Scroll(c).Grow(1).Children(func() {
		for i := range kv.Rows {
			i := i
			row := &kv.Rows[i]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				enabled := ui.Checkbox(c, &row.Enabled, "").Label("启用该行")
				key := ui.TextInput(c, &row.Key).Placeholder("Key").Width(170)
				value := ui.TextInput(c, &row.Value).Placeholder("Value").Grow(1)
				del := iconButtonPlain(c, iconX, "删除该行")
				if enabled.Changed() || key.Changed() || value.Changed() {
					kv.commit(text)
					ed.Dirty = true
				}
				if del.Clicked() {
					deleteAt = i
				}
			})
		}
		if len(kv.Rows) == 0 {
			ui.Text(c, "还没有条目").FontSize(12).TextColor(t.TextMuted).Padding(8)
		}
		ui.Row(c).Children(func() {
			if ui.Button(c, "添加一行").Clicked() {
				kv.Rows = append(kv.Rows, kvRow{Enabled: true})
			}
		})
	})
	if deleteAt >= 0 && deleteAt < len(kv.Rows) {
		kv.Rows = append(kv.Rows[:deleteAt], kv.Rows[deleteAt+1:]...)
		kv.commit(text)
		ed.Dirty = true
	}
}
