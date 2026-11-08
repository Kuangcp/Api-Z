package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (r *Root) touchMRU(id string) {
	if id == "" {
		return
	}
	out := make([]string, 0, len(r.mru)+1)
	out = append(out, id)
	for _, v := range r.mru {
		if v != id {
			out = append(out, v)
		}
	}
	if len(out) > 30 {
		out = out[:30]
	}
	r.mru = out
}

func (r *Root) removeMRU(id string) {
	out := r.mru[:0]
	for _, v := range r.mru {
		if v != id {
			out = append(out, v)
		}
	}
	r.mru = out
}

// switchRecent is Alt-Tab for requests: the second most recently used tab
// becomes active, and the two trade places, so repeated Ctrl+Tab toggles.
func (r *Root) switchRecent() {
	if len(r.mru) < 2 {
		return
	}
	id := r.mru[1]
	for i, ed := range r.tabs {
		if ed.ID == id {
			r.active = i
			r.selected = id
			r.touchMRU(id)
			return
		}
	}
}

func (r *Root) globalSearch(c *ui.Context) {
	if !r.searchOpen {
		return
	}
	ui.Modal(c, &r.searchOpen, func() {
		t := c.Theme()
		ui.Text(c, "全局搜索").FontSize(15).Bold()
		input := ui.TextInput(c, &r.globalQuery).Placeholder("请求名 / URL / 方法").AutoFocus()
		q := strings.ToLower(strings.TrimSpace(r.globalQuery))
		matches := make([]app.Request, 0, 64)
		for _, req := range r.tree.Requests {
			if q == "" ||
				strings.Contains(strings.ToLower(req.Name), q) ||
				strings.Contains(strings.ToLower(req.URL), q) ||
				strings.Contains(strings.ToLower(req.Method), q) {
				matches = append(matches, req)
			}
			if len(matches) >= 100 {
				break
			}
		}
		ui.Text(c, fmt.Sprintf("%d 个结果", len(matches))).FontSize(11).TextColor(t.TextMuted)
		if len(matches) == 0 {
			ui.Text(c, "没有匹配的请求").FontSize(12).TextColor(t.TextMuted).Padding(8)
		} else {
			ui.List(c, &r.globalList, len(matches), func(i int) {
				req := matches[i]
				btn := ui.ButtonBase(c).Label(req.Method+" "+req.Name).Justify(ui.Start).
					Padding(4, 8).Radius(5)
				if btn.Hovered() {
					btn.Background(t.SurfaceHover)
				}
				btn.Children(func() {
					ui.Text(c, req.Method).FontSize(10).Bold().TextColor(methodColor(c, req.Method)).Width(44)
					ui.Column(c).Grow(1).Children(func() {
						ui.Text(c, req.Name).FontSize(12.5).SingleLine()
						ui.Text(c, req.URL).FontSize(10.5).TextColor(t.TextMuted).SingleLine()
					})
				})
				if btn.Clicked() {
					r.searchOpen = false
					r.openRequest(req.ID)
				}
			}).Height(380)
		}
		if input.Submitted() && len(matches) > 0 {
			r.searchOpen = false
			r.openRequest(matches[0].ID)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "关闭").Clicked() {
				r.searchOpen = false
			}
		})
	}).Width(760)
}
