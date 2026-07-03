package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"github.com/idct/helena/internal/model"
)

func enableButton(b *ttwidget.Button, on bool) {
	if b == nil {
		return
	}
	if on {
		b.Enable()
	} else {
		b.Disable()
	}
}

// tipButton builds a compact icon-only sidebar button (themed Font Awesome
// icon) with a hover tooltip, since icon-only buttons need a label affordance
// (Fyne core has no tooltips — this uses the fyne-tooltip add-on).
func tipButton(icon, tip string, tapped func()) *ttwidget.Button {
	return tipButtonRes(themedIcon(icon), tip, tapped)
}

// tipButtonRes is tipButton for a pre-built icon resource (e.g. a Fyne theme
// icon like theme.SettingsIcon()) rather than a named bundled SVG.
func tipButtonRes(icon fyne.Resource, tip string, tapped func()) *ttwidget.Button {
	b := ttwidget.NewButtonWithIcon("", icon, tapped)
	b.SetToolTip(tip)
	return b
}

// thinHSplit / thinVSplit build a resizable split whose divider renders as a
// thin subtle line instead of Fyne's thick shadow bar. The thinSplit widget
// hard-codes the hairline styling, so no theme-scope overrides are needed
// (the previous splitTheme + paneTheme wrapping cost three Fyne theme scopes
// per split, each of which re-parses fonts and walks the subtree eagerly).
func thinHSplit(leading, trailing fyne.CanvasObject, offset float64) fyne.CanvasObject {
	s := newThinSplit(true, leading, trailing)
	s.SetOffset(offset)
	return s
}

func thinVSplit(top, bottom fyne.CanvasObject, offset float64) fyne.CanvasObject {
	s := newThinSplit(false, top, bottom)
	s.SetOffset(offset)
	return s
}

// flushColumn is a vertical layout with zero spacing: every row gets its
// minimum height except the row at flexIdx, which takes all remaining space.
// It replaces the old rootTheme (SizeNamePadding → 0) theme-scope override:
// the hairline separators sit flush between the toolbar, body and status bar
// without cascading a zero-padding theme over the whole widget tree.
type flushColumn struct{ flexIdx int }

func (f *flushColumn) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w, h float32
	for _, o := range objects {
		if !o.Visible() {
			continue
		}
		min := o.MinSize()
		if min.Width > w {
			w = min.Width
		}
		h += min.Height
	}
	return fyne.NewSize(w, h)
}

func (f *flushColumn) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	fixed := float32(0)
	for i, o := range objects {
		if i != f.flexIdx && o.Visible() {
			fixed += o.MinSize().Height
		}
	}
	flexH := size.Height - fixed
	if flexH < 0 {
		flexH = 0
	}
	y := float32(0)
	for i, o := range objects {
		if !o.Visible() {
			continue
		}
		h := o.MinSize().Height
		if i == f.flexIdx {
			h = flexH
		}
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(size.Width, h))
		y += h
	}
}

func methodNames() []string {
	out := make([]string, len(model.Methods))
	for i, mth := range model.Methods {
		out[i] = string(mth)
	}
	return out
}

func bodyTypeNames() []string {
	out := make([]string, len(model.BodyTypes))
	for i, b := range model.BodyTypes {
		out[i] = string(b)
	}
	return out
}

// pruneEmptyKV returns kvs with rows whose Key trims to empty removed; used at
// save time so blank "+ Add" rows the user never filled in don't reach disk.
func pruneEmptyKV(kvs []model.KeyValue) []model.KeyValue {
	out := kvs[:0]
	for _, kv := range kvs {
		if strings.TrimSpace(kv.Key) != "" {
			out = append(out, kv)
		}
	}
	return out
}

// pruneEmptyChain drops chain steps where either Alias or Request is
// blank. Returns the cleaned slice and the count of rows that had ONE
// of the two filled in (the user filled in part of a step, then saved
// without completing it). Both-blank rows are dropped silently — they
// are typically leftover "+ Add step" clicks. Callers should surface
// the half-filled count so the user notices the lost intent.
func pruneEmptyChain(steps []model.ChainStep) (cleaned []model.ChainStep, halfFilledDropped int) {
	cleaned = steps[:0]
	for _, s := range steps {
		aliasFilled := strings.TrimSpace(s.Alias) != ""
		refFilled := strings.TrimSpace(s.Request) != ""
		if aliasFilled && refFilled {
			cleaned = append(cleaned, s)
			continue
		}
		if aliasFilled || refFilled {
			halfFilledDropped++
		}
	}
	return cleaned, halfFilledDropped
}
