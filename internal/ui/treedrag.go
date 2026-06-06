package ui

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
)

// indexAppend is a sentinel "drop at the end of the container" index; the
// session's MoveNode clamps it to the destination slice length.
const indexAppend = 1 << 30

// dropPlan is the resolved outcome of hovering a drag over the tree: either a
// collection reorder, a node move, or invalid (nothing happens on release).
// The indicator fields tell the overlay what to draw.
type dropPlan struct {
	valid        bool
	isCollection bool
	// collection reorder:
	fromCol, toCol int
	// node move:
	srcID, dstParent string
	index            int
	// indicator (absolute screen geometry of the target row):
	rowTop, rowH float32
	lineAtBottom bool // draw the insert line at the row's bottom edge, not top
	into         bool // draw the into-container outline instead of a line
}

// dragTreeNode is the per-row Dragged callback: it records the drag, computes
// the live drop plan for the pointer position, and updates the indicators.
func (m *MainUI) dragTreeNode(srcID string, e *fyne.DragEvent) {
	if !m.dragActive {
		m.dragActive = true
		m.dragSrcID = srcID
	}
	m.dragLastAbs = e.AbsolutePosition
	m.showDropIndicator(m.computeDrop(srcID, e.AbsolutePosition))
}

// dropTreeNode is the per-row DragEnd callback: it recomputes the drop plan
// from the last pointer position, applies it, and clears the drag state +
// indicators.
func (m *MainUI) dropTreeNode(srcID string) {
	defer m.endDrag()
	if !m.dragActive {
		return
	}
	m.applyDrop(m.computeDrop(srcID, m.dragLastAbs))
}

func (m *MainUI) endDrag() {
	m.dragActive = false
	m.dragSrcID = ""
	if m.dropIndicator != nil {
		m.dropIndicator.Hide()
	}
	if m.dropInto != nil {
		m.dropInto.Hide()
	}
}

// computeDrop resolves where the dragged node would land if released at abs.
func (m *MainUI) computeDrop(srcID string, abs fyne.Position) dropPlan {
	tgtID, rowTop, rowH, ok := m.rowAt(abs)
	if !ok || tgtID == srcID {
		return dropPlan{}
	}
	topHalf := abs.Y < rowTop+rowH/2

	if isCollectionID(srcID) {
		return m.planCollectionDrop(srcID, tgtID, topHalf, rowTop, rowH)
	}
	return m.planNodeDrop(srcID, tgtID, topHalf, rowTop, rowH)
}

// planCollectionDrop reorders the top-level collection list. A collection can
// only land among collections, so the target resolves to its collection root.
func (m *MainUI) planCollectionDrop(srcID, tgtID string, topHalf bool, rowTop, rowH float32) dropPlan {
	srcCol := collectionIndexOf(srcID)
	tgtCol := collectionIndexOf(tgtID)
	if srcCol < 0 || tgtCol < 0 || srcCol == tgtCol {
		return dropPlan{}
	}
	// Index of the target within the list after the source is removed.
	posT := tgtCol
	if srcCol < tgtCol {
		posT = tgtCol - 1
	}
	to := posT
	if !topHalf {
		to++
	}
	return dropPlan{
		valid: true, isCollection: true, fromCol: srcCol, toCol: to,
		rowTop: rowTop, rowH: rowH, lineAtBottom: !topHalf,
	}
}

// planNodeDrop resolves a folder/request move within the same collection.
func (m *MainUI) planNodeDrop(srcID, tgtID string, topHalf bool, rowTop, rowH float32) dropPlan {
	if collectionIndexOf(srcID) != collectionIndexOf(tgtID) {
		return dropPlan{} // cross-collection not supported
	}
	// Can't drop a folder onto itself or into its own descendant.
	if tgtID == srcID || strings.HasPrefix(tgtID, srcID+"/") {
		return dropPlan{}
	}
	srcKind := nodeKind(srcID)
	tgtBranch := m.sess.Tree().IsBranch(tgtID)
	tgtParent, tgtIdx, ok := splitNode(tgtID)
	if !ok {
		return dropPlan{}
	}

	switch {
	case tgtBranch && !topHalf:
		// Drop INTO the folder/collection (append).
		return dropPlan{valid: true, srcID: srcID, dstParent: tgtID, index: indexAppend,
			rowTop: rowTop, rowH: rowH, into: true}
	case tgtBranch && topHalf && srcKind == "f":
		if isCollectionID(tgtID) {
			// Top half of a collection root → make the folder its first child.
			// splitNode(rootID) yields an empty parent, so target the root
			// itself at index 0 instead of producing dstParent="" (which the
			// move would reject as cross-collection).
			return dropPlan{valid: true, srcID: srcID, dstParent: tgtID, index: 0,
				rowTop: rowTop, rowH: rowH, into: true}
		}
		// A folder before another folder — same (folder) slice, so align indices.
		return dropPlan{valid: true, srcID: srcID, dstParent: tgtParent, index: tgtIdx,
			rowTop: rowTop, rowH: rowH, lineAtBottom: false}
	case tgtBranch && topHalf:
		// A request can't be a folder's sibling; fall back to dropping into it.
		return dropPlan{valid: true, srcID: srcID, dstParent: tgtID, index: indexAppend,
			rowTop: rowTop, rowH: rowH, into: true}
	default:
		// Target is a request (leaf). Same-kind → align to its index; otherwise
		// (folder onto request) append to the request's container.
		idx := indexAppend
		if srcKind == "r" {
			idx = tgtIdx
			if !topHalf {
				idx++
			}
		}
		return dropPlan{valid: true, srcID: srcID, dstParent: tgtParent, index: idx,
			rowTop: rowTop, rowH: rowH, lineAtBottom: !topHalf}
	}
}

