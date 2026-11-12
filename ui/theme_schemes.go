package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

type schemePalette struct {
	Background string
	Surface    string
	Border     string
	Text       string
	TextMuted  string
}

var defaultPalette = struct{ Light, Dark schemePalette }{
	Light: schemePalette{Background: "#f8f8f8", Surface: "#ffffff", Border: "#e3e3e3", Text: "#1f1f1f", TextMuted: "#666666"},
	Dark:  schemePalette{Background: "#1a1a1a", Surface: "#242424", Border: "#343434", Text: "#e6e6e6", TextMuted: "#b4b4b4"},
}

var schemePalettes = map[string]struct{ Light, Dark schemePalette }{
	"": defaultPalette,
	"ocean": {
		Light: schemePalette{Background: "#edf4fb", Surface: "#ffffff", Border: "#d3e2f2", Text: "#17212b", TextMuted: "#5b6b7c"},
		Dark:  schemePalette{Background: "#0f1822", Surface: "#172433", Border: "#2a3d52", Text: "#e6f0fa", TextMuted: "#9fb2c6"},
	},
	"forest": {
		Light: schemePalette{Background: "#eef5ef", Surface: "#ffffff", Border: "#d2e3d5", Text: "#15231a", TextMuted: "#5a6c5f"},
		Dark:  schemePalette{Background: "#0e1712", Surface: "#16241c", Border: "#2a3e32", Text: "#e2efe6", TextMuted: "#9db2a3"},
	},
	"warm": {
		Light: schemePalette{Background: "#faf4ec", Surface: "#fffdf9", Border: "#e8d9c8", Text: "#241b12", TextMuted: "#7a6958"},
		Dark:  schemePalette{Background: "#1a1410", Surface: "#251d16", Border: "#3b3026", Text: "#f1e7dc", TextMuted: "#b6a695"},
	},
}

var schemeOptions = []struct{ key, label string }{
	{"", "默认"}, {"ocean", "海洋"}, {"forest", "森林"}, {"warm", "暖阳"},
}

func schemeIndex(key string) int {
	for i, opt := range schemeOptions {
		if opt.key == key {
			return i
		}
	}
	return 0
}

func schemePaletteFor(key string, dark bool) schemePalette {
	pair, ok := schemePalettes[key]
	if !ok {
		pair = defaultPalette
	}
	if dark {
		return pair.Dark
	}
	return pair.Light
}

func applySchemePalette(t *ui.Theme, p schemePalette) {
	set := func(dst *ui.Color, hex string) {
		if hex = strings.TrimSpace(hex); hex != "" {
			*dst = ui.Hex(hex)
		}
	}
	set(&t.Background, p.Background)
	set(&t.Surface, p.Surface)
	set(&t.Border, p.Border)
	set(&t.Text, p.Text)
	set(&t.TextMuted, p.TextMuted)
	if p.Surface != "" {
		base := ui.Hex(p.Surface)
		t.SurfaceHover = base.Mix(t.Text, 0.06)
		t.SurfacePressed = base.Mix(t.Text, 0.10)
	}
}

// WindowBackgroundColor is the frameless window's backing color, matching
// the theme scheme and following the system mode when Mode is 0.
func WindowBackgroundColor(p app.ThemePrefs) string {
	light := schemePaletteFor(p.Scheme, false).Background
	dark := schemePaletteFor(p.Scheme, true).Background
	switch p.Mode {
	case 1:
		return light
	case 2:
		return dark
	}
	return "light-dark(" + light + ", " + dark + ")"
}

// WindowCornerRadius returns the frameless window's corner radius.
func WindowCornerRadius(p app.ThemePrefs) int {
	r := p.Radius
	if r <= 0 {
		r = 6
	}
	if r > 12 {
		r = 12
	}
	return r + 4
}

func themeRadius(p app.ThemePrefs) float32 {
	r := p.Radius
	if r < 0 || r > 12 {
		r = 6
	}
	if r == 0 {
		r = 6
	}
	return float32(r)
}
