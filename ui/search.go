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

// recentRequests returns the up-to-20 most recently used requests that
// still exist, in MRU order.
func (r *Root) recentRequests() []app.Request {
	out := make([]app.Request, 0, 20)
	for _, id := range r.mru {
		if req, ok := r.tree.Request(id); ok {
			out = append(out, req)
			if len(out) == 20 {
				break
			}
		}
	}
	return out
}

func (r *Root) openRecentOverlay(reverse bool) {
	items := r.recentRequests()
	if len(items) == 0 {
		r.status = "暂无最近请求"
		return
	}
	r.recentSel = 1
	if reverse || len(items) == 1 {
		r.recentSel = len(items) - 1
	}
	r.recentList.ScrollIntoView(r.recentSel)
	r.recentScroll = true
	r.recentOpen = true
}

func (r *Root) cycleRecent(delta int) {
	items := r.recentRequests()
	if len(items) == 0 {
		r.recentOpen = false
		return
	}
	r.recentSel = (r.recentSel + delta + len(items)) % len(items)
	r.recentList.ScrollIntoView(r.recentSel)
	r.recentScroll = true
}

func (r *Root) openRecentSelection() {
	items := r.recentRequests()
	if r.recentSel < 0 || r.recentSel >= len(items) {
		r.recentOpen = false
		return
	}
	id := items[r.recentSel].ID
	r.recentOpen = false
	r.openRequest(id)
	r.focusRequest(id)
}

func (r *Root) recentOverlay(c *ui.Context) {
	if !r.recentOpen {
		return
	}
	t := c.Theme()
	items := r.recentRequests()
	ui.Modal(c, &r.recentOpen, func() {
		// Context.Shortcut ignores chords from outside the modal, so the
		// overlay registers its own window shortcuts while it is visible.
		c.OnWindowShortcut(ui.Ctrl, ui.KeyTab, func() { r.cycleRecent(1) })
		c.OnWindowShortcut(ui.Ctrl|ui.Shift, ui.KeyTab, func() { r.cycleRecent(-1) })
		ui.Text(c, "最近请求").FontSize(15).Bold()
		ui.Text(c, "Ctrl+Tab 下一个 · Shift+Ctrl+Tab 上一个 · Enter 打开 · Esc 关闭").
			FontSize(11).TextColor(t.TextMuted)
		if len(items) == 0 {
			ui.Text(c, "暂无最近请求").FontSize(12).TextColor(t.TextMuted).Padding(8)
			return
		}
		if r.recentSel >= len(items) {
			r.recentSel = len(items) - 1
		}
		if r.recentScroll {
			r.recentList.ScrollIntoView(r.recentSel)
			r.recentScroll = false
		}
		ui.List(c, &r.recentList, len(items), func(i int) {
			req := items[i]
			r.requestResultRow(c, req, i == r.recentSel, func() {
				r.recentSel = i
				r.openRecentSelection()
			})
		}).Height(360)
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "关闭").Clicked() {
				r.recentOpen = false
			}
		})
	}).Width(700).Height(500)
}

func (r *Root) requestResultRow(c *ui.Context, req app.Request, selected bool, onClick func()) {
	t := c.Theme()
	btn := ui.ButtonBase(c).Label(req.Method+" "+req.Name).Justify(ui.Start).
		Padding(4, 8).Radius(5)
	if selected {
		btn.Background(t.Accent.Alpha(0.16))
	} else if btn.Hovered() {
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
		onClick()
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
				r.requestResultRow(c, req, false, func() {
					r.searchOpen = false
					r.openRequest(req.ID)
					r.revealRequestInTree(req.ID)
				})
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
