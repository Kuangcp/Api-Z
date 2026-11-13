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
	if len(out) > app.MaxRecentRequests {
		out = out[:app.MaxRecentRequests]
	}
	r.mru = out
	r.saveRecentRequests()
}

func (r *Root) loadRecentRequests() {
	ids := app.LoadRecentRequests(r.recentPath)
	kept := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := r.tree.Request(id); ok {
			kept = append(kept, id)
		}
	}
	r.mru = kept
	if len(kept) != len(ids) {
		r.saveRecentRequests()
	}
}

func (r *Root) saveRecentRequests() {
	if r.recentPath == "" {
		return
	}
	exists := make(map[string]bool, len(r.tree.Requests))
	for _, req := range r.tree.Requests {
		exists[req.ID] = true
	}
	kept := make([]string, 0, len(r.mru))
	for _, id := range r.mru {
		if exists[id] {
			kept = append(kept, id)
		}
	}
	r.mru = kept
	_ = app.SaveRecentRequests(r.recentPath, r.mru)
}

func (r *Root) removeMRU(id string) {
	out := r.mru[:0]
	for _, v := range r.mru {
		if v != id {
			out = append(out, v)
		}
	}
	r.mru = out
	r.saveRecentRequests()
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
			r.focusRequest(id)
			r.touchMRU(id)
			r.rememberActiveRequest()
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
		if q == "" {
			for _, id := range r.mru {
				if req, ok := r.tree.Request(id); ok {
					matches = append(matches, req)
				}
				if len(matches) >= app.MaxRecentRequests {
					break
				}
			}
		} else {
			for _, req := range r.tree.Requests {
				if strings.Contains(strings.ToLower(req.Name), q) ||
					strings.Contains(strings.ToLower(req.URL), q) ||
					strings.Contains(strings.ToLower(req.Method), q) {
					matches = append(matches, req)
				}
				if len(matches) >= 100 {
					break
				}
			}
		}
		label := fmt.Sprintf("%d 个结果", len(matches))
		if q == "" {
			label = fmt.Sprintf("最近请求（%d / %d）", len(matches), app.MaxRecentRequests)
		}
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		if len(matches) == 0 {
			msg := "没有匹配的请求"
			if q == "" {
				msg = "暂无最近请求"
			}
			ui.Text(c, msg).FontSize(12).TextColor(t.TextMuted).Padding(8)
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
					r.revealRequestInTree(req.ID)
				}
			}).Height(380)
		}
		if input.Submitted() && len(matches) > 0 {
			r.searchOpen = false
			r.openRequest(matches[0].ID)
			r.revealRequestInTree(matches[0].ID)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "关闭").Clicked() {
				r.searchOpen = false
			}
		})
	}).Width(760)
}
