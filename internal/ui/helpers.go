package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
	b := ttwidget.NewButtonWithIcon("", themedIcon(icon), tapped)
	b.SetToolTip(tip)
	return b
}

// thinHSplit / thinVSplit build a resizable split whose divider renders as a
// thin subtle line (splitTheme) instead of Fyne's thick shadow bar, while the
// panes keep normal sizing (paneTheme restores it inside the split's override).
func thinHSplit(leading, trailing fyne.CanvasObject, offset float64) fyne.CanvasObject {
	s := container.NewHSplit(
		container.NewThemeOverride(leading, paneTheme{}),
		container.NewThemeOverride(trailing, paneTheme{}),
	)
	s.SetOffset(offset)
	return container.NewThemeOverride(s, splitTheme{})
}

func thinVSplit(top, bottom fyne.CanvasObject, offset float64) fyne.CanvasObject {
	s := container.NewVSplit(
		container.NewThemeOverride(top, paneTheme{}),
		container.NewThemeOverride(bottom, paneTheme{}),
	)
	s.SetOffset(offset)
	return container.NewThemeOverride(s, splitTheme{})
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
