package ui

import (
	"github.com/egoist/mygo/ui"
)

func (r *Root) settingsModal(c *ui.Context) {
	if !r.settingsOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &r.settingsOpen, func() {
		ui.Text(c, "设置").FontSize(15).Bold()
		ui.Tabs(c, &r.settingsTab, "主题", "快捷键")
		if r.settingsTab == 0 {
			r.themeSettings(c, t)
		} else {
			r.keymapSettings(c, t)
		}
		if r.settingsErr != "" {
			ui.Text(c, r.settingsErr).FontSize(12).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "关闭").Clicked() {
				r.recording = ""
				r.settingsOpen = false
			}
		})
	}).Width(780).Height(620)
}

func (r *Root) themeSettings(c *ui.Context, t *ui.Theme) {
	ui.Text(c, "外观模式").FontSize(12).TextColor(t.TextMuted)
	if ui.Segmented(c, &r.themePrefs.Mode, "跟随系统", "浅色", "深色").
		Label("外观模式").Changed() {
		r.saveThemePrefs()
	}
	ui.Text(c, "强调色").FontSize(12).TextColor(t.TextMuted)
	presets := []string{"#4f8cff", "#22c55e", "#a855f7", "#f59e0b", "#ef4444", "#06b6d4"}
	ui.Row(c).Gap(8).Children(func() {
		for _, hex := range presets {
			hex := hex
			btn := ui.ButtonBase(c).Label(hex).Padding(4).Radius(999)
			btn.Children(func() {
				box := ui.Box(c).Size(20, 20).Radius(10).Background(ui.Hex(hex))
				if r.accentHex == hex {
					box.Border(2, t.Text)
				}
			})
			if btn.Clicked() {
				r.accentHex = hex
				r.saveThemePrefs()
			}
		}
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "自定义").Width(64).TextColor(t.TextMuted)
		if ui.TextInput(c, &r.accentHex).Placeholder("#rrggbb").Width(160).Changed() {
			r.saveThemePrefs()
		}
		if ui.Button(c, "恢复默认").Clicked() {
			r.resetTheme()
		}
	})
	ui.Text(c, "预览").FontSize(12).TextColor(t.TextMuted)
	ui.Row(c).Gap(8).Padding(12).Radius(8).Background(t.Surface).Children(func() {
		ui.PrimaryButton(c, "主按钮")
		ui.Button(c, "次按钮")
		ui.Text(c, "文本示例").TextColor(t.Text)
	})
}

func (r *Root) keymapSettings(c *ui.Context, t *ui.Theme) {
	ui.Text(c, "点击「录制」后按下组合键；Esc 取消。冲突的旧绑定会被移除。").
		FontSize(11.5).TextColor(t.TextMuted)
	ui.Scroll(c).Height(400).Children(func() {
		for _, a := range keyActions {
			a := a
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(2, 0).Children(func() {
				ui.Text(c, a.Label).Width(190)
				if r.recording == a.ID {
					ui.Text(c, "按下快捷键…").FontSize(11.5).TextColor(t.Accent).Grow(1)
				} else {
					ui.Text(c, r.keymap[a.ID]).Font("monospace").FontSize(11.5).Width(150)
					ui.Spacer(c)
				}
				if r.recording == a.ID {
					if ui.Button(c, "取消").Clicked() {
						r.recording = ""
						r.recordMsg = "已取消"
					}
				} else if ui.Button(c, "录制").Clicked() {
					r.recording = a.ID
					r.recordMsg = ""
				}
				if ui.Button(c, "默认").Clicked() {
					r.keymap[a.ID] = a.Default
					r.saveKeymap()
					r.recordMsg = "已恢复 " + a.Label
				}
			})
		}
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "全部恢复默认").Clicked() {
			r.keymap = defaultKeymap()
			r.saveKeymap()
			r.recordMsg = "已恢复全部默认"
		}
		if r.recordMsg != "" {
			ui.Text(c, r.recordMsg).FontSize(11.5).TextColor(t.TextMuted)
		}
	})
}
