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
	cp := *base
	cp.Font = strings.TrimSpace(p.Font)
	accent := strings.TrimSpace(p.Accent)
	if accent == "" || !app.ValidHex(accent) {
		return &cp
	}
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
	key := fmt.Sprintf("%d|%s|%s|%d", r.themePrefs.Mode, r.themePrefs.Accent, r.themePrefs.Font, r.themePrefs.Scale)
	if key == r.themeKey {
		return
	}
	r.themeKey = key
	c.SetTheme(resolveTheme(r.baseTheme, r.themePrefs))
	c.SetZoom(themeScaleFactor(r.themePrefs.Scale))
}

func (r *Root) saveThemePrefs() {
	accent := strings.TrimSpace(r.accentHex)
	if accent != "" && !app.ValidHex(accent) {
		r.settingsErr = "强调色必须是 #rrggbb"
		return
	}
	r.themePrefs.Accent = accent
	r.themePrefs = app.NormalizeThemePrefs(r.themePrefs)
	if err := app.SaveThemePrefs(r.themePath, r.themePrefs); err != nil {
		r.settingsErr = "保存主题失败: " + err.Error()
		return
	}
	r.settingsErr = ""
	r.themeKey = ""
}

func (r *Root) resetTheme() {
	r.themePrefs.Mode = 0
	r.themePrefs.Accent = ""
	r.accentHex = ""
	r.saveThemePrefs()
}

var themeScaleSteps = []int{90, 100, 110, 125}

func themeScaleFactor(scale int) float32 {
	if scale <= 0 {
		scale = app.DefaultThemeScale
	}
	return float32(scale) / 100
}

// themeScaleIndex returns the step nearest to scale, so a hand-edited
// theme.json still shows a sensible choice.
func themeScaleIndex(scale int) int {
	if scale <= 0 {
		scale = app.DefaultThemeScale
	}
	best, bestDist := 0, 1<<30
	for i, step := range themeScaleSteps {
		dist := step - scale
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			best, bestDist = i, dist
		}
	}
	return best
}
