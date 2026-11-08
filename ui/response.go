package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"

	"api-z/app"
	"api-z/httpclient"
)

func (ed *Editor) responsePane(c *ui.Context) {
	if ed.hist != nil {
		ed.historyPane(c)
		return
	}
	sess := ed.root.session(ed.ID)
	snap := sess.Snapshot()
	if snap.Done && snap.Status > 0 {
		ed.root.status = fmt.Sprintf("%s %d · %dms · %s", ed.Method, snap.Status, snap.DurationMS, app.FormatBytes(snap.Bytes))
	}

	if snap.AguiVer != ed.aguiVer {
		ed.agui = sess.AguiSnapshot()
		ed.aguiVer = snap.AguiVer
		ed.aguiRows = buildAguiRows(ed.agui)
	}
	if snap.ModelVer != ed.modelVer {
		text := sess.ModelText()
		ed.modelVer = snap.ModelVer
		ed.modelLines = strings.Split(text, "\n")
	}
	if ed.jsonSource != "" && ed.jsonSource != snap.SpoolPath {
		ed.jsonOpen = false
		ed.jsonEd = nil
		ed.jsonSource = ""
	}

	ui.Column(c).Fill().Children(func() {
		ed.responseToolbar(c, sess, snap)
		if ed.StreamMode == 0 && ed.jsonOpen && ed.jsonEd != nil {
			editor.View(c, ed.jsonEd).Grow(1).FillWidth()
			return
		}
		switch ed.StreamMode {
		case 0:
			ed.rawView(c, sess, snap)
		case 1:
			ed.modelView(c, snap)
		case 2:
			ed.aguiView(c, snap)
		}
	})
}

func (ed *Editor) responseToolbar(c *ui.Context, sess *app.Session, snap app.SessionSnapshot) {
	t := c.Theme()
	ui.Row(c).Padding(4, 8).Gap(10).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		statusColor := t.TextMuted
		statusText := "未发送"
		if snap.Failed {
			statusColor = t.Danger
			statusText = "失败"
		} else if snap.Status > 0 {
			statusText = fmt.Sprintf("%d %s", snap.Status, snap.StatusText)
			if snap.Status >= 400 {
				statusColor = t.Danger
			} else if snap.Status >= 300 {
				statusColor = t.Warning
			} else {
				statusColor = t.Success
			}
		} else if snap.Running {
			statusText = "请求中…"
		}
		ui.Text(c, statusText).FontSize(12).Bold().TextColor(statusColor)
		if snap.DurationMS > 0 {
			ui.Text(c, fmt.Sprintf("%dms", snap.DurationMS)).FontSize(11).TextColor(t.TextMuted)
		}
		if snap.Bytes > 0 {
			ui.Text(c, app.FormatBytes(snap.Bytes)).FontSize(11).TextColor(t.TextMuted)
		}
		if snap.IsSSE {
			ui.Text(c, fmt.Sprintf("Event %d", snap.Events)).FontSize(11).TextColor(t.TextMuted)
			if snap.TTFTMS > 0 {
				ui.Text(c, fmt.Sprintf("TTFT %dms", snap.TTFTMS)).FontSize(11).TextColor(t.TextMuted)
			}
		}
		if snap.Truncated {
			ui.Text(c, "已截断显示").FontSize(11).TextColor(t.Warning)
		}
		ui.Box(c).Grow(1)
		ui.MenuButton(c, "历史", func(m *ui.Menu) {
			list, err := ed.root.store.ListExchanges(ed.ID, 10)
			if err != nil || len(list) == 0 {
				m.Item("暂无历史").Disabled(true)
				return
			}
			for _, e := range list {
				label := fmt.Sprintf("%s  %d  %s", time.UnixMilli(e.SavedAt).Format("01-02 15:04"), e.Status, app.FormatBytes(e.SizeBytes))
				if m.Item(label).Chosen() {
					ed.openHistory(e.HARPath)
				}
			}
		}).Label("历史")
		canJSON := snap.Done && !snap.IsSSE && app.IsJSONContentType(snap.ContentType)
		if ui.Button(c, "JSON").Disabled(!canJSON).Clicked() {
			ed.toggleJSON(sess, snap)
		}
		ui.Segmented(c, &ed.StreamMode, "Raw", "Model", "AGUI").Label("响应视图")
		if ui.Button(c, "复制").Clicked() {
			c.WriteClipboard(sess.CopyText())
			c.Toast("已复制响应正文")
		}
		if snap.Running {
			if ui.Button(c, "取消").Clicked() {
				sess.Cancel()
			}
		}
		if snap.Error != "" {
			ui.Text(c, snap.Error).FontSize(11).TextColor(t.Danger).SingleLine().MaxWidth(360).
				Tooltip(snap.Error)
		}
	})
}

