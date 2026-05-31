package ui

import (
	"image/color"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Compile-time checks: bodyView must be focusable so the canvas routes Ctrl+C
// to it, and shortcutable so it actually receives the copy shortcut.
var (
	_ fyne.Focusable    = (*bodyView)(nil)
	_ fyne.Shortcutable = (*bodyView)(nil)
)

// styledRun is a maximal run of same-colored text within a single visual line.
// A nil col means "render in the theme foreground", so plain/structural text
// follows the active theme instead of being baked to a fixed color.
type styledRun struct {
	text string
	col  color.Color
}

// bodyView is a read-only, viewport-virtualized text viewer for response bodies.
// It exists because widget.Entry (Raw) and widget.TextGrid (Pretty) both
// materialize a render object per character/line for the *entire* document, so a
// 150 KB body balloons to >1 GB. bodyView instead keeps the document as plain
// run slices (which share the source string's backing array, so they cost almost
// nothing) and renders only the lines currently scrolled into view via
// widget.List. Memory stays flat regardless of body size — "notepad-cheap".
//
// Long source lines are soft-wrapped to the current width so a minified
// one-line body can't produce a single giant row. The view is focusable and
// copies its whole text on Ctrl+C; it carries no per-character selection.
type bodyView struct {
	widget.BaseWidget

	full        string        // whole document, used for copy
	placeholder string        // shown (greyed) when full is empty
	logical     [][]styledRun // one entry per source ('\n'-separated) line
	display     [][]styledRun // logical wrapped to cols; one visual line each
	list        *widget.List
	cols        int     // current wrap width in characters (0 → unwrapped)
	charW       float32 // monospace advance width
	lineH       float32 // single-line height
}

// newBodyView builds an empty viewer ready to be placed in a tab.
func newBodyView(placeholder string) *bodyView {
	b := &bodyView{placeholder: placeholder}
	b.charW, b.lineH = monoMetrics()
	b.list = widget.NewList(
		func() int { return len(b.display) },
		func() fyne.CanvasObject { return newBodyRow(b.charW, b.lineH) },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*bodyRow)
			if id >= 0 && id < len(b.display) {
				row.setRuns(b.display[id])
			} else {
				row.setRuns(nil)
			}
		},
	)
	// The List highlights tapped rows; we don't want selection, only focus so
	// Ctrl+C reaches us. Focus the view and immediately drop the highlight.
	b.list.OnSelected = func(widget.ListItemID) {
		if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil {
			c.Focus(b)
		}
		b.list.UnselectAll()
	}
	b.ExtendBaseWidget(b)
	b.showPlaceholder()
	return b
}

// SetText replaces the content with plain (single-color) text — the Raw view
// and error paths. Splitting on '\n' yields one run per source line; the runs
// reference substrings of s, so this does not copy the body.
func (b *bodyView) SetText(s string) {
	if s == "" {
		b.full = ""
		b.showPlaceholder()
		return
	}
	lines := strings.Split(s, "\n")
	logical := make([][]styledRun, len(lines))
	for i, ln := range lines {
		if ln == "" {
			logical[i] = nil
			continue
		}
		logical[i] = []styledRun{{text: ln}}
	}
	b.setLines(s, logical)
}

// setLines installs pre-built logical lines (used by the Pretty view, which
// colors per token) and the full text for copy, then wraps and refreshes.
func (b *bodyView) setLines(full string, logical [][]styledRun) {
	b.full = full
	b.logical = logical
	b.rewrap()
}

// showPlaceholder renders the greyed placeholder as the sole line.
func (b *bodyView) showPlaceholder() {
	if b.placeholder == "" {
		b.logical = nil
	} else {
		grey := theme.Color(theme.ColorNamePlaceHolder)
		b.logical = [][]styledRun{{{text: b.placeholder, col: grey}}}
	}
	b.rewrap()
}

// rewrap recomputes display lines from logical lines at the current width and
// repaints from the top.
func (b *bodyView) rewrap() {
	b.display = wrapLines(b.logical, b.cols)
	if b.list != nil {
		b.list.Refresh()
		b.list.ScrollToTop()
	}
}

// Resize recomputes the wrap column whenever the available width changes, so
// soft-wrapping tracks the panel like a notepad.
func (b *bodyView) Resize(size fyne.Size) {
	b.BaseWidget.Resize(size)
	if cols := b.colsFor(size.Width); cols != b.cols {
		b.cols = cols
		b.rewrap()
	}
}

// colsFor converts a pixel width into a character column budget, leaving room
// for the list's padding and scrollbar.
func (b *bodyView) colsFor(width float32) int {
	if b.charW <= 0 {
		return 0
	}
	usable := width - 2*theme.Size(theme.SizeNamePadding) - theme.Size(theme.SizeNameScrollBar)
	c := int(usable / b.charW)
	if c < 1 {
		c = 1
	}
	return c
}

// CreateRenderer renders the embedded virtualized list.
func (b *bodyView) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(b.list)
}

