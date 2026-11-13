package ui

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

var envColorPresets = []struct{ label, hex string }{
	{"自动", ""},
	{"蓝", "#3b82f6"},
	{"绿", "#22c55e"},
	{"红", "#ef4444"},
	{"橙", "#f59e0b"},
	{"紫", "#a855f7"},
}

const envInfoColorHex = "#3b82f6"

func envTextColor(c *ui.Context, env app.Environment) ui.Color {
	t := c.Theme()
	if app.ValidHex(env.Color) {
		return ui.Hex(env.Color)
	}
	switch app.EnvironmentColorHint(env.Name, env.Tag) {
	case "red":
		return t.Danger
	case "green":
		return t.Success
	case "blue":
		return ui.Hex(envInfoColorHex)
	}
	return t.Text
}

type envGroup struct {
	Tag  string
	Envs []app.Environment
}

// envGroups returns environments grouped by tag; the untagged group is
// last, matching the environment manager and api-x's cascade menu.
func (r *Root) envGroups() []envGroup {
	index := map[string]int{}
	var groups []envGroup
	for _, env := range r.env.Environments {
		tag := strings.TrimSpace(env.Tag)
		if tag == "" {
			tag = "其他"
		}
		i, ok := index[tag]
		if !ok {
			i = len(groups)
			index[tag] = i
			groups = append(groups, envGroup{Tag: tag})
		}
		groups[i].Envs = append(groups[i].Envs, env)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Tag == "其他" {
			return false
		}
		if groups[j].Tag == "其他" {
			return true
		}
		return groups[i].Tag < groups[j].Tag
	})
	return groups
}

// envNameWithoutTag strips a repeated tag prefix from an environment name
// ("文献 - Local" + "文献" -> "Local"), like api-x.
func envNameWithoutTag(name, tag string) string {
	tag = strings.TrimSpace(tag)
	name = strings.TrimSpace(name)
	if tag == "" || !strings.HasPrefix(name, tag) {
		return name
	}
	rest := strings.TrimLeft(strings.TrimPrefix(name, tag), " -— _·|")
	if rest == "" {
		return name
	}
	return rest
}

func (r *Root) activeEnvGroup() int {
	active := r.env.ActiveEnvironmentID
	for i, g := range r.envGroups() {
		for _, env := range g.Envs {
			if env.ID == active {
				return i
			}
		}
	}
	return 0
}

func (r *Root) activateEnvID(id string) {
	r.env.ActiveEnvironmentID = id
	r.envName = ""
	for _, env := range r.env.Environments {
		if env.ID == id {
			r.envName = env.Name
			break
		}
	}
	_ = r.store.SaveEnvironments(r.env)
}

// envMenu is the title-bar environment selector: tag | name on the button,
// and a two-column cascade menu (tags left, environments right).
const (
	envMenuTagsWidth float32 = 150
	envMenuGap       float32 = 4
)

// envMenuColumnWidth sizes the environment column to its longest name, so
// short groups do not leave a wide empty strip on the right.
func envMenuColumnWidth(envs []app.Environment, tag string) float32 {
	longest := float32(0)
	for _, env := range envs {
		label := envNameWithoutTag(env.Name, tag)
		w := float32(0)
		for _, ch := range label {
			if ch > 127 {
				w += 13
			} else {
				w += 7.5
			}
		}
		if w > longest {
			longest = w
		}
	}
	w := longest + 16 + 20 + 6
	if w < 120 {
		w = 120
	}
	if w > 240 {
		w = 240
	}
	return w
}

