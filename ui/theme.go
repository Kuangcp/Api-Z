package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func resolveTheme(base *ui.Theme, p app.ThemePrefs) *ui.Theme {
	if base == nil {
		base = ui.LightTheme()
	}
	switch p.Mode {
	case 1:
		base = ui.LightTheme()
	case 2:
		base = ui.DarkTheme()
	}
	accent := strings.TrimSpace(p.Accent)
	if accent == "" || !app.ValidHex(accent) {
		return base
	}
	cp := *base
	col := ui.Hex(accent)
	cp.Accent = col
	cp.AccentHover = col.Mix(ui.RGB(255, 255, 255), 0.14)
	cp.AccentPressed = col.Mix(ui.RGB(0, 0, 0), 0.16)
	cp.InactiveAccent = col
	cp.AccentText = ui.RGB(255, 255, 255)
	return &cp
}

func (r *Root) applyTheme(c *ui.Context) {
	if r.baseTheme == nil {
		cp := *c.Theme()
		r.baseTheme = &cp
	}
	key := fmt.Sprintf("%d|%s", r.themePrefs.Mode, r.themePrefs.Accent)
	if key == r.themeKey {
		return
	}
	r.themeKey = key
	c.SetTheme(resolveTheme(r.baseTheme, r.themePrefs))
}

func (r *Root) saveThemePrefs() {
	accent := strings.TrimSpace(r.accentHex)
	if accent != "" && !app.ValidHex(accent) {
		r.settingsErr = "强调色必须是 #rrggbb"
		return
	}
	r.themePrefs.Accent = accent
	if err := app.SaveThemePrefs(r.themePath, r.themePrefs); err != nil {
		r.settingsErr = "保存主题失败: " + err.Error()
		return
	}
	r.settingsErr = ""
	r.themeKey = ""
}

func (r *Root) resetTheme() {
	r.themePrefs = app.ThemePrefs{}
	r.accentHex = ""
	r.saveThemePrefs()
}
