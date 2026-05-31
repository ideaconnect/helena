package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/assets"
)

// requestTab is one tab in the editor tab strip: a colored method chip, the
// request name, and a close button, over a highlight rectangle that fills
// when the tab is active. Tapping the body selects the tab; tapping the
// close button closes it. Mirrors the methodPicker / treeRow custom-widget
// idiom (BaseWidget + Tapped + SimpleRenderer).
type requestTab struct {
	widget.BaseWidget
	bg        *canvas.Rectangle
	method    *canvas.Text
	name      *widget.Label
	closeBtn  *widget.Button
	onSelect  func()
	onDrag    func(*fyne.DragEvent)
	onDragEnd func()
}

// newRequestTab builds a tab bound to its select / close callbacks. Seed the
// visible state with setTab and setActive.
func newRequestTab(onSelect, onClose func()) *requestTab {
	t := &requestTab{
		bg:       canvas.NewRectangle(color.Transparent),
		method:   canvas.NewText("", nil),
		name:     widget.NewLabel(""),
		onSelect: onSelect,
	}
	t.method.TextStyle = fyne.TextStyle{Bold: true}
	t.closeBtn = widget.NewButtonWithIcon("", assets.Icon("xmark-circle-solid"), onClose)
	t.closeBtn.Importance = widget.LowImportance
	t.ExtendBaseWidget(t)
	return t
}

// setTab updates the method chip and name shown on the tab. A blank name
// renders as "Untitled" so a brand-new scratch tab still has a label.
func (t *requestTab) setTab(method, name string) {
	if name == "" {
		name = "Untitled"
	}
	t.method.Text = method
	t.method.Color = methodColor(method)
	t.method.Refresh()
	t.name.SetText(name)
}

// setActive toggles the highlight rectangle behind the tab.
func (t *requestTab) setActive(active bool) {
	if active {
		t.bg.FillColor = theme.Color(theme.ColorNameSelection)
	} else {
		t.bg.FillColor = color.Transparent
	}
	t.bg.Refresh()
}

// Tapped selects the tab. The close button intercepts taps over its own
// area, so this fires only for the chip / name region. Fyne distinguishes a
// tap from a drag by movement, so dragging to reorder does not also select.
func (t *requestTab) Tapped(_ *fyne.PointEvent) {
	if t.onSelect != nil {
		t.onSelect()
	}
}

// Dragged / DragEnd make the tab reorderable: the strip live-reorders as the
// pointer crosses neighboring tabs, and persists the new order on release.
func (t *requestTab) Dragged(e *fyne.DragEvent) {
	if t.onDrag != nil {
		t.onDrag(e)
	}
}

func (t *requestTab) DragEnd() {
	if t.onDragEnd != nil {
		t.onDragEnd()
	}
}

func (t *requestTab) CreateRenderer() fyne.WidgetRenderer {
	content := container.NewHBox(t.method, t.name, t.closeBtn)
	return widget.NewSimpleRenderer(container.NewStack(t.bg, content))
}
