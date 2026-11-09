package ui

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// findVarPrefix finds an unclosed "{{prefix" before caret. Offsets are runes.
func findVarPrefix(text string, caret int) (start int, prefix string, ok bool) {
	runes := []rune(text)
	if caret < 0 || caret > len(runes) {
		return 0, "", false
	}
	start = -1
	for i := caret - 2; i >= 0; i-- {
		if runes[i] == '{' && runes[i+1] == '{' {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, "", false
	}
	prefix = string(runes[start+2 : caret])
	if strings.ContainsAny(prefix, "{}\n\r\t") {
		return 0, "", false
	}
	return start, prefix, true
}

// insertVariable replaces [start,end) with "{{key}}" (or "{{key" when the
// closing braces are already after the caret) and returns the new caret.
func insertVariable(text string, start, end int, key string) (string, int) {
	runes := []rune(text)
	if start < 0 || end < start || end > len(runes) {
		return text, end
	}
	suffix := string(runes[end:])
	insert := "{{" + key
	if !strings.HasPrefix(suffix, "}}") {
		insert += "}}"
	}
	out := make([]rune, 0, len(runes)-(end-start)+len([]rune(insert)))
	out = append(out, runes[:start]...)
	out = append(out, []rune(insert)...)
	out = append(out, runes[end:]...)
	return string(out), start + len([]rune(insert))
}

func (r *Root) envValueMap() map[string]string {
	values := map[string]string{}
	if env := r.env.Active(); env != nil {
		for _, v := range env.Variables {
			if k := strings.TrimSpace(v.Key); k != "" {
				values[k] = v.Value
			}
		}
	}
	if len(values) == 0 {
		for i := range r.env.Environments {
			for _, v := range r.env.Environments[i].Variables {
				if k := strings.TrimSpace(v.Key); k != "" {
					values[k] = v.Value
				}
			}
		}
	}
	return values
}

func (r *Root) envKeyOptions() []string {
	values := r.envValueMap()
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (r *Root) matchEnvKeys(prefix string) ([]string, map[string]string) {
	values := r.envValueMap()
	q := strings.ToLower(strings.TrimSpace(prefix))
	var keys []string
	for k := range values {
		if q == "" || strings.Contains(strings.ToLower(k), q) {
			keys = append(keys, k)
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		pi := strings.HasPrefix(strings.ToLower(keys[i]), q)
		pj := strings.HasPrefix(strings.ToLower(keys[j]), q)
		if pi != pj {
			return pi
		}
		return keys[i] < keys[j]
	})
	if len(keys) > 50 {
		keys = keys[:50]
	}
	return keys, values
}

func (r *Root) varAutocomplete(c *ui.Context, ed *Editor, field string, anchor ui.Element, text *string, changed bool) {
	focused := anchor.Focused()
	if !focused && !(ed.autoOpen && ed.autoField == field) {
		return
	}
	if !changed && !(ed.autoOpen && ed.autoField == field) {
		return
	}
	var start, end int
	var prefix string
	if focused {
		_, end = anchor.TextSelection()
		s, p, ok := findVarPrefix(*text, end)
		if !ok {
			ed.autoOpen = false
			return
		}
		start, prefix = s, p
		ed.autoStart, ed.autoEnd = start, end
	} else {
		start, end = ed.autoStart, ed.autoEnd
		runes := []rune(*text)
		if start < 0 || end < start || end > len(runes) || end < start+2 {
			ed.autoOpen = false
			return
		}
		prefix = string(runes[start+2 : end])
	}
	keys, values := r.matchEnvKeys(prefix)
	if len(keys) == 0 {
		ed.autoOpen = false
		return
	}
	if ed.autoField != field {
		ed.autoField = field
		ed.autoOpen = true
	}
	if changed {
		ed.autoOpen = true
	}
	ui.Popover(c, anchor, &ed.autoOpen, func() {
		ui.Column(c).Gap(1).Padding(4).Children(func() {
			for _, key := range keys {
				key := key
				btn := ui.Button(c, "{{"+key+"}}").Justify(ui.Start).Padding(3, 8).Radius(5)
				if v := values[key]; v != "" {
					btn = btn.Tooltip("值: " + v)
				}
				if btn.Clicked() {
					newText, caret := insertVariable(*text, start, end, key)
					*text = newText
					ed.Dirty = true
					ed.autoOpen = false
					ed.autoField = ""
					anchor.SetTextSelection(caret, caret)
				}
			}
		})
	}).Width(300)
}

func (ed *Editor) completeBody(req editor.CompletionRequest) {
	st := ed.bodyEd
	if st == nil {
		return
	}
	text := st.Buffer.String()
	start := req.Offset - len(req.Prefix)
	if start < 2 || start > len(text) || text[start-2:start] != "{{" {
		st.ShowCompletions(start, nil)
		return
	}
	keys, values := ed.root.matchEnvKeys(req.Prefix)
	suffix := text[req.Offset:]
	items := make([]editor.Completion, 0, len(keys))
	for _, key := range keys {
		insert := key
		if !strings.HasPrefix(suffix, "}}") {
			insert += "}}"
		}
		items = append(items, editor.Completion{Label: key, Insert: insert, Detail: values[key]})
	}
	st.ShowCompletions(start, items)
}
