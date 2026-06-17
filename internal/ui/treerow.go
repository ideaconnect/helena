package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// treeRow is the per-row template the Collections tree uses: a brand-colored
// bold method chip (requests only — see method.go) followed by the name
// (branches: just the label).
//
// It is a display widget plus drag source: it implements fyne.Draggable so a
// row can be picked up and dropped elsewhere in the tree (see treedrag.go), but
// NOT Tappable — the enclosing tree node still handles selection (a full-width
// tap, distinct from a drag) and paints the full-width hover + selection
// backgrounds. Node actions live on the sidebar toolbar (shell.go). The name
// ellipsis-truncates so long names never overflow the panel.
//
// One template is recycled across many rows by Fyne; setRequest / setBranch
// update the visible widgets and the captured id for the bound node so the drag
// callbacks act on the right node.
type treeRow struct {
	widget.BaseWidget
	id        string
	method    *canvas.Text
	name      *widget.Label
	onDrag    func(id string, e *fyne.DragEvent)
	onDragEnd func(id string)
	dragging  func() bool // reports whether a tree drag is currently in flight
}

func newTreeRow(onDrag func(string, *fyne.DragEvent), onDragEnd func(string), dragging func() bool) *treeRow {
	name := widget.NewLabel("")
	name.Truncation = fyne.TextTruncateEllipsis
	r := &treeRow{
		method:    canvas.NewText("", nil),
		name:      name,
		onDrag:    onDrag,
		onDragEnd: onDragEnd,
		dragging:  dragging,
	}
	r.method.TextStyle = fyne.TextStyle{Bold: true}
	r.ExtendBaseWidget(r)
	return r
}

// Cursor implements desktop.Cursorable: while a drag is in flight the row shows
// a grab (pointer) cursor so the gesture reads as moving the node. Fyne
// recomputes the cursor from the object under the pointer on every move, even
// during a drag, so this flips as soon as the drag starts. Fyne has no
// dedicated "grabbing" cursor, so the hand pointer stands in.
func (r *treeRow) Cursor() desktop.Cursor {
	if r.dragging != nil && r.dragging() {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

// setRequest configures the row as a request: colored bold method chip + name.
func (r *treeRow) setRequest(id, method, name string) {
	r.id = id
	r.method.Text = method
	r.method.Color = methodColor(method)
	r.method.Show()
	r.method.Refresh()
	r.name.SetText(name)
}

// setBranch configures the row as a folder or collection: just a label.
// Hiding the chip flips treeRowLayout to flush-name mode (see CreateRenderer).
func (r *treeRow) setBranch(id, label string) {
	r.id = id
	r.method.Text = ""
	r.method.Hide()
	r.name.SetText(label)
}

// Dragged / DragEnd make the row a drag source. The tree node distinguishes a
// tap (select) from a drag by movement, so dragging does not also select.
func (r *treeRow) Dragged(e *fyne.DragEvent) {
	if r.onDrag != nil {
		r.onDrag(r.id, e)
	}
}

func (r *treeRow) DragEnd() {
	if r.onDragEnd != nil {
		r.onDragEnd(r.id)
	}
}

func (r *treeRow) CreateRenderer() fyne.WidgetRenderer {
	// treeRowLayout (below) arranges the chip + name. The chip is pulled left
	// into the disclosure-arrow column so a request lines up with same-depth
	// folders' arrows; for a branch the chip is hidden and the name renders at
	// the content origin. No background of its own — the tree node paints
	// hover/selection.
	return widget.NewSimpleRenderer(container.New(&treeRowLayout{row: r}, r.method, r.name))
}

// treeRowLayout positions a recycled tree row's chip + name.
//
// widget.Tree lays every node's content out at x = pad + Indent() + iconSize +
// pad and paints a branch's disclosure arrow at x = pad + Indent() (see
// treeNode.Layout / treeNode.Indent in Fyne). The gap from the content origin
// back to the arrow column is therefore iconSize + pad. A request has no arrow,
// so we pull its method chip left by that gap — putting the chip in the arrow
// column, vertically aligned with sibling folders' arrows (the requested
// behaviour). The chip overhangs the content container's left edge, which is
// fine: the node's full-width background sits under it and the tree's scroll
// clip is far to the left. The name then flows after the chip.
//
// For a branch the chip is hidden and the name sits at the content origin
// (glyph inset by the Label's own innerPadding), so folder names are unchanged.
type treeRowLayout struct{ row *treeRow }

// arrowGap is iconSize + pad: the distance from the content origin back to the
// disclosure-arrow column, resolved against the row's (sidebar-scoped) theme.
func (l *treeRowLayout) arrowGap() float32 {
	return theme.SizeForWidget(theme.SizeNameInlineIcon, l.row) +
		theme.SizeForWidget(theme.SizeNamePadding, l.row)
}

func (l *treeRowLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	m := l.row.name.MinSize()
	if h := l.row.method.MinSize().Height; h > m.Height {
		m.Height = h
	}
	return fyne.NewSize(0, m.Height)
}

func (l *treeRowLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	name := l.row.name
	if !l.row.method.Visible() { // branch: name flush at the content origin
		name.Move(fyne.NewPos(0, 0))
		name.Resize(size)
		return
	}
	gap := l.arrowGap()
	mw := l.row.method.MinSize().Width
	// Full height + y=0 keeps canvas.Text vertically centred as before.
	l.row.method.Resize(fyne.NewSize(mw, size.Height))
	l.row.method.Move(fyne.NewPos(-gap, 0))
	nameX := -gap + mw + theme.SizeForWidget(theme.SizeNameInnerPadding, l.row)
	name.Move(fyne.NewPos(nameX, 0))
	name.Resize(fyne.NewSize(size.Width-nameX, size.Height))
}
