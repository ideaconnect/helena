package ui

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/assertion"
	"github.com/idct/helena/internal/model"
)

// assertionSourceSuggestions seeds the source SelectEntry with the common
// response expressions; users can type any res.json.<path> / res.header.<name>.
var assertionSourceSuggestions = []string{"res.status", "res.body", "res.header.Content-Type", "res.json."}

// buildAssertionsTab assembles the request-editor's Assertions tab (#88) — a
// list of (enabled, source, operator, expected) rows plus an "+ Add assertion"
// button. The rows are evaluated against the response after Send and reported
// in the Scripts console alongside test()/expect() results. Editing mirrors the
// Chain tab: each widget writes back into currentRequest.Assertions by index
// while m.loading is false.
func (m *MainUI) buildAssertionsTab() fyne.CanvasObject {
	m.assertionRows = container.NewVBox()
	addBtn := widget.NewButton("+ Add assertion", m.addAssertion)
	help := widget.NewLabelWithStyle(
		"No-code response checks. Source examples: res.status, res.body, res.header.Name, res.json.path.to.field.",
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	return container.NewBorder(help, addBtn, nil, nil, container.NewVScroll(m.assertionRows))
}

// loadAssertionsTab populates the tab from req under the m.loading guard.
func (m *MainUI) loadAssertionsTab(req *model.Request) {
	if m.assertionRows == nil {
		return
	}
	if req == nil {
		m.assertionRows.RemoveAll()
		m.assertionRows.Refresh()
		return
	}
	m.rebuildAssertionRows()
}

// addAssertion appends a blank enabled assertion (defaulting to res.status
// equals) and rebuilds the rows.
func (m *MainUI) addAssertion() {
	if m.currentRequest == nil {
		return
	}
	m.currentRequest.Assertions = append(m.currentRequest.Assertions,
		model.Assertion{Enabled: true, Source: "res.status", Op: assertion.OpEquals})
	m.rebuildAssertionRows()
}

// rebuildAssertionRows re-creates one row widget per assertion under the loading
// guard so freshly-seeded widgets don't echo back into the model.
func (m *MainUI) rebuildAssertionRows() {
	prev := m.loading
	m.loading = true
	defer func() { m.loading = prev }()
	m.assertionRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Assertions {
			m.assertionRows.Add(m.buildAssertionRow(i))
		}
	}
	m.assertionRows.Refresh()
}

// buildAssertionRow renders one assertion: an enabled check, a source
// SelectEntry, an operator Select, an expected entry, and a × delete button.
func (m *MainUI) buildAssertionRow(idx int) fyne.CanvasObject {
	row := &m.currentRequest.Assertions[idx]

	enabled := widget.NewCheck("", func(b bool) {
		if !m.loading && m.currentRequest != nil && idx < len(m.currentRequest.Assertions) {
			m.currentRequest.Assertions[idx].Enabled = b
			m.refreshActiveTabDirty()
		}
	})
	enabled.SetChecked(row.Enabled)

	source := widget.NewSelectEntry(assertionSourceSuggestions)
	source.SetPlaceHolder("res.status")
	source.SetText(row.Source)
	source.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil && idx < len(m.currentRequest.Assertions) {
			m.currentRequest.Assertions[idx].Source = s
			m.refreshActiveTabDirty()
		}
	}

	op := widget.NewSelect(assertion.Operators, func(s string) {
		if !m.loading && m.currentRequest != nil && idx < len(m.currentRequest.Assertions) {
			m.currentRequest.Assertions[idx].Op = s
			m.refreshActiveTabDirty()
		}
	})
	if row.Op != "" {
		op.SetSelected(row.Op)
	} else {
		op.SetSelected(assertion.OpEquals)
	}

	expected := widget.NewEntry()
	expected.SetPlaceHolder("expected")
	expected.SetText(row.Expected)
	expected.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil && idx < len(m.currentRequest.Assertions) {
			m.currentRequest.Assertions[idx].Expected = s
			m.refreshActiveTabDirty()
		}
	}

	del := widget.NewButton("×", func() {
		if idx < len(m.currentRequest.Assertions) {
			m.currentRequest.Assertions = slices.Delete(m.currentRequest.Assertions, idx, idx+1)
		}
		m.rebuildAssertionRows()
	})

	return container.NewBorder(nil, nil, enabled, del,
		container.NewGridWithColumns(3, source, op, expected))
}
