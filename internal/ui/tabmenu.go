package ui

import (
	"fmt"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// tabContextMenuItems builds the per-tab right-click menu: Save, then the
// close variants. "Close Others" is disabled on a lone tab and "Close to the
// Right" on the last one, so the menu reads the same on every tab.
func (m *MainUI) tabContextMenuItems(t *openTab) []*fyne.MenuItem {
	save := fyne.NewMenuItem("Save", func() { m.saveTab(t) })
	closeItem := fyne.NewMenuItem("Close", func() { m.requestCloseTab(t) })
	others := fyne.NewMenuItem("Close Others", func() { m.requestCloseTabs(m.tabsOtherThan(t), t) })
	others.Disabled = len(m.tabs) <= 1
	right := fyne.NewMenuItem("Close to the Right", func() { m.requestCloseTabs(m.tabsRightOf(t), t) })
	right.Disabled = len(m.tabsRightOf(t)) == 0
	return []*fyne.MenuItem{save, fyne.NewMenuItemSeparator(), closeItem, others, right}
}

// showTabContextMenu pops the tab's context menu up at the pointer. It is the
// requestTab's secondary-tap (right-click / long-press) handler.
func (m *MainUI) showTabContextMenu(t *openTab, e *fyne.PointEvent) {
	rt := m.tabWidgets[t]
	if rt == nil || m.tabIndexOf(t) < 0 {
		return
	}
	canv := fyne.CurrentApp().Driver().CanvasForObject(rt)
	if canv == nil {
		return
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", m.tabContextMenuItems(t)...), canv, e.AbsolutePosition)
}

// saveTab saves t's request. saveRequest works on the bound editor (it flushes
// the debounced body, prunes rows, and restores the URL-fold baseline keyed by
// the current node), so a non-active tab is activated first — the strip
// switches to it, which is also the tab the user then sees being saved.
func (m *MainUI) saveTab(t *openTab) {
	i := m.tabIndexOf(t)
	if i < 0 {
		return
	}
	if i != m.activeTabIdx {
		m.activateTab(i)
		if m.tabIndexOf(t) < 0 {
			return // the request vanished; activateTab closed the tab
		}
	}
	m.saveRequest()
}

// tabsOtherThan returns every open tab except t, in strip order.
func (m *MainUI) tabsOtherThan(t *openTab) []*openTab {
	out := make([]*openTab, 0, len(m.tabs))
	for _, ot := range m.tabs {
		if ot != t {
			out = append(out, ot)
		}
	}
	return out
}

// tabsRightOf returns the tabs after t in strip order; nil when t is last or
// not open.
func (m *MainUI) tabsRightOf(t *openTab) []*openTab {
	i := m.tabIndexOf(t)
	if i < 0 || i+1 >= len(m.tabs) {
		return nil
	}
	return slices.Clone(m.tabs[i+1:])
}

// requestCloseTabs is the bulk entry point behind "Close Others" / "Close to
// the Right". Like requestCloseTab it confirms — once, for the whole set — when
// any victim is a scratch tab with content (that work would be lost), then
// closes them all via closeTabs. Tree-backed tabs close silently: their edits
// live in the tree and the quit guard catches them at exit.
func (m *MainUI) requestCloseTabs(victims []*openTab, anchor *openTab) {
	n := 0
	for _, v := range victims {
		if v.scratch && scratchHasContent(v.scratchReq) {
			n++
		}
	}
	if n > 0 && m.win != nil {
		msg := fmt.Sprintf("%d unsaved requests will be lost.", n)
		if n == 1 {
			msg = "1 unsaved request will be lost."
		}
		dialog.ShowConfirm("Discard tabs?", msg, func(yes bool) {
			if yes {
				m.closeTabs(victims, anchor)
			}
		}, m.win)
		return
	}
	m.closeTabs(victims, anchor)
}

// closeTabs removes every victim from the strip in one pass. anchor is the tab
// the gesture was invoked on: it is never closed, and it takes over as the
// active tab when the active one was among the victims — so the editor rebinds
// once, rather than walking neighbour-by-neighbour through closeTab (which
// would loadRequest once per intermediate activation). Victims no longer open
// (closed while a confirm was up) are skipped; a no-op when nothing remains to
// close or the anchor itself is gone.
func (m *MainUI) closeTabs(victims []*openTab, anchor *openTab) {
	if m.tabIndexOf(anchor) < 0 {
		return
	}
	drop := make(map[*openTab]bool, len(victims))
	var freed int
	for _, v := range victims {
		if v != anchor && !drop[v] && m.tabIndexOf(v) >= 0 {
			drop[v] = true
			freed += cachedBodyLen(v)
		}
	}
	if len(drop) == 0 {
		return
	}
	// The dropped tabs' cached responses go with them; reclaim once the strip
	// has been rebuilt (same policy as closeTab / closeAllTabs).
	defer reclaimAfterLargeBody(freed)
	active := m.activeTab()
	kept := make([]*openTab, 0, len(m.tabs)-len(drop))
	for _, t := range m.tabs {
		if !drop[t] {
			kept = append(kept, t)
		}
	}
	m.tabs = kept // a fresh slice: nothing pins the dropped tabs' responses
	if active == nil || drop[active] {
		m.activateTab(m.tabIndexOf(anchor)) // rebuilds the strip + persists
		return
	}
	m.activeTabIdx = m.tabIndexOf(active)
	m.rebuildTabBar()
	m.persistTabs()
}
