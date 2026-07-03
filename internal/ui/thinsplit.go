package ui

// thinSplit is a drag-resizable two-pane split whose divider renders as a
// thin hairline (like VS Code / Bruno) instead of Fyne's thick shadow bar
// with a centre grab handle. It exists so the main layout's splits need no
// container.NewThemeOverride wrapping: every override call mints a fresh Fyne
// theme scope, and Fyne re-parses fonts per scope × text style and eagerly
// walks the wrapped subtree — the previous splitTheme + paneTheme wrapping
// cost six scopes across the two main splits. The divider's old themed
// metrics are hard-coded here instead (thickness = splitTheme padding 1.5 × 2,
// idle fill = the separator colour, no grab handle).
//
// Adapted from fyne.io/fyne/v2 container.Split (BSD-3-Clause, © Fyne.io and
// friends), with the theme-derived divider sizing and the handle removed.

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	// thinDividerThickness is the divider bar's cross-axis size: the old
	// splitTheme's SizeNamePadding (1.5) × 2 — thin but still grabbable.
	thinDividerThickness float32 = 3
	// thinDividerMinLength mirrors the old themed minimum (padding × 6); it
	// only feeds the divider's MinSize, which the split's own MinSize sums.
	thinDividerMinLength float32 = 9
)

var _ fyne.CanvasObject = (*thinSplit)(nil)

// thinSplit divides its size between two children around a draggable hairline.
// Offset semantics match container.Split: 0.0 = leading at min size, 1.0 =
// trailing at min size.
type thinSplit struct {
	widget.BaseWidget
	Offset     float64
	Horizontal bool
	Leading    fyne.CanvasObject
	Trailing   fyne.CanvasObject

	// offsetUpdated tells the renderer the next refresh is only an offset
	// change (resize + move — no child refresh); cleared in Refresh.
	offsetUpdated bool
}

func newThinSplit(horizontal bool, leading, trailing fyne.CanvasObject) *thinSplit {
	s := &thinSplit{
		Offset:     0.5,
		Horizontal: horizontal,
		Leading:    leading,
		Trailing:   trailing,
	}
	s.BaseWidget.ExtendBaseWidget(s)
	return s
}

// CreateRenderer is a private method to Fyne which links this widget to its renderer.
func (s *thinSplit) CreateRenderer() fyne.WidgetRenderer {
	s.BaseWidget.ExtendBaseWidget(s)
	d := newThinDivider(s)
	return &thinSplitRenderer{
		split:   s,
		divider: d,
		objects: []fyne.CanvasObject{s.Leading, d, s.Trailing},
	}
}

// SetOffset sets the offset (0.0 to 1.0) of the divider.
func (s *thinSplit) SetOffset(offset float64) {
	if s.Offset == offset {
		return
	}
	s.Offset = offset
	s.offsetUpdated = true
	s.Refresh()
}

var _ fyne.WidgetRenderer = (*thinSplitRenderer)(nil)

type thinSplitRenderer struct {
	split   *thinSplit
	divider *thinDivider
	objects []fyne.CanvasObject
}

func (r *thinSplitRenderer) Destroy() {}

