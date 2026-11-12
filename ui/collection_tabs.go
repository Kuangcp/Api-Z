package ui

import (
	"sort"

	"github.com/egoist/mygo/ui"

	"api-z/app"
)

const (
	collectionTabMaxWidth = 120
	collectionTabPadding  = 16
	collectionTabGap      = 2
	collectionOverflowW   = 30
)

// sortedCollections returns the collections in the same order as the tree.
func (r *Root) sortedCollections() []app.Collection {
	cols := append([]app.Collection(nil), r.tree.Collections...)
	sort.SliceStable(cols, func(i, j int) bool {
		if cols[i].SortOrder != cols[j].SortOrder {
			return cols[i].SortOrder < cols[j].SortOrder
		}
		return cols[i].Name < cols[j].Name
	})
	return cols
}

// ensureActiveCollection keeps activeCollection pointing at an existing
// collection: the current one when it still exists, else the most recently
// used one, else the first collection.
func (r *Root) ensureActiveCollection() {
	cols := r.sortedCollections()
	if len(cols) == 0 {
		r.activeCollection = ""
		r.colMRU = nil
		return
	}
	exists := make(map[string]bool, len(cols))
	for _, col := range cols {
		exists[col.ID] = true
	}
	if exists[r.activeCollection] {
		r.pruneCollectionMRU(exists)
		return
	}
	kept := make([]string, 0, len(r.colMRU))
	for _, id := range r.colMRU {
		if exists[id] {
			kept = append(kept, id)
		}
	}
	r.colMRU = kept
	if len(kept) > 0 {
		r.activeCollection = kept[0]
		return
	}
	r.activeCollection = cols[0].ID
}

func (r *Root) pruneCollectionMRU(exists map[string]bool) {
	kept := r.colMRU[:0]
	for _, id := range r.colMRU {
		if exists[id] {
			kept = append(kept, id)
		}
	}
	r.colMRU = kept
}

// activateCollection switches the sidebar tree to a collection and marks it
// most recently used, so it stays visible when the tab strip overflows.
func (r *Root) activateCollection(id string) {
	if id == "" {
		return
	}
	if _, ok := r.tree.Collection(id); !ok {
		return
	}
	r.activeCollection = id
	r.selected = "c:" + id
	out := make([]string, 0, len(r.colMRU)+1)
	out = append(out, id)
	for _, v := range r.colMRU {
		if v != id {
			out = append(out, v)
		}
	}
	if len(out) > 30 {
		out = out[:30]
	}
	r.colMRU = out
}

func collectionTabWidth(name string) float32 {
	w := float32(0)
	for _, ch := range name {
		if ch > 127 {
			w += 11.5
		} else {
			w += 6.6
		}
	}
	if w > collectionTabMaxWidth {
		w = collectionTabMaxWidth
	}
	return w + collectionTabPadding
}

// collectionTabs splits the collections into the tabs that fit the sidebar
// and the rest, which the overflow menu offers. The active collection always
// stays visible; the remaining slots go to the most recently used ones.
func (r *Root) collectionTabs() (visible, hidden []app.Collection) {
	cols := r.sortedCollections()
	if len(cols) == 0 {
		return nil, nil
	}
	budget := r.sidebarWidth - 14
	widths := make(map[string]float32, len(cols))
	total := float32(0)
	for _, col := range cols {
		w := collectionTabWidth(col.Name)
		widths[col.ID] = w
		total += w + collectionTabGap
	}
	if total <= budget {
		return cols, nil
	}
	budget -= collectionOverflowW
	if budget < collectionTabWidth("…") {
		budget = collectionTabWidth("…")
	}

	order := make([]string, 0, len(cols)+len(r.colMRU))
	seen := make(map[string]bool, len(cols))
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			order = append(order, id)
		}
	}
	add(r.activeCollection)
	for _, id := range r.colMRU {
		add(id)
	}
	for _, col := range cols {
		add(col.ID)
	}

	chosen := make(map[string]bool, len(cols))
	used := float32(0)
	for i, id := range order {
		w, ok := widths[id]
		if !ok {
			continue
		}
		// The active collection shows even when it alone exceeds the width.
		if i > 0 && used+w+collectionTabGap > budget {
			continue
		}
		chosen[id] = true
		used += w + collectionTabGap
	}

	for _, col := range cols {
		if chosen[col.ID] {
			visible = append(visible, col)
		} else {
			hidden = append(hidden, col)
		}
	}
	return visible, hidden
}

func (r *Root) collectionTabBar(c *ui.Context) {
	t := c.Theme()
	visible, hidden := r.collectionTabs()
	if len(visible) == 0 {
		ui.Text(c, "还没有集合").FontSize(11.5).TextColor(t.TextMuted).Padding(2, 4)
		return
	}
	ui.Row(c).Gap(collectionTabGap).Children(func() {
		for _, col := range visible {
			col := col
			active := col.ID == r.activeCollection
			btn := ui.ButtonBase(c).Label(col.Name).Justify(ui.Start).
				Padding(3, 8).Radius(5)
			if active {
				btn.Background(t.Accent.Alpha(0.16))
			} else if btn.Hovered() {
				btn.Background(t.SurfaceHover)
			}
			btn.Children(func() {
				txt := ui.Text(c, col.Name).FontSize(12).SingleLine().
					MaxWidth(collectionTabMaxWidth).Tooltip(col.Name)
				if active {
					txt.Bold()
				}
			})
			if btn.Clicked() {
				r.activateCollection(col.ID)
			}
			btn.ContextMenu(func(m *ui.Menu) {
				r.collectionTabMenu(c, m, col.ID)
			})
		}
		if len(hidden) > 0 {
			iconMenuButton(c, iconChevronDown, "更多集合", func(m *ui.Menu) {
				for _, col := range hidden {
					col := col
					if m.Item(col.Name).Chosen() {
						r.activateCollection(col.ID)
					}
				}
			})
		}
	})
}

func (r *Root) collectionTabMenu(c *ui.Context, m *ui.Menu, id string) {
	col, ok := r.tree.Collection(id)
	if !ok {
		return
	}
	r.treeContextMenu(c, m, treeRow{kind: rowCollection, id: "c:" + id, name: col.Name})
}
