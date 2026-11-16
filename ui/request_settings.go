package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func fmtTimeoutSeconds(ms int) string {
	v := float64(ms) / 1000
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

func (r *Root) requestTimeoutsMS() (int, int) {
	p := app.NormalizeRequestPrefs(r.requestPrefs)
	return p.ReadTimeoutMS, p.TotalTimeoutMS
}

func (r *Root) requestSettings(c *ui.Context, t *ui.Theme) {
	ui.Scroll(c).Height(480).Children(func() {
		ui.Text(c, "请求超时").FontSize(13).Bold()
		ui.Text(c, "调试 AI SSE 时可把总超时调大；流式响应持续期间也受总超时限制。").
			FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "首字节读超时（秒）").Width(160).TextColor(t.TextMuted)
			ui.TextInput(c, &r.readTimeoutInput).Placeholder("120").Width(130)
			ui.Text(c, "等待响应头/首字节的最长时间；默认 120s").
				FontSize(10.5).TextColor(t.TextMuted)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "完整请求超时（秒）").Width(160).TextColor(t.TextMuted)
			ui.TextInput(c, &r.totalTimeoutInput).Placeholder("120").Width(130)
			ui.Text(c, "包含连接、等待和读取完整响应；默认 120s").
				FontSize(10.5).TextColor(t.TextMuted)
		})
		ui.Text(c, "范围 1–86400 秒；修改后对下一次发送生效。").
			FontSize(10.5).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "保存请求设置").Clicked() {
				r.saveRequestSettings()
			}
			if ui.Button(c, "恢复默认").Clicked() {
				r.readTimeoutInput = fmtTimeoutSeconds(app.DefaultReadTimeoutMS)
				r.totalTimeoutInput = fmtTimeoutSeconds(app.DefaultTotalTimeoutMS)
				r.requestPrefs = app.DefaultRequestPrefs()
				if err := app.SaveRequestPrefs(r.requestPrefsPath, r.requestPrefs); err != nil {
					r.settingsErr = "保存请求设置失败: " + err.Error()
					return
				}
				r.settingsErr = ""
				r.status = "请求超时已恢复默认"
			}
		})
		if r.settingsErr != "" {
			ui.Text(c, r.settingsErr).FontSize(11.5).TextColor(t.Danger)
		}
	})
}

func (r *Root) saveRequestSettings() {
	read, err := parseTimeoutSeconds(r.readTimeoutInput)
	if err != nil {
		r.settingsErr = "首字节读超时: " + err.Error()
		return
	}
	total, err := parseTimeoutSeconds(r.totalTimeoutInput)
	if err != nil {
		r.settingsErr = "完整请求超时: " + err.Error()
		return
	}
	prefs := app.NormalizeRequestPrefs(app.RequestPrefs{ReadTimeoutMS: read, TotalTimeoutMS: total})
	if err := app.SaveRequestPrefs(r.requestPrefsPath, prefs); err != nil {
		r.settingsErr = "保存请求设置失败: " + err.Error()
		return
	}
	r.requestPrefs = prefs
	r.readTimeoutInput = fmtTimeoutSeconds(prefs.ReadTimeoutMS)
	r.totalTimeoutInput = fmtTimeoutSeconds(prefs.TotalTimeoutMS)
	r.settingsErr = ""
	r.status = fmt.Sprintf("请求超时已保存：首字节 %s · 总超时 %s",
		fmtTimeoutSeconds(prefs.ReadTimeoutMS)+"s", fmtTimeoutSeconds(prefs.TotalTimeoutMS)+"s")
}

func parseTimeoutSeconds(s string) (int, error) {
	s = strings.TrimSpace(s)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("请输入大于 0 的秒数")
	}
	ms := int(v*1000 + 0.5)
	if ms < app.MinRequestTimeoutMS || ms > app.MaxRequestTimeoutMS {
		return 0, fmt.Errorf("范围应为 1–86400 秒")
	}
	return ms, nil
}
