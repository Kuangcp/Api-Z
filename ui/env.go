package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

func (r *Root) openEnvManager() {
	r.envDraft = r.env.Clone()
	r.envSel = 0
	for i, e := range r.envDraft.Environments {
		if e.ID == r.env.ActiveEnvironmentID {
			r.envSel = i
		}
	}
	r.envErr = ""
	r.envOpen = true
}

func (r *Root) envManager(c *ui.Context) {
	if !r.envOpen {
		return
	}
	t := c.Theme()
	ui.Modal(c, &r.envOpen, func() {
		ui.Text(c, "环境管理").FontSize(15).Bold()
		ui.Row(c).Gap(14).Children(func() {
			ui.Column(c).Width(250).Children(func() {
				ui.Row(c).Gap(6).Children(func() {
					if ui.Button(c, "新增环境").Clicked() {
						r.envDraft.Environments = append(r.envDraft.Environments, app.Environment{
							ID:   app.NewID(),
							Name: "新环境",
						})
						r.envSel = len(r.envDraft.Environments) - 1
					}
					if ui.Button(c, "删除环境").Disabled(len(r.envDraft.Environments) == 0).Clicked() {
						r.deleteEnv()
					}
				})
				ui.Scroll(c).Height(460).Children(func() {
					r.envList(c)
				})
			})
			ui.Column(c).Grow(1).Children(func() {
				r.envEditor(c, t)
			})
		})
		if r.envErr != "" {
			ui.Text(c, r.envErr).FontSize(12).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				r.envOpen = false
			}
			if ui.PrimaryButton(c, "保存环境").Clicked() {
				r.saveEnvManager()
			}
		})
	}).Width(880).Height(640)
}

func (r *Root) envList(c *ui.Context) {
	t := c.Theme()
	if len(r.envDraft.Environments) == 0 {
		ui.Text(c, "还没有环境").FontSize(12).TextColor(t.TextMuted).Padding(8)
		return
	}
	groups := map[string][]int{}
	var order []string
	for i, e := range r.envDraft.Environments {
		tag := strings.TrimSpace(e.Tag)
		if tag == "" {
			tag = "其他"
		}
		if _, ok := groups[tag]; !ok {
			order = append(order, tag)
		}
		groups[tag] = append(groups[tag], i)
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i] == "其他" {
			return false
		}
		if order[j] == "其他" {
			return true
		}
		return order[i] < order[j]
	})
	for _, tag := range order {
		ui.Text(c, tag).FontSize(10.5).TextColor(t.TextMuted).Padding(4, 8)
		for _, i := range groups[tag] {
			env := r.envDraft.Environments[i]
			active := env.ID == r.envDraft.ActiveEnvironmentID
			btn := ui.ButtonBase(c).Label(env.Name).Justify(ui.Start).Padding(3, 8).Radius(5)
			if i == r.envSel {
				btn.Background(t.Accent.Alpha(0.14))
			} else if btn.Hovered() {
				btn.Background(t.SurfaceHover)
			}
			btn.Children(func() {
				ui.Text(c, env.Name).FontSize(12.5).SingleLine().Grow(1)
				if active {
					ui.Text(c, "当前").FontSize(10).TextColor(t.Success)
				}
				ui.Text(c, fmt.Sprintf("%d", len(env.Variables))).FontSize(10).TextColor(t.TextMuted)
			})
			if btn.Clicked() {
				r.envSel = i
			}
		}
	}
}

func (r *Root) envEditor(c *ui.Context, t *ui.Theme) {
	if len(r.envDraft.Environments) == 0 {
		ui.Text(c, "点击「新增环境」开始").FontSize(12).TextColor(t.TextMuted)
		return
	}
	if r.envSel < 0 || r.envSel >= len(r.envDraft.Environments) {
		r.envSel = 0
	}
	env := &r.envDraft.Environments[r.envSel]
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "名称").Width(56).TextColor(t.TextMuted)
		ui.TextInput(c, &env.Name).Grow(1)
	})
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "标签").Width(56).TextColor(t.TextMuted)
		ui.TextInput(c, &env.Tag).Placeholder("如 Local / Test / Prod").Grow(1)
		if env.ID == r.envDraft.ActiveEnvironmentID {
			ui.Text(c, "当前环境").FontSize(11).TextColor(t.Success)
		} else if ui.Button(c, "设为当前").Clicked() {
			r.envDraft.ActiveEnvironmentID = env.ID
		}
	})
	ui.Text(c, "变量：在请求中用 {{key}} 引用").FontSize(10.5).TextColor(t.TextMuted)
	ui.Row(c).Children(func() {
		ui.Text(c, "Key").FontSize(11).TextColor(t.TextMuted).Width(200)
		ui.Text(c, "Value").FontSize(11).TextColor(t.TextMuted).Grow(1)
		if ui.Button(c, "添加变量").Clicked() {
			env.Variables = append(env.Variables, app.EnvVariable{})
		}
	})
	ui.Scroll(c).Height(260).Children(func() {
		for i := range env.Variables {
			i := i
			ui.Row(c).Gap(6).Children(func() {
				ui.TextInput(c, &env.Variables[i].Key).Placeholder("name").Width(200)
				ui.TextInput(c, &env.Variables[i].Value).Placeholder("value").Grow(1)
				ui.Button(c, "移除").OnClick(func() {
					env.Variables = append(env.Variables[:i], env.Variables[i+1:]...)
				})
			})
		}
		if len(env.Variables) == 0 {
			ui.Text(c, "还没有变量").FontSize(12).TextColor(t.TextMuted).Padding(8)
		}
	})
}

func (r *Root) deleteEnv() {
	if len(r.envDraft.Environments) == 0 {
		return
	}
	removed := r.envDraft.Environments[r.envSel]
	r.envDraft.Environments = append(r.envDraft.Environments[:r.envSel], r.envDraft.Environments[r.envSel+1:]...)
	if r.envDraft.ActiveEnvironmentID == removed.ID {
		if len(r.envDraft.Environments) > 0 {
			r.envDraft.ActiveEnvironmentID = r.envDraft.Environments[0].ID
		} else {
			r.envDraft.ActiveEnvironmentID = ""
		}
	}
	if r.envSel >= len(r.envDraft.Environments) {
		r.envSel = len(r.envDraft.Environments) - 1
	}
	if r.envSel < 0 {
		r.envSel = 0
	}
}

func (r *Root) saveEnvManager() {
	cleaned := r.envDraft.Clean()
	if err := r.store.SaveEnvironments(cleaned); err != nil {
		r.envErr = "保存失败: " + err.Error()
		return
	}
	r.env = cleaned
	r.envNames = envNames(cleaned)
	if active := cleaned.Active(); active != nil {
		r.envName = active.Name
	} else {
		r.envName = ""
	}
	r.envErr = ""
	r.envOpen = false
	r.status = fmt.Sprintf("环境已保存（%d 个）", len(cleaned.Environments))
}