func (r *thinSplitRenderer) Layout(size fyne.Size) {
	var dividerPos, leadingPos, trailingPos fyne.Position
	var dividerSize, leadingSize, trailingSize fyne.Size

	dividerVisible := r.split.Leading.Visible() && r.split.Trailing.Visible()
	if !r.split.Leading.Visible() {
		trailingPos = fyne.NewPos(0, 0)
		trailingSize = size
	} else if !r.split.Trailing.Visible() {
		leadingPos = fyne.NewPos(0, 0)
		leadingSize = size
	} else if dividerVisible {
		if r.split.Horizontal {
			lw, tw := r.computeSplitLengths(size.Width, r.minLeadingWidth(), r.minTrailingWidth())
			leadingPos.X = 0
			leadingSize.Width = lw
			leadingSize.Height = size.Height
			dividerPos.X = lw
			dividerSize.Width = thinDividerThickness
			dividerSize.Height = size.Height
			trailingPos.X = lw + dividerSize.Width
			trailingSize.Width = tw
			trailingSize.Height = size.Height
		} else {
			lh, th := r.computeSplitLengths(size.Height, r.minLeadingHeight(), r.minTrailingHeight())
			leadingPos.Y = 0
			leadingSize.Width = size.Width
			leadingSize.Height = lh
			dividerPos.Y = lh
			dividerSize.Width = size.Width
			dividerSize.Height = thinDividerThickness
			trailingPos.Y = lh + dividerSize.Height
			trailingSize.Width = size.Width
			trailingSize.Height = th
		}
	}

	r.divider.Move(dividerPos)
	r.divider.Resize(dividerSize)
	r.divider.Hidden = !dividerVisible

	r.split.Leading.Move(leadingPos)
	r.split.Leading.Resize(leadingSize)
	r.split.Trailing.Move(trailingPos)
	r.split.Trailing.Resize(trailingSize)
	canvas.Refresh(r.divider)
}

func (r *thinSplitRenderer) MinSize() fyne.Size {
	s := fyne.NewSize(0, 0)
	dividerVisible := r.split.Leading.Visible() && r.split.Trailing.Visible()
	for i, o := range r.objects {
		if (i == 1 /*divider*/ && !dividerVisible) || (i != 1 && !o.Visible()) {
			continue
		}
		min := o.MinSize()
		if r.split.Horizontal {
			s.Width += min.Width
			s.Height = fyne.Max(s.Height, min.Height)
		} else {
			s.Width = fyne.Max(s.Width, min.Width)
			s.Height += min.Height
		}
	}
	return s
}

func (r *thinSplitRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *thinSplitRenderer) Refresh() {
	if r.split.offsetUpdated {
		r.Layout(r.split.Size())
		r.split.offsetUpdated = false
		return
	}

	r.objects[0] = r.split.Leading
	// [1] is the divider, which doesn't change.
	r.objects[2] = r.split.Trailing
	r.Layout(r.split.Size())

	r.split.Leading.Refresh()
	r.divider.Refresh()
	r.split.Trailing.Refresh()
	canvas.Refresh(r.split)
}

func (r *thinSplitRenderer) computeSplitLengths(total, lMin, tMin float32) (float32, float32) {
	available := float64(total - thinDividerThickness)
	if available <= 0 {
		return 0, 0
	}
	ld := float64(lMin)
	tr := float64(tMin)
	offset := r.split.Offset

	min := ld / available
	max := 1 - tr/available
	if min <= max {
		if offset < min {
			offset = min
		}
		if offset > max {
			offset = max
		}
	} else {
		offset = ld / (ld + tr)
	}

	ld = offset * available
	tr = available - ld
	return float32(ld), float32(tr)
}

func (r *thinSplitRenderer) minLeadingWidth() float32 {
	if r.split.Leading.Visible() {
		return r.split.Leading.MinSize().Width
	}
	return 0
}

func (r *thinSplitRenderer) minLeadingHeight() float32 {
	if r.split.Leading.Visible() {
		return r.split.Leading.MinSize().Height
	}
	return 0
}

func (r *thinSplitRenderer) minTrailingWidth() float32 {
	if r.split.Trailing.Visible() {
		return r.split.Trailing.MinSize().Width
	}
	return 0
}

func (r *thinSplitRenderer) minTrailingHeight() float32 {
	if r.split.Trailing.Visible() {
		return r.split.Trailing.MinSize().Height
	}
	return 0
}

var (
	_ fyne.CanvasObject  = (*thinDivider)(nil)
	_ fyne.Draggable     = (*thinDivider)(nil)
	_ desktop.Cursorable = (*thinDivider)(nil)
	_ desktop.Hoverable  = (*thinDivider)(nil)
)