func (ed *Editor) openHistory(path string) {
	resp, err := app.ReadHARResponse(path)
	if err != nil {
		ed.root.err = "读取历史失败: " + err.Error()
		return
	}
	ed.hist = &resp
	body := strings.TrimSuffix(resp.Body, "\n")
	if body == "" {
		ed.histLines = []string{"（正文未保存，或超过 64MiB）"}
	} else {
		ed.histLines = strings.Split(body, "\n")
	}
	ed.histJSON = false
	ed.histJSONEd = nil
	ed.root.status = fmt.Sprintf("历史 %d · %dms · %s", resp.Status, resp.DurationMS, app.FormatBytes(resp.SizeBytes))
}

func (ed *Editor) historyPane(c *ui.Context) {
	t := c.Theme()
	resp := ed.hist
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(4, 8).Gap(10).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Text(c, "历史").FontSize(12).Bold().TextColor(t.Warning)
			ui.Text(c, fmt.Sprintf("%d %s", resp.Status, resp.StatusText)).FontSize(12)
			ui.Text(c, fmt.Sprintf("%dms", resp.DurationMS)).FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, app.FormatBytes(resp.SizeBytes)).FontSize(11).TextColor(t.TextMuted)
			if resp.IsSSE {
				ui.Text(c, fmt.Sprintf("Event %d", resp.EventCount)).FontSize(11).TextColor(t.TextMuted)
			}
			ui.Box(c).Grow(1)
			if ui.Button(c, "JSON").Clicked() {
				if ed.histJSONEd == nil {
					raw := resp.Body
					var buf bytes.Buffer
					if err := json.Indent(&buf, []byte(raw), "", "  "); err == nil {
						raw = buf.String()
					}
					b := editor.NewBuffer(raw)
					b.SetLanguage(editor.LanguageFor("response.json"))
					ed.histJSONEd = &editor.State{Buffer: b, ReadOnly: true}
				}
				ed.histJSON = !ed.histJSON
			}
			if ui.Button(c, "复制").Clicked() {
				c.WriteClipboard(resp.Body)
				c.Toast("已复制历史正文")
			}
			if ui.Button(c, "返回实时").Clicked() {
				ed.hist = nil
				ed.histLines = nil
			}
		})
		if ed.histJSON && ed.histJSONEd != nil {
			editor.View(c, ed.histJSONEd).Grow(1).FillWidth()
			return
		}
		ui.List(c, &ed.histList, len(ed.histLines), func(i int) {
			line := ed.histLines[i]
			if line == "" {
				line = " "
			}
			ui.Text(c, clipLine(line, 4096)).Font("monospace").FontSize(12).SingleLine()
		}).Grow(1)
	})
}

func (ed *Editor) toggleJSON(sess *app.Session, snap app.SessionSnapshot) {
	if ed.jsonOpen {
		ed.jsonOpen = false
		return
	}
	raw := sess.CopyText()
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err == nil {
		raw = buf.String()
	}
	b := editor.NewBuffer(raw)
	b.SetLanguage(editor.LanguageFor("response.json"))
	ed.jsonEd = &editor.State{Buffer: b, ReadOnly: true}
	ed.jsonSource = snap.SpoolPath
	ed.jsonOpen = true
}

func (ed *Editor) rawView(c *ui.Context, sess *app.Session, snap app.SessionSnapshot) {
	t := c.Theme()
	if snap.Binary {
		ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
			ui.Text(c, "二进制响应").FontSize(14).Bold()
			ui.Text(c, fmt.Sprintf("%s，已保存到：%s", app.FormatBytes(snap.Bytes), snap.SpoolPath)).
				TextColor(t.TextMuted).Selectable()
		})
		return
	}
	n := snap.LineCount
	ed.RawList.FollowEnd = snap.Running
	ui.List(c, &ed.RawList, n, func(i int) {
		line := sess.Line(i)
		if line == "" {
			line = " "
		}
		ui.Text(c, clipLine(line, 4096)).Font("monospace").FontSize(12).SingleLine()
	}).Grow(1).Children(func() {
		if n == 0 {
			msg := "等待响应…"
			if snap.Done {
				msg = "响应为空"
			}
			ui.Text(c, msg).Padding(12).TextColor(t.TextMuted)
		}
	})
}

func (ed *Editor) modelView(c *ui.Context, snap app.SessionSnapshot) {
	t := c.Theme()
	if len(ed.modelLines) == 0 {
		ui.Text(c, "没有可提取的模型文本（SSE 的 JSON delta）").Padding(12).TextColor(t.TextMuted)
		return
	}
	ed.ModelList.FollowEnd = snap.Running
	ui.List(c, &ed.ModelList, len(ed.modelLines), func(i int) {
		line := ed.modelLines[i]
		if line == "" {
			line = " "
		}
		ui.Text(c, clipLine(line, 4096)).Font("monospace").FontSize(12).SingleLine()
	}).Grow(1)
}

