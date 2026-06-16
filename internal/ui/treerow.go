package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
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
	id         string
	method     *canvas.Text
	methodSlot *fyne.Container // left-pads the chip to align with the name label's text inset (#tree-align)
	name       *widget.Label
	onDrag     func(id string, e *fyne.DragEvent)
	onDragEnd  func(id string)
	dragging   func() bool // reports whether a tree drag is currently in flight
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
	if r.methodSlot != nil {
		r.methodSlot.Show()
	}
	r.method.Refresh()
	r.name.SetText(name)
}

// setBranch configures the row as a folder or collection: just a label.
// The whole method slot (not just the chip) is hidden so the Border drops its
// left object and the name reflows flush — see CreateRenderer.
func (r *treeRow) setBranch(id, label string) {
	r.id = id
	r.method.Text = ""
	r.method.Hide()
	if r.methodSlot != nil {
		r.methodSlot.Hide()
	}
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
	// Method chip on the left, name filling the rest (it truncates to the row
	// width). No background of its own — the tree node paints hover/selection.
	//
	// The chip is left-padded by SizeNameInnerPadding so its glyph starts at the
	// same x as the name Label's text (a widget.Label/RichText insets its glyph
	// by innerPadding on the left). Without this a request's method chip sits
	// innerPadding to the LEFT of a same-depth folder's name (the bug). The whole
	// slot — not just the chip — is hidden for branches: a hidden chip inside a
	// *visible* wrapper would still reserve the wrapper's LeftPadding
	// (CustomPaddedLayout.MinSize adds it even when its child is hidden, and
	// Border checks the wrapper's visibility, not the child's), pushing the
	// folder name right. Hiding the wrapper makes Border ignore the left slot so
	// folder names stay flush. Top/bottom pad stay 0 so the chip keeps the full
	// row height and canvas.Text self-centers vertically as before.
	pad := theme.SizeForWidget(theme.SizeNameInnerPadding, r)
	r.methodSlot = container.New(layout.NewCustomPaddedLayout(0, 0, pad, 0), r.method)
	if !r.method.Visible() {
		r.methodSlot.Hide()
	}
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, r.methodSlot, nil, r.name))
}
