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
	if ed.jsonEd != nil {
		ed.jsonEd.Font = ed.root.bodyEditorFont()
	}
	if ed.jsonSource != "" && ed.jsonSource != snap.SpoolPath {
		ed.jsonOpen = false
		ed.jsonEd = nil
		ed.jsonSource = ""
	}
	ed.maybeAutoOpenJSON(sess, snap)

	ui.Column(c).Fill().Children(func() {
		ed.responseToolbar(c, sess, snap)
		ed.responseTabs(c, sess, snap)
		switch ed.RespTab {
		case 1:
			spec, sent := sess.Spec()
			var requestHeaders []string
			if sent {
				requestHeaders = requestHeaderLines(spec)
			}
			ed.responseHeaders(c, requestHeaders, snap.Headers)
			return
		case 2:
			ed.responseRequest(c, sess)
			return
		}
		if ed.StreamMode == 0 && ed.jsonOpen && ed.jsonEd != nil {
			editor.View(c, ed.jsonEd).Grow(1).FillWidth()
			return
		}
		mode := ed.StreamMode
		if !app.IsSSEContentType(snap.ContentType) {
			mode = 0
		}
		switch mode {
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
			metrics := []string{fmt.Sprintf("Event %d", snap.Events)}
			if snap.TTFTMS > 0 {
				metrics = append(metrics, fmt.Sprintf("TTFT %dms", snap.TTFTMS))
			}
			if snap.TPOTMS > 0 {
				metrics = append(metrics, fmt.Sprintf("TPOT %dms", snap.TPOTMS))
			}
			ui.Text(c, strings.Join(metrics, " · ")).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
		if snap.Truncated {
			ui.Text(c, "已截断显示").FontSize(11).TextColor(t.Warning)
		}
		ui.Box(c).Grow(1)
		iconMenuButton(c, iconHistory, "历史", func(m *ui.Menu) {
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
		if iconButton(c, iconCopy, "复制").Clicked() {
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

func (ed *Editor) responseTabs(c *ui.Context, sess *app.Session, snap app.SessionSnapshot) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Tabs(c, &ed.RespTab, "Body", "Headers", "Request")
		ui.Spacer(c)
		if ed.RespTab == 0 {
			canJSON := snap.Done && !snap.IsSSE && app.IsJSONContentType(snap.ContentType)
			if iconButton(c, iconFormat, "JSON").Disabled(!canJSON).Clicked() {
				ed.toggleJSON(sess, snap)
			}
			if app.IsSSEContentType(snap.ContentType) {
				modes := []string{"Raw", "Model", "AGUI"}
				mode := ed.StreamMode
				if mode < 0 || mode >= len(modes) {
					mode = 0
				}
				cur := modes[mode]
				if ui.Select(c, &cur, modes).Label("响应视图").Width(110).Changed() {
					for i, name := range modes {
						if name == cur {
							ed.StreamMode = i
						}
					}
				}
			}
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
			if resp.TTFTMS > 0 {
				ui.Text(c, fmt.Sprintf("TTFT %dms", resp.TTFTMS)).FontSize(11).TextColor(t.TextMuted)
			}
			if resp.TPOTMS > 0 {
				ui.Text(c, fmt.Sprintf("TPOT %dms", resp.TPOTMS)).FontSize(11).TextColor(t.TextMuted)
			}
			ui.Box(c).Grow(1)
			if iconButton(c, iconCopy, "复制").Clicked() {
				c.WriteClipboard(resp.Body)
				c.Toast("已复制历史正文")
			}
			if ui.Button(c, "返回实时").Clicked() {
				ed.hist = nil
				ed.histLines = nil
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Tabs(c, &ed.RespTab, "Body", "Headers", "Request")
			ui.Spacer(c)
			if ed.RespTab == 0 && iconButton(c, iconFormat, "JSON").Clicked() {
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
		})
		if ed.RespTab == 1 {
			ed.responseHeaders(c, resp.RequestHeaders, resp.Headers)
			return
		}
		if ed.RespTab == 2 {
			ed.historyRequest(c, resp)
			return
		}
		if ed.histJSON && ed.histJSONEd != nil {
			ed.histJSONEd.Font = ed.root.bodyEditorFont()
			editor.View(c, ed.histJSONEd).Grow(1).FillWidth()
			return
		}
		ui.List(c, &ed.histList, len(ed.histLines), func(i int) {
			line := ed.histLines[i]
			if line == "" {
				line = " "
			}
			ui.Text(c, clipLine(line, 4096)).Font(ed.root.bodyFontFamily("monospace")).
				FontSize(ed.root.bodyTextSize(12)).SingleLine()
		}).Grow(1)
	})
}

func (ed *Editor) toggleJSON(sess *app.Session, snap app.SessionSnapshot) {
	if ed.jsonOpen {
		ed.jsonOpen = false
		return
	}
	ed.openJSON(sess, snap)
}

func (ed *Editor) openJSON(sess *app.Session, snap app.SessionSnapshot) {
	raw := sess.CopyText()
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err == nil {
		raw = buf.String()
	}
	b := editor.NewBuffer(raw)
	b.SetLanguage(editor.LanguageFor("response.json"))
	ed.jsonEd = &editor.State{Buffer: b, ReadOnly: true, Font: ed.root.bodyEditorFont()}
	ed.jsonSource = snap.SpoolPath
	ed.jsonOpen = true
}

// maybeAutoOpenJSON shows formatted JSON once per response when the server
// declared a JSON content type. A manual toggle afterwards is respected.
func (ed *Editor) maybeAutoOpenJSON(sess *app.Session, snap app.SessionSnapshot) {
	if !snap.Done || snap.IsSSE || snap.Binary || !app.IsJSONContentType(snap.ContentType) {
		return
	}
	if ed.jsonAutoSource == snap.SpoolPath {
		return
	}
	ed.jsonAutoSource = snap.SpoolPath
	ed.openJSON(sess, snap)
}

func (ed *Editor) rawView(c *ui.Context, sess *app.Session, snap app.SessionSnapshot) {
	t := c.Theme()
	if snap.Binary {
		ed.binaryView(c, snap)
		return
	}
	n := snap.LineCount
	if snap.IsSSE && ed.SelectedEvent >= snap.Events {
		ed.SelectedEvent = -1
		ed.eventIdx = -1
		ed.eventEd = nil
	}
	if snap.IsSSE && ed.SelectedEvent >= 0 {
		ed.ensureEventEditor(sess, ed.SelectedEvent)
	}
	ed.RawList.FollowEnd = snap.Running
	ui.Column(c).Fill().Children(func() {
		list := func() {
			ui.List(c, &ed.RawList, n, func(i int) {
				line := sess.Line(i)
				if line == "" {
					line = " "
				}
				event := -1
				if snap.IsSSE {
					event = sess.LineEvent(i)
				}
				if event >= 0 {
					selected := event == ed.SelectedEvent
					btn := ui.ButtonBase(c).Label("SSE 事件行").Justify(ui.Start).Padding(0, 2).Radius(3)
					if selected {
						btn.Background(t.Accent.Alpha(0.18))
					} else if btn.Hovered() {
						btn.Background(t.SurfaceHover)
					}
					btn.Children(func() {
						ed.sseLine(c, line)
					})
					if btn.Clicked() {
						if ed.SelectedEvent == event {
							ed.SelectedEvent = -1
							ed.eventIdx = -1
							ed.eventEd = nil
						} else {
							ed.SelectedEvent = event
							ed.eventIdx = -1
							ed.eventEd = nil
						}
					}
					return
				}
				ui.Text(c, clipLine(line, 4096)).Font(ed.root.bodyFontFamily("monospace")).
					FontSize(ed.root.bodyTextSize(12)).SingleLine()
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
		if snap.IsSSE && ed.SelectedEvent >= 0 && ed.eventEd != nil {
			ui.SplitVerticalPercent(c, &ed.EventSplit, list, func() {
				ed.sseEventPanel(c, sess, snap)
			}).Grow(1)
		} else {
			list()
		}
	})
}

// sseLine renders one SSE display line with a muted timestamp and data:
// prefix, while the payload keeps the normal text color.
func (ed *Editor) sseLine(c *ui.Context, line string) {
	t := c.Theme()
	family := ed.root.bodyFontFamily("monospace")
	size := ed.root.bodyTextSize(12)
	ts, rest := splitSSETimestamp(line)
	if ts != "" {
		ui.Text(c, ts).Font(family).FontSize(size).TextColor(t.TextMuted)
		ui.Box(c).Width(4)
	}
	switch {
	case rest == "":
		ui.Text(c, " ").Font(family).FontSize(size)
	case strings.HasPrefix(rest, "data:"):
		ui.Text(c, "data:").Font(family).FontSize(size).TextColor(t.TextMuted)
		ui.Box(c).Width(4)
		payload := strings.TrimPrefix(rest, "data:")
		payload = strings.TrimPrefix(payload, " ")
		ui.Text(c, clipLine(payload, 4096)).Font(family).FontSize(size).SingleLine()
	default:
		ui.Text(c, clipLine(rest, 4096)).Font(family).FontSize(size).SingleLine()
	}
}

func splitSSETimestamp(line string) (string, string) {
	if len(line) < 13 {
		return "", line
	}
	for i := 0; i < 12; i++ {
		ch := line[i]
		switch i {
		case 2, 5:
			if ch != ':' {
				return "", line
			}
		case 8:
			if ch != '.' {
				return "", line
			}
		default:
			if ch < '0' || ch > '9' {
				return "", line
			}
		}
	}
	if line[12] != ' ' {
		return "", line
	}
	return line[:12], line[13:]
}

func (ed *Editor) ensureEventEditor(sess *app.Session, idx int) {
	if ed.eventIdx == idx {
		return
	}
	ed.eventIdx = idx
	ed.eventEd = nil
	ev, ok := sess.Event(idx)
	if !ok {
		return
	}
	raw := strings.TrimSpace(ev.Data)
	if raw == "" || !json.Valid([]byte(raw)) {
		return
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return
	}
	b := editor.NewBuffer(buf.String())
	b.SetLanguage(editor.LanguageFor("response.json"))
	ed.eventEd = &editor.State{Buffer: b, ReadOnly: true, Font: ed.root.bodyEditorFont()}
}

func (ed *Editor) sseEventPanel(c *ui.Context, sess *app.Session, snap app.SessionSnapshot) {
	t := c.Theme()
	// A click on another event row during the split's first pane rebuild
	// clears eventEd before the second pane is built.
	if ed.eventEd == nil {
		return
	}
	ev, ok := sess.Event(ed.SelectedEvent)
	if !ok {
		return
	}
	ui.Column(c).Fill().BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("Event #%d", ed.SelectedEvent+1)).FontSize(11.5).Bold()
			if !ev.At.IsZero() {
				ui.Text(c, ev.At.Format("15:04:05.000")).Font("monospace").FontSize(11).
					TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			if ui.Button(c, "复制 JSON").Clicked() {
				c.WriteClipboard(ed.eventEd.Buffer.String())
				c.Toast("已复制事件 JSON")
			}
		})
		ed.eventEd.Font = ed.root.bodyEditorFont()
		editor.View(c, ed.eventEd).Grow(1).FillWidth()
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
		ui.Text(c, clipLine(line, 4096)).Font(ed.root.bodyFontFamily("monospace")).
			FontSize(ed.root.bodyTextSize(12)).SingleLine()
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
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "中断").FontSize(10.5).Bold().TextColor(t.Warning)
				if m.Reason != "" {
					ui.Text(c, "· "+m.Reason).FontSize(10.5).TextColor(t.TextMuted)
				}
				if m.ToolName != "" {
					ui.Text(c, "· "+m.ToolName).FontSize(10.5).TextColor(t.Accent)
				}
			})
			if m.Text != "" {
				ui.Text(c, m.Text).FontSize(12).Selectable()
			}
			if m.UIType != "" || m.UIContent != "" {
				ui.Text(c, "UI 组件").FontSize(10.5).TextColor(t.TextMuted)
				if m.UIType != "" {
					ui.Text(c, m.UIType).FontSize(11).Bold().TextColor(t.Warning)
				}
				if m.UIContent != "" {
					ui.Text(c, truncate(m.UIContent, 32_000)).Font("monospace").FontSize(11.5).
						Selectable().TextColor(t.Text)
				}
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