// --- focus + copy (no per-character selection) ---

func (b *bodyView) FocusGained()            {}
func (b *bodyView) FocusLost()              {}
func (b *bodyView) TypedRune(rune)          {}
func (b *bodyView) TypedKey(*fyne.KeyEvent) {}

// TypedShortcut copies the whole body on Ctrl/Cmd+C.
func (b *bodyView) TypedShortcut(s fyne.Shortcut) {
	if _, ok := s.(*fyne.ShortcutCopy); ok {
		b.copyAll()
	}
}

// copyAll places the full document on the clipboard.
func (b *bodyView) copyAll() {
	if b.full == "" {
		return
	}
	if app := fyne.CurrentApp(); app != nil {
		if cb := app.Clipboard(); cb != nil {
			cb.SetContent(b.full)
		}
	}
}

// monoMetrics measures the monospace cell so wrapping and row layout agree.
func monoMetrics() (charW, lineH float32) {
	size := theme.Size(theme.SizeNameText)
	s := fyne.MeasureText("0", size, fyne.TextStyle{Monospace: true})
	return s.Width, s.Height
}

// wrapLines splits each logical line into one or more display lines no wider
// than cols runes, carrying the per-run colors across the cut. cols <= 0 leaves
// the lines unwrapped (the pre-layout state).
func wrapLines(logical [][]styledRun, cols int) [][]styledRun {
	if cols <= 0 {
		return logical
	}
	out := make([][]styledRun, 0, len(logical))
	for _, line := range logical {
		if runeLen(line) <= cols {
			out = append(out, line)
			continue
		}
		var cur []styledRun
		curLen := 0
		for _, run := range line {
			rs := []rune(run.text)
			for len(rs) > 0 {
				if curLen == cols {
					out = append(out, cur)
					cur = nil
					curLen = 0
				}
				take := cols - curLen
				if take > len(rs) {
					take = len(rs)
				}
				cur = append(cur, styledRun{text: string(rs[:take]), col: run.col})
				curLen += take
				rs = rs[take:]
			}
		}
		out = append(out, cur)
	}
	return out
}

// runeLen is the total rune count across a line's runs.
func runeLen(line []styledRun) int {
	n := 0
	for _, r := range line {
		n += utf8.RuneCountInString(r.text)
	}
	return n
}

// bodyRow renders one visual line as a sequence of colored monospace runs. It
// is the recycled List item template, so only viewport-visible rows exist.
type bodyRow struct {
	widget.BaseWidget
	runs  []styledRun
	charW float32
	lineH float32
}

func newBodyRow(charW, lineH float32) *bodyRow {
	r := &bodyRow{charW: charW, lineH: lineH}
	r.ExtendBaseWidget(r)
	return r
}

func (r *bodyRow) setRuns(runs []styledRun) {
	r.runs = runs
	r.Refresh()
}

func (r *bodyRow) CreateRenderer() fyne.WidgetRenderer {
	rr := &bodyRowRenderer{row: r}
	rr.refresh()
	return rr
}

type bodyRowRenderer struct {
	row   *bodyRow
	texts []*canvas.Text
}

// refresh syncs the canvas.Text pool to the current runs, reusing objects
// across recycling so scrolling allocates nothing steady-state.
func (rr *bodyRowRenderer) refresh() {
	runs := rr.row.runs
	size := theme.Size(theme.SizeNameText)
	fg := theme.Color(theme.ColorNameForeground)
	for len(rr.texts) < len(runs) {
		t := canvas.NewText("", fg)
		t.TextStyle = fyne.TextStyle{Monospace: true}
		t.TextSize = size
		rr.texts = append(rr.texts, t)
	}
	rr.texts = rr.texts[:len(runs)]
	for i, run := range runs {
		t := rr.texts[i]
		t.Text = run.text
		if run.col != nil {
			t.Color = run.col
		} else {
			t.Color = fg
		}
		t.TextSize = size
	}
}

func (rr *bodyRowRenderer) Refresh() {
	rr.refresh()
	rr.Layout(rr.row.Size())
	canvas.Refresh(rr.row)
}

func (rr *bodyRowRenderer) Layout(_ fyne.Size) {
	x := float32(0)
	for _, t := range rr.texts {
		w := rr.row.charW * float32(utf8.RuneCountInString(t.Text))
		t.Move(fyne.NewPos(x, 0))
		t.Resize(fyne.NewSize(w, rr.row.lineH))
		x += w
	}
}

func (rr *bodyRowRenderer) MinSize() fyne.Size {
	w := float32(0)
	for _, t := range rr.texts {
		w += rr.row.charW * float32(utf8.RuneCountInString(t.Text))
	}
	return fyne.NewSize(w, rr.row.lineH)
}

func (rr *bodyRowRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(rr.texts))
	for i, t := range rr.texts {
		objs[i] = t
	}
	return objs
}

func (rr *bodyRowRenderer) Destroy() {}