func (r *Root) envMenu(c *ui.Context) {
	t := c.Theme()
	tag, name := "", "无环境"
	nameColor := t.Text
	if active := r.env.Active(); active != nil {
		tag = strings.TrimSpace(active.Tag)
		name = envNameWithoutTag(active.Name, tag)
		if name == "" {
			name = "无环境"
		}
		nameColor = envTextColor(c, *active)
	}
	anchor := ui.ButtonBase(c).Label("切换环境").Justify(ui.Start).
		Padding(3, 8).Radius(6)
	if anchor.Hovered() {
		anchor.Background(t.SurfaceHover)
	}
	anchor.Children(func() {
		if tag != "" {
			ui.Text(c, tag).FontSize(11).TextColor(t.TextMuted).SingleLine().MaxWidth(90)
			ui.Text(c, "|").FontSize(11).TextColor(t.TextMuted)
		}
		ui.Text(c, name).FontSize(11.5).SingleLine().MaxWidth(150).TextColor(nameColor)
		ui.Box(c).Width(4)
		ui.Text(c, "▾").FontSize(9).TextColor(t.TextMuted)
	})
	if anchor.Clicked() {
		if !r.envMenuOpen {
			r.envGroup = r.activeEnvGroup()
		}
		r.envMenuOpen = !r.envMenuOpen
	}
	ui.PopoverBase(c, anchor, &r.envMenuOpen, func(panel ui.Element) {
		// Align the environment column under the title-bar selector: shift
		// the whole panel left by the tag column so the right column starts
		// where the button does.
		panel.AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).
			MarginStart(-(envMenuTagsWidth + envMenuGap + 8))
		panel.Background(t.Surface).Border(1, t.Border).Radius(8).Padding(4).
			Children(func() {
				ui.Row(c).Gap(envMenuGap).AlignItems(ui.Start).Children(func() {
					ui.Column(c).Width(envMenuTagsWidth).Children(func() {
						ui.Scroll(c).MaxHeight(320).Children(func() {
							groups := r.envGroups()
							if len(groups) == 0 {
								ui.Text(c, "还没有环境").FontSize(12).TextColor(t.TextMuted).Padding(6, 8)
							}
							for i, group := range groups {
								i, group := i, group
								btn := ui.ButtonBase(c).Label(group.Tag).Justify(ui.Start).
									Padding(4, 8).Radius(5)
								if i == r.envGroup || btn.Hovered() {
									btn.Background(t.Accent.Alpha(0.12))
									r.envGroup = i
								}
								btn.Children(func() {
									ui.Text(c, group.Tag).FontSize(12).Bold().SingleLine().Grow(1)
									ui.Text(c, "›").FontSize(11).TextColor(t.TextMuted)
								})
								if btn.Clicked() && len(group.Envs) > 0 {
									r.activateEnvID(group.Envs[0].ID)
									r.envMenuOpen = false
								}
								if i < len(groups)-1 {
									ui.Box(c).Height(1).Margin(4, 2).Background(t.Border)
								}
							}
						})
						ui.Box(c).Height(1).Margin(4, 2).Background(t.Border)
						noEnv := ui.ButtonBase(c).Label("无环境").Justify(ui.Start).
							Padding(4, 8).Radius(5).FillWidth()
						noEnv.Children(func() { ui.Text(c, "无环境").FontSize(12.5).SingleLine() })
						if noEnv.Hovered() {
							noEnv.Background(t.SurfaceHover)
						}
						if noEnv.Clicked() {
							r.activateEnvID("")
							r.envMenuOpen = false
						}
						manage := ui.ButtonBase(c).Label("管理环境…").Justify(ui.Start).
							Padding(4, 8).Radius(5).FillWidth()
						manage.Children(func() { ui.Text(c, "管理环境…").FontSize(12.5).SingleLine() })
						if manage.Hovered() {
							manage.Background(t.SurfaceHover)
						}
						if manage.Clicked() {
							r.envMenuOpen = false
							r.openEnvManager()
						}
					})
					if r.envGroup >= 0 {
						groups := r.envGroups()
						if r.envGroup < len(groups) {
							group := groups[r.envGroup]
							ui.Column(c).Width(envMenuColumnWidth(group.Envs, group.Tag)).Children(func() {
								ui.Scroll(c).MaxHeight(320).Children(func() {
									for _, env := range group.Envs {
										env := env
										active := env.ID == r.env.ActiveEnvironmentID
										label := envNameWithoutTag(env.Name, group.Tag)
										if label == "" {
											label = "(未命名)"
										}
										btn := ui.ButtonBase(c).Label(label).Justify(ui.Start).
											Padding(7, 10).Radius(5).FillWidth()
										if btn.Hovered() {
											btn.Background(t.SurfaceHover)
										}
										btn.Children(func() {
											if active {
												ui.Text(c, "✓").FontSize(12).TextColor(t.Accent).Width(16)
											} else {
												ui.Box(c).Width(16)
											}
											ui.Text(c, label).FontSize(13.5).SingleLine().
												TextColor(envTextColor(c, env))
										})
										if btn.Clicked() {
											r.activateEnvID(env.ID)
											r.envMenuOpen = false
										}
									}
								})
							})
						}
					}
				})
			})
	})
}
