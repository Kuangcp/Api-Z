package ui

import (
	"net/url"
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
	Mode   int // 0 text, 1 table
	Rows   []kvRow
	Raw    []string
	Source string
	Form   bool // body table: URL-encoded key=value instead of Name: Value lines
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

func looksLikeFormLines(body string) bool {
	found := false
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "! "))
		key, _, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) == "" {
			return false
		}
		found = true
	}
	return found
}

func parseFormKV(text string) ([]kvRow, []string) {
	if looksLikeFormLines(text) {
		return parseKV(text)
	}
	var rows []kvRow
	var raw []string
	for _, part := range strings.Split(text, "&") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rawKey, rawValue, hasValue := strings.Cut(part, "=")
		key, err := url.QueryUnescape(rawKey)
		if err != nil {
			raw = append(raw, part)
			continue
		}
		row := kvRow{Enabled: true, Key: key}
		if hasValue {
			value, err := url.QueryUnescape(rawValue)
			if err != nil {
				raw = append(raw, part)
				continue
			}
			row.Value = value
		}
		rows = append(rows, row)
	}
	return rows, raw
}

func (kv *kvState) parse(text string) ([]kvRow, []string) {
	if kv.Form {
		return parseFormKV(text)
	}
	return parseKV(text)
}

func (kv *kvState) commit(dst *string) {
	if kv.Form {
		kv.commitForm(dst)
		return
	}
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

func (kv *kvState) commitForm(dst *string) {
	pairs := make([]string, 0, len(kv.Rows)+len(kv.Raw))
	for _, row := range kv.Rows {
		key := strings.TrimSpace(row.Key)
		if key == "" {
			continue
		}
		pairs = append(pairs, url.QueryEscape(key)+"="+url.QueryEscape(row.Value))
	}
	for _, raw := range kv.Raw {
		if strings.TrimSpace(raw) != "" {
			pairs = append(pairs, raw)
		}
	}
	*dst = strings.Join(pairs, "&")
	kv.Source = *dst
}

// kvEditor renders the text/form toggle and the active editor. field names
// the autocomplete slot ("params"/"headers").
func (r *Root) kvEditor(c *ui.Context, ed *Editor, field, hint string, text *string, kv *kvState) {
	t := c.Theme()
	if kv.Source != *text {
		kv.Rows, kv.Raw = kv.parse(*text)
		kv.Source = *text
	}
	mode := kv.Mode
	tableLabel := "表单"
	if kv.Form {
		tableLabel = "表格"
	}
	if ui.Segmented(c, &kv.Mode, "文本", tableLabel).Label("编辑模式").Changed() {
		kv.Source = ""
		if kv.Mode == 1 {
			kv.Rows, kv.Raw = kv.parse(*text)
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
		if !kv.Form {
			ui.Text(c, "启用").FontSize(10.5).TextColor(t.TextMuted).Width(34)
		}
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
				var enabled ui.Element
				if !kv.Form {
					enabled = ui.Checkbox(c, &row.Enabled, "").Label("启用该行")
				}
				key := ui.TextInput(c, &row.Key).Placeholder("Key").Width(170)
				value := ui.TextInput(c, &row.Value).Placeholder("Value").Grow(1)
				del := iconButtonPlain(c, iconX, "删除该行")
				changed := key.Changed() || value.Changed()
				if !kv.Form {
					changed = enabled.Changed() || changed
				}
				if changed {
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
