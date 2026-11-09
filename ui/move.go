package ui

import (
	"sort"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

type moveDest struct {
	CollectionID string
	FolderID     string
	Label        string
	Depth        int
}

func (r *Root) beginMove(kind, id, name string) {
	r.moveKind = kind
	r.moveID = id
	r.moveName = name
	r.moveErr = ""
	switch kind {
	case app.NodeRequest:
		if req, ok := r.tree.Request(id); ok {
			r.moveCollection = req.CollectionID
			r.moveFolder = req.FolderID
		}
	case app.NodeFolder:
		if f, ok := r.folderByID(id); ok {
			r.moveCollection = f.CollectionID
			r.moveFolder = f.ParentID
		}
	}
	r.moveOpen = true
}

func (r *Root) currentParent() (string, string) {
	switch r.moveKind {
	case app.NodeRequest:
		if req, ok := r.tree.Request(r.moveID); ok {
			return req.CollectionID, req.FolderID
		}
	case app.NodeFolder:
		if f, ok := r.folderByID(r.moveID); ok {
			return f.CollectionID, f.ParentID
		}
	}
	return "", ""
}

func (r *Root) folderSubtree(rootID string) map[string]bool {
	out := map[string]bool{rootID: true}
	for changed := true; changed; {
		changed = false
		for _, f := range r.tree.Folders {
			if out[f.ParentID] && !out[f.ID] {
				out[f.ID] = true
				changed = true
			}
		}
	}
	return out
}

func (r *Root) moveDestinations() []moveDest {
	skip := map[string]bool{}
	if r.moveKind == app.NodeFolder {
		skip = r.folderSubtree(r.moveID)
	}
	collections := append([]app.Collection(nil), r.tree.Collections...)
	sort.SliceStable(collections, func(i, j int) bool {
		if collections[i].SortOrder != collections[j].SortOrder {
			return collections[i].SortOrder < collections[j].SortOrder
		}
		return collections[i].Name < collections[j].Name
	})
	byParent := map[string][]app.Folder{}
	for _, f := range r.tree.Folders {
		byParent[f.ParentID] = append(byParent[f.ParentID], f)
	}
	for k := range byParent {
		sort.SliceStable(byParent[k], func(i, j int) bool {
			if byParent[k][i].SortOrder != byParent[k][j].SortOrder {
				return byParent[k][i].SortOrder < byParent[k][j].SortOrder
			}
			return byParent[k][i].Name < byParent[k][j].Name
		})
	}
	var out []moveDest
	for _, col := range collections {
		out = append(out, moveDest{
			CollectionID: col.ID,
			Label:        col.Name + "（根）",
			Depth:        0,
		})
		var walk func(parentID, path string, depth int)
		walk = func(parentID, path string, depth int) {
			for _, f := range byParent[parentID] {
				if skip[f.ID] {
					continue
				}
				label := f.Name
				if path != "" {
					label = path + " / " + f.Name
				}
				out = append(out, moveDest{
					CollectionID: col.ID,
					FolderID:     f.ID,
					Label:        col.Name + " / " + label,
					Depth:        depth,
				})
				walk(f.ID, label, depth+1)
			}
		}
		walk("", "", 1)
	}
	return out
}

func (r *Root) moveModal(c *ui.Context) {
	if !r.moveOpen {
		return
	}
	t := c.Theme()
	dests := r.moveDestinations()
	currentCollection, currentFolder := r.currentParent()
	valid := r.moveCollection != "" &&
		(r.moveCollection != currentCollection || r.moveFolder != currentFolder) &&
		len(dests) > 0
	ui.Modal(c, &r.moveOpen, func() {
		ui.Text(c, "移动「"+r.moveName+"」").FontSize(15).Bold()
		ui.Text(c, "选择目标集合或文件夹；文件夹不能移动到自身或子文件夹。").
			FontSize(11.5).TextColor(t.TextMuted)
		if len(dests) == 0 {
			ui.Text(c, "没有可用的目标").FontSize(12).TextColor(t.TextMuted)
		} else {
			ui.Scroll(c).Height(380).Children(func() {
				for _, d := range dests {
					isCurrent := d.CollectionID == currentCollection && d.FolderID == currentFolder
					selected := d.CollectionID == r.moveCollection && d.FolderID == r.moveFolder
					btn := ui.ButtonBase(c).Label(d.Label).Justify(ui.Start).
						Padding(3, 8).PaddingStart(8 + float32(d.Depth)*14).Radius(5)
					if selected {
						btn.Background(t.Accent.Alpha(0.14))
					} else if btn.Hovered() {
						btn.Background(t.SurfaceHover)
					}
					btn.Children(func() {
						ui.Text(c, d.Label).FontSize(12).SingleLine().Grow(1)
						if isCurrent {
							ui.Text(c, "当前").FontSize(10).TextColor(t.TextMuted)
						}
					})
					if btn.Clicked() {
						r.moveCollection = d.CollectionID
						r.moveFolder = d.FolderID
					}
				}
			})
		}
		if r.moveErr != "" {
			ui.Text(c, r.moveErr).FontSize(12).TextColor(t.Danger)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				r.moveOpen = false
			}
			if ui.PrimaryButton(c, "移动").Disabled(!valid).Clicked() {
				r.applyMove()
			}
		})
	}).Width(680).Height(560)
}

func (r *Root) applyMove() {
	if err := r.store.MoveNodeTo(r.moveKind, r.moveID, r.moveCollection, r.moveFolder); err != nil {
		r.moveErr = "移动失败: " + err.Error()
		return
	}
	r.expandPath(r.moveCollection, r.moveFolder)
	kind, id := r.moveKind, r.moveID
	r.moveOpen = false
	r.moveErr = ""
	r.reloadTree()
	r.selected = selectionID(kind, id)
	if kind == app.NodeRequest {
		r.status = "已移动请求「" + r.moveName + "」"
	} else {
		r.status = "已移动文件夹「" + r.moveName + "」"
	}
}
