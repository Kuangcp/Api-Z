package ui

import (
	"sort"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

type treeDrag struct {
	Kind string
	ID   string
}

type dropSpot int

const (
	dropNone dropSpot = iota
	dropBefore
	dropAfter
	dropInto
)

func attachTreeDrag(row ui.Element, kind, id string) {
	if kind == app.NodeCollection {
		return
	}
	row.Drag(treeDrag{Kind: kind, ID: id})
	if row.Dragging() {
		row.Opacity(0.45)
	}
}

func (r *Root) attachTreeDrop(c *ui.Context, row ui.Element, target treeRow) {
	if drag, over := ui.DragOver[treeDrag](row); over {
		if spot := r.treeDropSpot(row, target, drag); spot != dropNone {
			styleTreeDrop(row, spot, c.Theme())
		}
	}
	if drag, ok := ui.Drop[treeDrag](row); ok {
		if spot := r.treeDropSpot(row, target, drag); spot != dropNone {
			r.applyTreeDrop(drag, target, spot)
		}
	}
}

func (r *Root) treeDropSpot(row ui.Element, target treeRow, drag treeDrag) dropSpot {
	kind, id := r.nodeIdentity(target)
	if id == "" || drag.ID == id {
		return dropNone
	}
	switch kind {
	case app.NodeCollection:
		if drag.Kind == app.NodeFolder || drag.Kind == app.NodeRequest {
			return dropInto
		}
	case app.NodeFolder:
		if drag.Kind == app.NodeRequest {
			return dropInto
		}
		if drag.Kind != app.NodeFolder || r.folderSubtree(drag.ID)[id] {
			return dropNone
		}
		_, y, ok := row.PointerPosition()
		if !ok {
			return dropNone
		}
		h := row.Bounds().H
		if h <= 0 {
			return dropNone
		}
		switch {
		case y < h*0.28:
			return dropBefore
		case y > h*0.72:
			return dropAfter
		default:
			return dropInto
		}
	case app.NodeRequest:
		if drag.Kind != app.NodeRequest {
			return dropNone
		}
		_, y, ok := row.PointerPosition()
		if !ok {
			return dropNone
		}
		h := row.Bounds().H
		if h <= 0 {
			return dropNone
		}
		if y < h/2 {
			return dropBefore
		}
		return dropAfter
	}
	return dropNone
}

func styleTreeDrop(row ui.Element, spot dropSpot, t *ui.Theme) {
	switch spot {
	case dropInto:
		row.Background(t.Accent.Alpha(0.12))
	case dropBefore:
		row.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, t.Accent, 1)
		})
	case dropAfter:
		row.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 2, W: r.W, H: 2}, t.Accent, 1)
		})
	}
}

func (r *Root) applyTreeDrop(drag treeDrag, target treeRow, spot dropSpot) {
	targetKind, targetID := r.nodeIdentity(target)
	var err error
	switch targetKind {
	case app.NodeCollection:
		if spot != dropInto {
			return
		}
		err = r.store.MoveNodeTo(drag.Kind, drag.ID, targetID, "")
		r.expanded["c:"+targetID] = true
	case app.NodeFolder:
		folder, ok := r.folderByID(targetID)
		if !ok {
			return
		}
		switch spot {
		case dropInto:
			err = r.store.MoveNodeTo(drag.Kind, drag.ID, folder.CollectionID, folder.ID)
			r.expandPath(folder.CollectionID, folder.ID)
		case dropBefore:
			err = r.store.MoveNodeBefore(drag.Kind, drag.ID, folder.CollectionID, folder.ParentID, folder.ID)
			r.expandPath(folder.CollectionID, folder.ParentID)
		case dropAfter:
			before := r.nextSiblingID(app.NodeFolder, folder.CollectionID, folder.ParentID, folder.ID)
			err = r.store.MoveNodeBefore(drag.Kind, drag.ID, folder.CollectionID, folder.ParentID, before)
			r.expandPath(folder.CollectionID, folder.ParentID)
		}
	case app.NodeRequest:
		req, ok := r.tree.Request(targetID)
		if !ok {
			return
		}
		before := req.ID
		if spot == dropAfter {
			before = r.nextSiblingID(app.NodeRequest, req.CollectionID, req.FolderID, req.ID)
		}
		err = r.store.MoveNodeBefore(drag.Kind, drag.ID, req.CollectionID, req.FolderID, before)
		r.expandPath(req.CollectionID, req.FolderID)
	}
	if err != nil {
		r.err = "移动失败: " + err.Error()
		return
	}
	r.reloadTree()
	r.selected = selectionID(drag.Kind, drag.ID)
	if drag.Kind == app.NodeRequest {
		r.status = "已移动请求"
	} else {
		r.status = "已移动文件夹"
	}
}

func (r *Root) nextSiblingID(kind, collectionID, parentID, id string) string {
	var ids []string
	switch kind {
	case app.NodeFolder:
		for _, f := range r.tree.Folders {
			if f.CollectionID == collectionID && f.ParentID == parentID {
				ids = append(ids, f.ID)
			}
		}
		sort.SliceStable(ids, func(i, j int) bool { return r.folderOrder(ids[i]) < r.folderOrder(ids[j]) })
	case app.NodeRequest:
		for _, req := range r.tree.Requests {
			if req.CollectionID == collectionID && req.FolderID == parentID {
				ids = append(ids, req.ID)
			}
		}
		sort.SliceStable(ids, func(i, j int) bool { return r.requestOrder(ids[i]) < r.requestOrder(ids[j]) })
	}
	for i, v := range ids {
		if v == id && i+1 < len(ids) {
			return ids[i+1]
		}
	}
	return ""
}

func (r *Root) folderOrder(id string) int {
	for _, f := range r.tree.Folders {
		if f.ID == id {
			return f.SortOrder
		}
	}
	return 0
}

func (r *Root) requestOrder(id string) int {
	for _, req := range r.tree.Requests {
		if req.ID == id {
			return req.SortOrder
		}
	}
	return 0
}