// applyDrop performs the planned move and refreshes the tree + tabs.
func (m *MainUI) applyDrop(p dropPlan) {
	if !p.valid {
		return
	}
	if p.isCollection {
		if err := m.sess.MoveCollection(p.fromCol, p.toCol); err != nil {
			m.Status.SetText(err.Error())
			return
		}
		m.Tree.Refresh()
		m.reconcileTabs() // collection reorder shifts every node ID
		m.refreshEnvironments()
		m.Status.SetText("Moved collection")
		return
	}
	newID, err := m.sess.MoveNode(p.srcID, p.dstParent, p.index)
	if err != nil {
		m.Status.SetText(err.Error())
		return
	}
	m.Tree.Refresh()
	m.reconcileTabs() // sibling shifts changed node IDs
	if newID != "" {
		m.Tree.Select(newID)
	}
	m.Status.SetText("Moved")
}

// rowAt returns the node id, plus the absolute top Y and height, of the tree
// row under abs — or ok=false when abs is outside the tree viewport or over no
// row. Only visible rows within the tree's bounds are considered, so stale
// positions of pooled off-screen rows don't produce false hits.
func (m *MainUI) rowAt(abs fyne.Position) (id string, rowTop, rowH float32, ok bool) {
	drv := fyne.CurrentApp().Driver()
	if m.Tree == nil || drv == nil {
		return "", 0, 0, false
	}
	treePos := drv.AbsolutePositionForObject(m.Tree)
	treeSz := m.Tree.Size()
	if abs.Y < treePos.Y || abs.Y >= treePos.Y+treeSz.Height {
		return "", 0, 0, false
	}
	for row, rid := range m.treeRows {
		if rid == "" || !row.Visible() {
			continue
		}
		p := drv.AbsolutePositionForObject(row)
		h := row.Size().Height
		if abs.Y >= p.Y && abs.Y < p.Y+h {
			return rid, p.Y, h, true
		}
	}
	return "", 0, 0, false
}

// showDropIndicator positions the line / into-outline overlays for plan, or
// hides them when the plan is invalid. Geometry is converted from absolute
// screen coordinates to the tree-area-relative coordinates the overlay uses.
func (m *MainUI) showDropIndicator(p dropPlan) {
	if m.dropIndicator == nil || m.dropInto == nil {
		return
	}
	if !p.valid {
		m.dropIndicator.Hide()
		m.dropInto.Hide()
		return
	}
	drv := fyne.CurrentApp().Driver()
	originY := drv.AbsolutePositionForObject(m.Tree).Y
	width := m.Tree.Size().Width
	relTop := p.rowTop - originY

	if p.into {
		m.dropIndicator.Hide()
		m.dropInto.Resize(fyne.NewSize(width, p.rowH))
		m.dropInto.Move(fyne.NewPos(0, relTop))
		m.dropInto.Show()
		m.dropInto.Refresh()
		return
	}
	m.dropInto.Hide()
	y := relTop
	if p.lineAtBottom {
		y = relTop + p.rowH
	}
	m.dropIndicator.Resize(fyne.NewSize(width, 2))
	m.dropIndicator.Move(fyne.NewPos(0, y-1))
	m.dropIndicator.Show()
	m.dropIndicator.Refresh()
}

// --- node-id helpers (string form: "0", "0/f1", "0/f1/r2") ---

func isCollectionID(id string) bool { return id != "" && !strings.Contains(id, "/") }

// collectionIndexOf returns the leading collection index of a node id, or -1.
func collectionIndexOf(id string) int {
	seg := id
	if i := strings.Index(id, "/"); i >= 0 {
		seg = id[:i]
	}
	n, err := strconv.Atoi(seg)
	if err != nil {
		return -1
	}
	return n
}

// nodeKind returns "c", "f", or "r" for a node id, or "" if malformed.
func nodeKind(id string) string {
	if id == "" {
		return ""
	}
	i := strings.LastIndex(id, "/")
	if i < 0 {
		return "c"
	}
	last := id[i+1:]
	if last == "" {
		return ""
	}
	return string(last[0])
}

// splitNode returns the parent container id and the leaf index for a folder or
// request node id (e.g. "0/f1/r2" → "0/f1", 2). For a bare collection id it
// returns ("", collectionIndex).
func splitNode(id string) (parent string, idx int, ok bool) {
	i := strings.LastIndex(id, "/")
	if i < 0 {
		n, err := strconv.Atoi(id)
		if err != nil {
			return "", 0, false
		}
		return "", n, true
	}
	last := id[i+1:]
	if len(last) < 2 {
		return "", 0, false
	}
	n, err := strconv.Atoi(last[1:])
	if err != nil {
		return "", 0, false
	}
	return id[:i], n, true
}
