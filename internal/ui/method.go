package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// methodColor returns the brand-spec color for an HTTP method, or a
// neutral fallback for unknown methods. Used both by the method
// picker on the address bar and by the tree row renderer.
//
// Palette:
//
//	GET     #53d060
//	POST    #8ec2ec
//	PUT     #e7ae80
//	DELETE  #c56956
//	PATCH   #e0a97c
//	OPTIONS #86ddc9
//	HEAD    #9cdcf0
//	TRACE   #c2c2c2
//	CONNECT #c2c2c2 (same as TRACE)
func methodColor(m string) color.Color {
	switch model.Method(m) {
	case model.GET:
		return color.NRGBA{R: 0x53, G: 0xd0, B: 0x60, A: 0xff}
	case model.POST:
		return color.NRGBA{R: 0x8e, G: 0xc2, B: 0xec, A: 0xff}
	case model.PUT:
		return color.NRGBA{R: 0xe7, G: 0xae, B: 0x80, A: 0xff}
	case model.DELETE:
		return color.NRGBA{R: 0xc5, G: 0x69, B: 0x56, A: 0xff}
	case model.PATCH:
		return color.NRGBA{R: 0xe0, G: 0xa9, B: 0x7c, A: 0xff}
	case model.OPTIONS:
		return color.NRGBA{R: 0x86, G: 0xdd, B: 0xc9, A: 0xff}
	case model.HEAD:
		return color.NRGBA{R: 0x9c, G: 0xdc, B: 0xf0, A: 0xff}
	case model.TRACE, model.CONNECT:
		return color.NRGBA{R: 0xc2, G: 0xc2, B: 0xc2, A: 0xff}
	default:
		return theme.Color(theme.ColorNameForeground)
	}
}

// methodPicker is the address-bar HTTP method selector. It shows the
// currently-chosen method in its brand color + bold text, and pops
// up a chooser menu on tap. Built as a custom Fyne widget rather
// than wrapping widget.Select because Select can't restyle the
// rendered text per option (theme-driven only).
type methodPicker struct {
	widget.BaseWidget
	text     *canvas.Text
	options  []string
	selected string
	onChange func(string)
}

// newMethodPicker constructs the picker bound to onChange (fires with
// the newly-selected method). Caller seeds the initial value via
// SetSelected.
func newMethodPicker(onChange func(string)) *methodPicker {
	t := canvas.NewText("GET", methodColor(string(model.GET)))
	t.TextStyle = fyne.TextStyle{Bold: true}
	t.TextSize = theme.TextSize() + 2
	p := &methodPicker{
		text:     t,
		options:  methodNames(),
		selected: string(model.GET),
		onChange: onChange,
	}
	p.ExtendBaseWidget(p)
	return p
}

// Selected returns the currently-chosen method.
func (p *methodPicker) Selected() string { return p.selected }

// SetSelected updates the displayed method and re-tints the text.
// Does NOT fire onChange — programmatic updates (e.g. loading a
// request from the tree) shouldn't loop back into write-back code.
func (p *methodPicker) SetSelected(s string) {
	if s == "" {
		s = string(model.GET)
	}
	p.selected = s
	p.text.Text = s
	p.text.Color = methodColor(s)
	p.text.Refresh()
}

// Tapped pops up the method chooser menu at the picker's bottom-left.
func (p *methodPicker) Tapped(_ *fyne.PointEvent) {
	canv := fyne.CurrentApp().Driver().CanvasForObject(p)
	if canv == nil {
		return
	}
	items := make([]*fyne.MenuItem, 0, len(p.options))
	for _, opt := range p.options {
		o := opt // capture
		items = append(items, fyne.NewMenuItem(o, func() {
			if p.selected == o {
				return
			}
			p.SetSelected(o)
			if p.onChange != nil {
				p.onChange(o)
			}
		}))
	}
	popUp := widget.NewPopUpMenu(fyne.NewMenu("", items...), canv)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(p)
	popUp.ShowAtPosition(fyne.NewPos(pos.X, pos.Y+p.Size().Height))
}

// CreateRenderer wraps the canvas.Text in a padded container so the
// picker has tap padding around the brand-colored method text.
func (p *methodPicker) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewPadded(p.text))
}