func (ed *Editor) aguiView(c *ui.Context, snap app.SessionSnapshot) {
	t := c.Theme()
	if len(ed.aguiRows) == 0 {
		ui.Text(c, "没有 AG-UI 事件（SSE 的 data 行需含 AG-UI type）").Padding(12).TextColor(t.TextMuted)
		return
	}
	ed.AguiList.FollowEnd = snap.Running
	ui.List(c, &ed.AguiList, len(ed.aguiRows), func(i int) {
		ed.aguiRow(c, ed.aguiRows[i], snap.Running)
	}).Grow(1).Padding(10, 12).Gap(10)
}

// buildAguiRows flattens messages into bounded rows. Markdown of a 600 KB
// single-line reply would otherwise shape the whole document every frame;
// chunks keep each layout small and let the list virtualize them.
func buildAguiRows(snap httpclient.AguiSnapshot) []aguiRow {
	const chunkSize = 8000
	var rows []aguiRow
	for _, m := range snap.Messages {
		if m.Kind != httpclient.AguiText {
			rows = append(rows, aguiRow{kind: m.Kind, msg: m, first: true, last: true})
			continue
		}
		chunks := splitText(m.Text, chunkSize)
		for i, chunk := range chunks {
			row := aguiRow{kind: m.Kind, msg: m, text: chunk, first: i == 0, last: i == len(chunks)-1}
			if i == 0 {
				row.role = m.Role
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func splitText(s string, n int) []string {
	if len(s) <= n {
		return []string{s}
	}
	var out []string
	for len(s) > n {
		cut := n
		if i := strings.LastIndexByte(s[:n], '\n'); i > n/2 {
			cut = i + 1
		}
		for cut > 0 && s[cut]&0xC0 == 0x80 {
			cut--
		}
		out = append(out, s[:cut])
		s = s[cut:]
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

func (ed *Editor) aguiRow(c *ui.Context, row aguiRow, running bool) {
	t := c.Theme()
	m := row.msg
	switch row.kind {
	case httpclient.AguiText:
		ui.Column(c).MaxWidth(920).Children(func() {
			if row.first && row.role != "" && row.role != "assistant" {
				ui.Text(c, "["+row.role+"]").FontSize(10.5).TextColor(t.TextMuted)
			}
			if running && !m.Complete && row.last {
				markdown.Stream(c, row.text, &ed.md)
			} else {
				markdown.View(c, row.text, &ed.md)
			}
		})
	case httpclient.AguiToolCall:
		ui.Column(c).Padding(8).Radius(8).Background(t.Surface).Border(1, t.Border).Gap(4).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "工具").FontSize(10.5).TextColor(t.TextMuted)
				ui.Text(c, defaultString(m.ToolName, "(unnamed)")).FontSize(12).Bold()
				if m.DurationMS > 0 {
					ui.Text(c, fmt.Sprintf("%dms", m.DurationMS)).FontSize(10.5).TextColor(t.TextMuted)
				}
				if !m.Complete {
					ui.Text(c, "运行中…").FontSize(10.5).TextColor(t.Warning)
				}
				ui.Spacer(c)
				if m.ToolCallID != "" {
					ui.Text(c, m.ToolCallID).FontSize(10).TextColor(t.TextMuted).SingleLine().MaxWidth(240)
				}
			})
			if m.Args != "" {
				ui.Text(c, "参数").FontSize(10.5).TextColor(t.TextMuted)
				ui.Text(c, truncate(m.Args, 16_000)).Font("monospace").FontSize(11.5).Selectable()
			}
			if m.Result != "" {
				ui.Text(c, "结果").FontSize(10.5).TextColor(t.TextMuted)
				ui.Text(c, truncate(m.Result, 32_000)).Font("monospace").FontSize(11.5).Selectable()
			}
		})
	case httpclient.AguiReasoning:
		ui.Column(c).Padding(8).Radius(8).Background(t.SurfaceHover).Gap(2).Children(func() {
			ui.Text(c, "思考").FontSize(10.5).TextColor(t.TextMuted)
			ui.Text(c, truncate(m.Text, 16_000)).FontSize(12).TextColor(t.TextMuted).Selectable()
		})
	case httpclient.AguiInterrupt:
		ui.Column(c).Padding(10).Radius(8).Border(1, t.Warning).Gap(4).Children(func() {
			ui.Text(c, "中断").FontSize(10.5).Bold().TextColor(t.Warning)
			if m.ToolName != "" {
				ui.Text(c, "工具: "+m.ToolName).FontSize(12)
			}
			if m.Text != "" {
				ui.Text(c, m.Text).FontSize(12).Selectable()
			}
		})
	}
}

// clipLine limits a raw line's laid-out text. SSE data lines can be
// hundreds of kilobytes; shaping all of one to ellipsize it blocks the
// frame. Copy still returns the full response.
func clipLine(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + " …"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…（已截断）"
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