// thinDivider is the draggable hairline: idle it paints the separator colour,
// hovered the theme hover colour; there is no centre grab handle.
type thinDivider struct {
	widget.BaseWidget
	split          *thinSplit
	hovered        bool
	startDragOff   *fyne.Position
	currentDragPos fyne.Position
}

func newThinDivider(split *thinSplit) *thinDivider {
	d := &thinDivider{split: split}
	d.ExtendBaseWidget(d)
	return d
}

// CreateRenderer is a private method to Fyne which links this widget to its renderer.
func (d *thinDivider) CreateRenderer() fyne.WidgetRenderer {
	d.ExtendBaseWidget(d)
	th := d.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	line := canvas.NewRectangle(th.Color(theme.ColorNameSeparator, v))
	return &thinDividerRenderer{
		divider: d,
		line:    line,
		objects: []fyne.CanvasObject{line},
	}
}

func (d *thinDivider) Cursor() desktop.Cursor {
	if d.split.Horizontal {
		return desktop.HResizeCursor
	}
	return desktop.VResizeCursor
}

func (d *thinDivider) DragEnd() {
	d.startDragOff = nil
}

func (d *thinDivider) Dragged(e *fyne.DragEvent) {
	if d.startDragOff == nil {
		d.currentDragPos = d.Position().Add(e.Position)
		start := e.Position.Subtract(e.Dragged)
		d.startDragOff = &start
	} else {
		d.currentDragPos = d.currentDragPos.Add(e.Dragged)
	}

	x, y := d.currentDragPos.Components()
	var offset, leadingRatio, trailingRatio float64
	if d.split.Horizontal {
		widthFree := float64(d.split.Size().Width - thinDividerThickness)
		leadingRatio = float64(d.split.Leading.MinSize().Width) / widthFree
		trailingRatio = 1. - (float64(d.split.Trailing.MinSize().Width) / widthFree)
		offset = float64(x-d.startDragOff.X) / widthFree
	} else {
		heightFree := float64(d.split.Size().Height - thinDividerThickness)
		leadingRatio = float64(d.split.Leading.MinSize().Height) / heightFree
		trailingRatio = 1. - (float64(d.split.Trailing.MinSize().Height) / heightFree)
		offset = float64(y-d.startDragOff.Y) / heightFree
	}

	if offset < leadingRatio {
		offset = leadingRatio
	}
	if offset > trailingRatio {
		offset = trailingRatio
	}
	d.split.SetOffset(offset)
}

func (d *thinDivider) MouseIn(*desktop.MouseEvent) {
	d.hovered = true
	d.Refresh()
}

func (d *thinDivider) MouseMoved(*desktop.MouseEvent) {}

func (d *thinDivider) MouseOut() {
	d.hovered = false
	d.Refresh()
}

var _ fyne.WidgetRenderer = (*thinDividerRenderer)(nil)

type thinDividerRenderer struct {
	divider *thinDivider
	line    *canvas.Rectangle
	objects []fyne.CanvasObject
}

func (r *thinDividerRenderer) Destroy() {}

func (r *thinDividerRenderer) Layout(size fyne.Size) {
	r.line.Resize(size)
}

func (r *thinDividerRenderer) MinSize() fyne.Size {
	if r.divider.split.Horizontal {
		return fyne.NewSize(thinDividerThickness, thinDividerMinLength)
	}
	return fyne.NewSize(thinDividerMinLength, thinDividerThickness)
}

func (r *thinDividerRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *thinDividerRenderer) Refresh() {
	th := r.divider.Theme()
	v := fyne.CurrentApp().Settings().ThemeVariant()
	if r.divider.hovered {
		r.line.FillColor = th.Color(theme.ColorNameHover, v)
	} else {
		r.line.FillColor = th.Color(theme.ColorNameSeparator, v)
	}
	r.line.Refresh()
	r.Layout(r.divider.Size())
}
