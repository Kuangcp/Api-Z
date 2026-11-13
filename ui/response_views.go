package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
	"api-z/httpclient"
)

func (ed *Editor) responseHeaders(c *ui.Context, requestHeaders, responseHeaders []string) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Padding(8).Children(func() {
		headerSection(c, t, "请求头", requestHeaders, "尚未发送")
		ui.Box(c).Height(1).Margin(8, 0).Background(t.Border)
		headerSection(c, t, "响应头", responseHeaders, "暂无响应头（请求尚未完成或失败）")
	})
}

func headerSection(c *ui.Context, t *ui.Theme, title string, headers []string, empty string) {
	ui.Text(c, fmt.Sprintf("%s（%d）", title, len(headers))).FontSize(11).Bold().
		TextColor(t.TextMuted).Padding(2, 4)
	if len(headers) == 0 {
		ui.Text(c, empty).FontSize(11.5).TextColor(t.TextMuted).Padding(2, 4)
		return
	}
	ui.Row(c).Gap(12).Padding(2, 4).Children(func() {
		ui.Text(c, "名称").FontSize(10.5).TextColor(t.TextMuted).Width(220)
		ui.Text(c, "值").FontSize(10.5).TextColor(t.TextMuted).Grow(1)
	})
	for _, line := range headers {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			value = line
		}
		ui.Row(c).Gap(12).AlignItems(ui.Start).Padding(3, 4).Children(func() {
			ui.Text(c, strings.TrimSpace(name)).Font("monospace").FontSize(11.5).
				TextColor(t.TextMuted).Width(220)
			ui.Text(c, strings.TrimSpace(value)).Font("monospace").FontSize(11.5).
				Selectable().Grow(1)
		})
	}
}

func requestHeaderLines(spec httpclient.Spec) []string {
	lines := make([]string, 0, len(spec.Headers)+1)
	for _, h := range spec.Headers {
		lines = append(lines, h.Name+": "+h.Value)
	}
	return lines
}

func (ed *Editor) responseRequest(c *ui.Context, sess *app.Session) {
	spec, sent := sess.Spec()
	if !sent {
		ui.Text(c, "尚未发送请求").Padding(12).TextColor(c.Theme().TextMuted)
		return
	}
	ed.requestTextView(c, spec)
}

func (ed *Editor) historyRequest(c *ui.Context, resp *app.HARResponse) {
	var headers []httpclient.Header
	for _, line := range resp.RequestHeaders {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers = append(headers, httpclient.Header{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}
	ed.requestTextView(c, httpclient.Spec{
		Method:  resp.RequestMethod,
		URL:     resp.RequestURL,
		Headers: headers,
		Body:    []byte(resp.RequestBody),
	})
}

func (ed *Editor) requestTextView(c *ui.Context, spec httpclient.Spec) {
	t := c.Theme()
	text := app.FormatHTTP(spec)
	ui.Row(c).Padding(4, 8).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Text(c, "实际发送").FontSize(11).TextColor(t.TextMuted)
		ui.Text(c, strings.ToUpper(defaultString(spec.Method, "GET"))).
			FontSize(11).Bold().TextColor(methodColor(c, spec.Method))
		ui.Text(c, spec.URL).FontSize(11).TextColor(t.TextMuted).SingleLine().Grow(1)
		if ui.Button(c, "复制请求").Clicked() {
			c.WriteClipboard(text)
			c.Toast("已复制实际请求")
		}
	})
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	ed.RequestList.FollowEnd = false
	ui.List(c, &ed.RequestList, len(lines), func(i int) {
		line := lines[i]
		if line == "" {
			line = " "
		}
		ui.Text(c, clipLine(line, 4096)).Font("monospace").FontSize(12).SingleLine()
	}).Grow(1).Padding(6, 8)
}
