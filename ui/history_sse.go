package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"api-z/httpclient"
)

func (ed *Editor) parseHistorySSE() {
	lines := ed.histLines
	ed.histLineEvent = make([]int, len(lines))
	for i := range ed.histLineEvent {
		ed.histLineEvent[i] = -1
	}
	var parser httpclient.SSEParser
	model := &httpclient.ModelState{}
	agui := httpclient.NewAguiState()
	var pending []int
	finish := func(ev httpclient.SSEEvent) {
		idx := len(ed.histEvents)
		ed.histEvents = append(ed.histEvents, ev)
		for _, line := range pending {
			if line >= 0 && line < len(ed.histLineEvent) {
				ed.histLineEvent[line] = idx
			}
		}
		pending = pending[:0]
		model.Apply(ev)
		agui.Apply(ev)
	}
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) != "" {
			pending = append(pending, i)
		}
		if ev, ok := parser.Line(line, time.Now()); ok {
			finish(ev)
		}
	}
	if ev, ok := parser.Flush(time.Now()); ok {
		finish(ev)
	}
	ed.histModelLines = strings.Split(model.Text(), "\n")
	ed.histAguiRows = buildAguiRows(agui.Snapshot())
}

func (ed *Editor) ensureHistEventEditor() {
	if ed.histEventIdx == ed.histSelectedEvent {
		return
	}
	ed.histEventIdx = ed.histSelectedEvent
	ed.histEventEd = nil
	if ed.histSelectedEvent < 0 || ed.histSelectedEvent >= len(ed.histEvents) {
		return
	}
	raw := strings.TrimSpace(ed.histEvents[ed.histSelectedEvent].Data)
	if raw == "" || !json.Valid([]byte(raw)) {
		return
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return
	}
	b := editor.NewBuffer(buf.String())
	b.SetLanguage(editor.LanguageFor("response.json"))
	ed.histEventEd = &editor.State{Buffer: b, ReadOnly: true, Font: ed.root.bodyEditorFont()}
}

func (ed *Editor) histEventPanel(c *ui.Context) {
	if ed.histEventEd == nil {
		return
	}
	t := c.Theme()
	ui.Column(c).Fill().BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("Event #%d", ed.histSelectedEvent+1)).FontSize(11.5).Bold()
			ui.Spacer(c)
			if ui.Button(c, "复制 JSON").Clicked() {
				c.WriteClipboard(ed.histEventEd.Buffer.String())
				c.Toast("已复制事件 JSON")
			}
		})
		ed.histEventEd.Font = ed.root.bodyEditorFont()
		editor.View(c, ed.histEventEd).Grow(1).FillWidth()
	})
}

func (ed *Editor) sseEventRow(c *ui.Context, line string, event int, selected bool, onClick func()) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Label("SSE 事件行").Justify(ui.Start).Padding(0, 2).Radius(3)
	if selected {
		btn.Background(t.Accent.Alpha(0.18))
	} else if btn.Hovered() {
		btn.Background(t.SurfaceHover)
	}
	btn.Children(func() { ed.sseLine(c, line) })
	if btn.Clicked() {
		onClick()
	}
}
