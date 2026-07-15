package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// requestTab is one tab in the editor tab strip: a colored method chip, the
// request name, and a close button, over a rounded-top background. When active
// the background fills with the editor's content colour so the tab connects to
// the content below and hides the under-strip separator line (see the tab-strip
// assembly in shell.go for the band + line). A custom renderer lays the parts
// out vertically centred with a little vertical breathing room around them.
type requestTab struct {
	widget.BaseWidget
	bg        *canvas.Rectangle
	method    *canvas.Text
	name      *canvas.Text
	closeBtn  *widget.Button
	onSelect  func()
	onDrag    func(*fyne.DragEvent)
	onDragEnd func()
}

func newRequestTab(onSelect, onClose func()) *requestTab {
	t := &requestTab{
		bg:       canvas.NewRectangle(color.Transparent),
		method:   canvas.NewText("", nil),
		name:     canvas.NewText("", theme.Color(theme.ColorNameForeground)),
		onSelect: onSelect,
	}
	radius := theme.Size(theme.SizeNameInputRadius)
	t.bg.TopLeftCornerRadius = radius
	t.bg.TopRightCornerRadius = radius
	t.method.TextStyle = fyne.TextStyle{Bold: true}
	t.closeBtn = widget.NewButtonWithIcon("", themedIcon("circle-xmark"), onClose)
	t.closeBtn.Importance = widget.LowImportance
	t.ExtendBaseWidget(t)
	return t
}

// setTab updates the method chip and name. A blank name renders as "Untitled".
// A dirty tab (unsaved edits) gets a trailing " *" on the name so the strip
// signals which requests have pending changes (#139 dirty markers).
func (t *requestTab) setTab(method, name string, dirty bool) {
	if name == "" {
		name = "Untitled"
	}
	if dirty {
		name += " *"
	}
	t.method.Text = method
	t.method.Color = methodColor(method)
	t.method.Refresh()
	t.name.Text = name
	t.name.Color = theme.Color(theme.ColorNameForeground)
	t.name.Refresh()
}

// setActive fills the active tab's background with the editor content colour
// (so it reads as connected to the content, covering the under-strip line);
// inactive tabs are transparent and show the strip band + line.
func (t *requestTab) setActive(active bool) {
	if active {
		t.bg.FillColor = theme.Color(theme.ColorNameBackground)
	} else {
		t.bg.FillColor = color.Transparent
	}
	t.bg.Refresh()
}

// Tapped selects the tab. The close button intercepts taps over its own area.
func (t *requestTab) Tapped(_ *fyne.PointEvent) {
	if t.onSelect != nil {
		t.onSelect()
	}
}

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
	return &requestTabRenderer{
		tab:  t,
		objs: []fyne.CanvasObject{t.bg, t.method, t.name, t.closeBtn},
	}
}

type requestTabRenderer struct {
	tab  *requestTab
	objs []fyne.CanvasObject
}

func (r *requestTabRenderer) Layout(size fyne.Size) {
	pad := theme.Padding()
	// The close button insets its icon by InnerPadding, so to keep the same
	// visual margin from the left edge to the method as from the right edge to
	// the close icon, the left margin matches the button box pad + that inset.
	leftPad := pad + theme.InnerPadding()
	r.tab.bg.Resize(size)
	r.tab.bg.Move(fyne.NewPos(0, 0))

	vmid := func(h float32) float32 { return (size.Height - h) / 2 } // vertical centre

	m := r.tab.method.MinSize()
	r.tab.method.Resize(m)
	r.tab.method.Move(fyne.NewPos(leftPad, vmid(m.Height)))

	c := r.tab.closeBtn.MinSize()
	r.tab.closeBtn.Resize(c)
	r.tab.closeBtn.Move(fyne.NewPos(size.Width-c.Width-pad, vmid(c.Height)))

	n := r.tab.name.MinSize()
	r.tab.name.Resize(n)
	r.tab.name.Move(fyne.NewPos(leftPad+m.Width+pad, vmid(n.Height))) // method's natural width + a gap
}

func (r *requestTabRenderer) MinSize() fyne.Size {
	pad := theme.Padding()
	leftPad := pad + theme.InnerPadding()
	n := r.tab.name.MinSize()
	c := r.tab.closeBtn.MinSize()
	m := r.tab.method.MinSize()
	h := n.Height
	for _, hh := range []float32{c.Height, m.Height} {
		if hh > h {
			h = hh
		}
	}
	h += theme.Padding() * 2 // vertical breathing so the close button isn't flush to the edges
	w := leftPad + m.Width + pad + n.Width + pad + c.Width + pad
	return fyne.NewSize(w, h)
}

func (r *requestTabRenderer) Refresh() {
	r.tab.bg.Refresh()
	r.tab.method.Refresh()
	r.tab.name.Refresh()
	canvas.Refresh(r.tab)
}

func (r *requestTabRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *requestTabRenderer) Destroy()                     {}
