package ui

import (
	"regexp"

	"github.com/egoist/mygo/ui"
)

var urlVariablePattern = regexp.MustCompile(`\{\{\s*[^{}]+\s*\}\}`)

// urlInput is a single-line URL field that paints a soft accent background
// behind {{variables}}. The highlight is drawn behind the glyphs' bounds
// over a short field; very long, horizontally scrolled URLs skip it rather
// than misplace the marks.
func (r *Root) urlInput(c *ui.Context, value *string) ui.Element {
	t := c.Theme()
	input := ui.TextInput(c, value).Placeholder("https://{{host}}/api").Grow(1)
	font := ui.Font{Family: t.Font, Size: t.FontSize}
	pad := t.Space(2.5) + 1
	input.DrawOver(func(p *ui.Painter, rect ui.Rect) {
		matches := urlVariablePattern.FindAllStringIndex(*value, -1)
		if len(matches) == 0 {
			return
		}
		textW, _ := ui.ShapeText(*value, font, 0).Size()
		if textW > rect.W-2*pad {
			return
		}
		measure := func(s string) float32 {
			w, _ := ui.ShapeText(s, font, 0).Size()
			return w
		}
		for _, m := range matches {
			x0 := measure((*value)[:m[0]])
			x1 := measure((*value)[:m[1]])
			if x1 <= x0 {
				continue
			}
			p.Fill(ui.Rect{X: rect.X + pad + x0, Y: rect.Y + 3,
				W: x1 - x0, H: rect.H - 6}, t.Accent.Alpha(0.18), 3)
		}
	})
	return input
}
