package ui

import (
	"regexp"
	"unicode/utf8"

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
	caretStart, caretEnd := input.TextSelection()
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
			startRune := utf8.RuneCountInString((*value)[:m[0]])
			endRune := startRune + utf8.RuneCountInString((*value)[m[0]:m[1]])
			if (caretStart > startRune && caretStart < endRune) ||
				(caretEnd > startRune && caretEnd < endRune) {
				continue // keep the caret visible while editing inside a token
			}
			x0 := measure((*value)[:m[0]])
			x1 := measure((*value)[:m[1]])
			if x1 <= x0 {
				continue
			}
			token := (*value)[m[0]:m[1]]
			layout := ui.ShapeText(token, font, 0)
			_, textH := layout.Size()
			y := rect.Y + (rect.H-textH)/2
			bg := ui.Rect{X: rect.X + pad + x0, Y: rect.Y + 3,
				W: x1 - x0, H: rect.H - 6}
			// Cover the input's own glyphs before drawing the token in the
			// accent color, so the variable reads as one colored chip.
			p.Fill(bg, t.Surface, 3)
			p.Fill(bg, t.Accent.Alpha(0.16), 3)
			p.TextLayout(layout, bg.X+2, y, t.Accent)
		}
	})
	return input
}
