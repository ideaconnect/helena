package ui

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/model"
)

// buildChainTab assembles the request-editor's Chain tab — a vertical
// list of (Alias, Request path) row pairs plus a "+ Add step" button.
// Each step names another request in the same collection by its
// slash-separated path (e.g. "Auth/Login"); the chain runner executes
// each before this request, binding the named alias on `chain.<alias>`
// for both the pre- and post-response scripts.
//
// Editing semantics mirror the Params and Headers tabs: each
// widget's OnChanged writes back into currentRequest.Chain by index
// while m.loading is false, and rebuildChainRows redraws the rows
// after add / delete / load.
func (m *MainUI) buildChainTab() fyne.CanvasObject {
	m.chainRows = container.NewVBox()
	addBtn := widget.NewButton("+ Add step", m.addChainStep)
	help := widget.NewLabelWithStyle(
		"Run another request before this one. Reference it by name path (e.g. \"Auth/Login\").",
		fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	header := container.NewVBox(help)
	return container.NewBorder(header, addBtn, nil, nil, container.NewVScroll(m.chainRows))
}

// loadChainTab populates the Chain tab from req. Called under the
// m.loading guard so OnChanged callbacks on the freshly created row
// widgets don't echo the loaded values back into currentRequest.
func (m *MainUI) loadChainTab(req *model.Request) {
	if m.chainRows == nil {
		return
	}
	if req == nil {
		m.chainRows.RemoveAll()
		m.chainRows.Refresh()
		return
	}
	m.rebuildChainRows()
}

// addChainStep appends a blank step and rebuilds the rows.
func (m *MainUI) addChainStep() {
	if m.currentRequest == nil {
		return
	}
	m.currentRequest.Chain = append(m.currentRequest.Chain, model.ChainStep{})
	m.rebuildChainRows()
}

// rebuildChainRows discards the current row widgets and re-creates one
// per ChainStep. Needed after add / delete and after loadRequest swaps
// the backing slice.
func (m *MainUI) rebuildChainRows() {
	// Seed rows under the loading guard so re-creating them (after add/delete)
	// never re-runs the ref OnChanged's RequestID derivation against a possibly
	// stale path — which would silently drop an ID pin on an unrelated row.
	prev := m.loading
	m.loading = true
	defer func() { m.loading = prev }()
	m.chainRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Chain {
			m.chainRows.Add(m.buildChainRow(i))
		}
	}
	m.chainRows.Refresh()
}

// buildChainRow renders one row of the Chain editor: an Alias entry on
// the left, a Request-path SelectEntry on the right, and a × delete
// button. The ref widget is a SelectEntry (editable combo) seeded with
// every other request in the active collection so users get
// path autocomplete instead of finding their typos at Send time.
// Writes back into currentRequest.Chain by index; idx is captured by
// each OnChanged closure.
func (m *MainUI) buildChainRow(idx int) fyne.CanvasObject {
	step := &m.currentRequest.Chain[idx]
	alias := widget.NewEntry()
	alias.SetPlaceHolder("alias")
	alias.SetText(step.Alias)
	alias.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil && idx < len(m.currentRequest.Chain) {
			m.currentRequest.Chain[idx].Alias = s
		}
	}
	ref := widget.NewSelectEntry(m.chainRefSuggestions())
	ref.SetPlaceHolder("Folder/Request name")
	ref.SetText(step.Request)
	ref.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil && idx < len(m.currentRequest.Chain) {
			m.currentRequest.Chain[idx].Request = s
			// Pin RequestID when the typed text matches a known path so
			// the chain ref survives the target being renamed. A
			// non-matching value clears the ID; resolution then falls
			// back to the literal path the user typed.
			if id, ok := m.sess.RequestIDForPath(s); ok {
				m.currentRequest.Chain[idx].RequestID = id
			} else {
				m.currentRequest.Chain[idx].RequestID = ""
			}
		}
	}
	del := widget.NewButton("×", func() {
		if idx < len(m.currentRequest.Chain) {
			m.currentRequest.Chain = slices.Delete(m.currentRequest.Chain, idx, idx+1)
		}
		m.rebuildChainRows()
	})
	return container.NewBorder(nil, nil, nil, del,
		container.NewGridWithColumns(2, alias, ref))
}

// chainRefSuggestions returns every other request's chain-ref path in
// the active collection, excluding the request the user is currently
// editing (that one would form an obvious cycle).
func (m *MainUI) chainRefSuggestions() []string {
	all := m.sess.AllRequestPaths()
	if m.currentRequest == nil || m.currentRequestID == "" {
		return all
	}
	out := make([]string, 0, len(all))
	for _, p := range all {
		// Exclude only the request being edited itself, matched by IDENTITY
		// (its resolved Request.ID), not by bare name — a different request that
		// merely shares this one's name in another folder is a legitimate target
		// and must stay suggestable. Cycles beyond the trivial self-reference are
		// caught at resolve time.
		if id, ok := m.sess.RequestIDForPath(p); ok && id == m.currentRequestID {
			continue
		}
		out = append(out, p)
	}
	return out
}
