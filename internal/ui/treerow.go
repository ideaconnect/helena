package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/assets"
)

// rowActions bundles the row-icon callbacks the tree row's
// rename / delete / duplicate buttons invoke. Each takes the row's
// current ID (captured by the row from setRequest / setBranch).
type rowActions struct {
	onRename    func(id string)
	onDelete    func(id string)
	onDuplicate func(id string) // requests only; folders / collections leave it nil
}

// treeRow is the per-row template the Collections tree uses. It
// renders:
//   - requests as `→ METHOD  Name   [rename] [duplicate] [delete]`
//     with the METHOD chip in its brand color (see method.go).
//   - branches (folders + collections) as `Label   [rename] [delete]`.
//
// One template is recycled across many tree rows by Fyne; setRequest /
// setBranch updates the visible widgets + the captured id so the
// per-row button callbacks operate on the right node.
type treeRow struct {
	widget.BaseWidget
	id        string
	prefix    *widget.Icon
	method    *canvas.Text
	name      *widget.Label
	renameBtn *widget.Button
	dupBtn    *widget.Button
	delBtn    *widget.Button
	actions   rowActions
}

func newTreeRow(actions rowActions) *treeRow {
	r := &treeRow{
		prefix:  widget.NewIcon(assets.Icon("nav-arrow-right")),
		method:  canvas.NewText("", nil),
		name:    widget.NewLabel(""),
		actions: actions,
	}
	r.method.TextStyle = fyne.TextStyle{Bold: true}
	r.renameBtn = widget.NewButtonWithIcon("", assets.Icon("input-field"), func() {
		if r.actions.onRename != nil {
			r.actions.onRename(r.id)
		}
	})
	r.dupBtn = widget.NewButtonWithIcon("", assets.Icon("copy"), func() {
		if r.actions.onDuplicate != nil {
			r.actions.onDuplicate(r.id)
		}
	})
	r.delBtn = widget.NewButtonWithIcon("", assets.Icon("xmark-circle-solid"), func() {
		if r.actions.onDelete != nil {
			r.actions.onDelete(r.id)
		}
	})
	// Visual styling: low-importance for rename / duplicate (neutral
	// edits), danger-importance for delete (so the red tint matches
	// the icon's intent).
	r.renameBtn.Importance = widget.LowImportance
	r.dupBtn.Importance = widget.LowImportance
	r.delBtn.Importance = widget.DangerImportance
	r.ExtendBaseWidget(r)
	return r
}

// setRequest configures the row as a request: arrow + colored bold
// method chip + name + all three action icons.
func (r *treeRow) setRequest(id, method, name string) {
	r.id = id
	r.prefix.Show()
	r.method.Text = method
	r.method.Color = methodColor(method)
	r.method.Show()
	r.method.Refresh()
	r.name.SetText(name)
	r.dupBtn.Show()
}

// setBranch configures the row as a folder or collection: just the
// label + rename / delete icons (no arrow, no method chip, no
// duplicate — duplicating a collection isn't a Helena primitive).
func (r *treeRow) setBranch(id, label string) {
	r.id = id
	r.prefix.Hide()
	r.method.Text = ""
	r.method.Hide()
	r.name.SetText(label)
	r.dupBtn.Hide()
}

func (r *treeRow) CreateRenderer() fyne.WidgetRenderer {
	// Layout: arrow + method chip + name on the left; action icons
	// pushed to the right via a stretchable spacer (NewBorder with
	// the action HBox in the trailing slot).
	leading := container.NewHBox(r.prefix, r.method, r.name)
	trailing := container.NewHBox(r.renameBtn, r.dupBtn, r.delBtn)
	// Tiny vertical-divider canvas-rectangle to give the buttons a
	// small visual gap from the label without inserting padding that
	// would make tree rows taller.
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(8, 1))
	row := container.New(layout.NewBorderLayout(nil, nil, nil, trailing),
		trailing, container.NewHBox(leading, spacer))
	return widget.NewSimpleRenderer(row)
}
