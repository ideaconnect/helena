package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/pathparam"
	"github.com/idct/helena/internal/vars"
)

// pathParamsHelpText is shown on the Path tab when the URL has no {name}
// placeholders, so an empty tab explains how path parameters are declared.
const pathParamsHelpText = "Path parameters fill the {name} placeholders in the URL's path " +
	"(e.g. {{base_url}}/users/{id}). Add a {name} to the URL to define one here."

// buildPathParamsTab builds the request editor's "Path" tab. Path parameters
// are the single-brace {name} placeholders in the URL's path; unlike the free
// Query/Headers tables the rows are DERIVED from the URL — one per placeholder,
// rebuilt by rebuildPathParamRows on load and on every URL edit — so the user
// fills a value without hand-editing the raw URL.
func (m *MainUI) buildPathParamsTab() fyne.CanvasObject {
	m.pathParamsHelp = widget.NewLabel(pathParamsHelpText)
	m.pathParamsHelp.Wrapping = fyne.TextWrapWord
	m.pathParamsRows = container.NewVBox()
	return container.NewBorder(m.pathParamsHelp, nil, nil, nil,
		container.NewVScroll(m.pathParamsRows))
}

// rebuildPathParamRows re-derives the Path tab's rows from the current URL's
// {name} tokens, prefilling each value from currentRequest.PathParams. It never
// mutates PathParams — merely opening a request and viewing the tab must leave
// the on-disk form untouched — so a value is written back only when the user
// actually edits a row (see upsertPathParam).
func (m *MainUI) rebuildPathParamRows() {
	if m.pathParamsRows == nil {
		return
	}
	m.pathParamsRows.RemoveAll()
	var names []string
	if m.currentRequest != nil {
		names = pathparam.Names(m.currentRequest.URL)
	}
	if len(names) == 0 {
		m.pathParamsHelp.Show()
		m.pathParamsRows.Refresh()
		return
	}
	m.pathParamsHelp.Hide()
	for _, name := range names {
		m.pathParamsRows.Add(m.buildPathParamRow(name))
	}
	m.pathParamsRows.Refresh()
}

// buildPathParamRow renders one path-parameter row: an enable check, the fixed
// placeholder name, and an editable value. Edits upsert into PathParams by name.
// OnChanged handlers are wired AFTER the initial SetText/SetChecked so a rebuild
// doesn't fire write-backs while loading.
func (m *MainUI) buildPathParamRow(name string) fyne.CanvasObject {
	kv, found := m.lookupPathParam(name)
	enabled := true // a not-yet-filled placeholder is enabled by default
	if found {
		enabled = kv.Enabled
	}

	check := widget.NewCheck("", nil)
	check.SetChecked(enabled)
	check.OnChanged = func(b bool) {
		if m.loading {
			return
		}
		m.upsertPathParam(name, func(p *model.KeyValue) { p.Enabled = b })
		m.updateURLPreview()
	}

	nameLabel := widget.NewLabel(name)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	valEntry := m.newShortcutEntry()
	valEntry.SetText(kv.Value)
	if kv.Description != "" {
		valEntry.SetPlaceHolder(kv.Description)
	} else {
		valEntry.SetPlaceHolder("value")
	}
	valEntry.OnChanged = func(s string) {
		if m.loading {
			return
		}
		m.upsertPathParam(name, func(p *model.KeyValue) { p.Value = s })
		m.updateURLPreview()
	}

	return container.NewBorder(nil, nil, container.NewHBox(check, nameLabel), nil, valEntry)
}

// lookupPathParam returns the stored path parameter for name (and whether one
// exists) from the loaded request.
func (m *MainUI) lookupPathParam(name string) (model.KeyValue, bool) {
	if m.currentRequest == nil {
		return model.KeyValue{}, false
	}
	for _, p := range m.currentRequest.PathParams {
		if p.Key == name {
			return p, true
		}
	}
	return model.KeyValue{}, false
}

// filledPathParam returns the resolved value for path parameter name when an
// enabled entry exists with a non-empty (post-{{var}}-resolution) value. It
// mirrors httpclient.Build's fill logic so the URL preview matches what the
// send path will actually put on the wire.
func (m *MainUI) filledPathParam(res *vars.Resolver, name string) (string, bool) {
	if m.currentRequest == nil {
		return "", false
	}
	for _, p := range m.currentRequest.PathParams {
		if !p.Enabled || p.Key != name {
			continue
		}
		if v, _ := res.Resolve(p.Value); v != "" {
			return v, true
		}
		return "", false
	}
	return "", false
}

// upsertPathParam applies mut to the PathParams entry named name, creating an
// enabled entry when none exists yet. The Path tab's write-backs route through
// here so a value/enabled edit lands on the loaded request and persists on Save.
func (m *MainUI) upsertPathParam(name string, mut func(*model.KeyValue)) {
	if m.currentRequest == nil {
		return
	}
	for i := range m.currentRequest.PathParams {
		if m.currentRequest.PathParams[i].Key == name {
			mut(&m.currentRequest.PathParams[i])
			return
		}
	}
	p := model.KeyValue{Enabled: true, Key: name}
	mut(&p)
	m.currentRequest.PathParams = append(m.currentRequest.PathParams, p)
}
