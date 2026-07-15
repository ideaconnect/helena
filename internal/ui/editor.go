package ui

import (
	"errors"
	"slices"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/responsefmt"
)

// formatForBodyType maps a request body type to the prettyview syntax format for
// the editable body widget: JSON/XML get structured highlighting + Reformat;
// everything else (text, form, multipart, none) renders as raw, uncoloured text.
func formatForBodyType(bt model.BodyType) prettyview.Format {
	switch bt {
	case model.BodyJSON:
		return prettyview.FormatJSON
	case model.BodyXML:
		return prettyview.FormatXML
	default:
		return prettyview.FormatRaw
	}
}

// syncBodyFromEditor copies the editor's live edit-buffer bytes into the current
// request. The widget's OnChanged is debounced, so any consumer that needs the
// authoritative body *now* (Save, Send, Validate, Format) must pull synchronously
// rather than trust the last settled callback.
func (m *MainUI) syncBodyFromEditor() {
	if m.currentRequest != nil && m.BodyContent != nil {
		m.currentRequest.Body.Content = string(m.BodyContent.Source())
	}
	// The GraphQL variables editor is debounced the same way (#70).
	if m.currentRequest != nil && m.bodyGraphQLVars != nil {
		m.currentRequest.Body.GraphQLVariables = string(m.bodyGraphQLVars.Source())
	}
}

func (m *MainUI) validateBody() {
	if m.currentRequest == nil {
		return
	}
	m.syncBodyFromEditor()
	body := []byte(m.currentRequest.Body.Content)
	switch m.currentRequest.Body.Type {
	case model.BodyJSON:
		if _, err := responsefmt.PrettyJSON(body); err != nil {
			m.Status.SetText("JSON invalid: " + err.Error())
		} else {
			m.Status.SetText("JSON is valid")
		}
	case model.BodyXML:
		// ErrNamespacedXML means the doc parsed fine (it's valid); we just won't
		// reformat it — so it must not read as "invalid".
		if _, err := responsefmt.PrettyXML(body); err != nil && !errors.Is(err, responsefmt.ErrNamespacedXML) {
			m.Status.SetText("XML invalid: " + err.Error())
		} else {
			m.Status.SetText("XML is valid")
		}
	default:
		m.Status.SetText("Validation only applies to JSON or XML bodies")
	}
}

func (m *MainUI) formatBody() {
	if m.currentRequest == nil {
		return
	}
	m.syncBodyFromEditor()
	body := []byte(m.currentRequest.Body.Content)
	var formatted string
	var err error
	switch m.currentRequest.Body.Type {
	case model.BodyJSON:
		formatted, err = responsefmt.PrettyJSON(body)
	case model.BodyXML:
		formatted, err = responsefmt.PrettyXML(body)
	default:
		m.Status.SetText("Format only applies to JSON or XML bodies")
		return
	}
	if errors.Is(err, responsefmt.ErrNamespacedXML) {
		m.Status.SetText("XML has namespaces — left unchanged (can't reformat without corrupting xmlns prefixes)")
		return
	}
	if err != nil {
		m.Status.SetText("Format failed: " + err.Error())
		return
	}
	m.currentRequest.Body.Content = formatted
	m.BodyContent.SetData([]byte(formatted), formatForBodyType(m.currentRequest.Body.Type))
	m.Status.SetText("Formatted")
}

func (m *MainUI) addParam() {
	if m.currentRequest == nil {
		return
	}
	m.currentRequest.Params = append(m.currentRequest.Params, model.KeyValue{Enabled: true})
	m.rebuildParamsRows()
}

func (m *MainUI) addHeader() {
	if m.currentRequest == nil {
		return
	}
	m.currentRequest.Headers = append(m.currentRequest.Headers, model.KeyValue{Enabled: true})
	m.rebuildHeadersRows()
}

func (m *MainUI) addBodyFormField() {
	if m.currentRequest == nil {
		return
	}
	m.currentRequest.Body.Form = append(m.currentRequest.Body.Form, model.KeyValue{Enabled: true})
	m.rebuildBodyFormRows()
}

// rebuildBodyFormRows re-creates the Body.Form KV editor rows — the
// form-urlencoded / multipart counterpart to rebuildParamsRows.
func (m *MainUI) rebuildBodyFormRows() {
	if m.bodyFormRows == nil {
		return
	}
	m.bodyFormRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Body.Form {
			m.bodyFormRows.Add(m.buildKVRow(&m.currentRequest.Body.Form, i, m.rebuildBodyFormRows, nil).obj)
		}
	}
	m.bodyFormRows.Refresh()
}

// refreshBodyEditorVisibility shows the structured Body.Form table for
// form-urlencoded / multipart bodies and the raw text editor for everything
// else, so the body tab always edits the field the send path actually uses.
func (m *MainUI) refreshBodyEditorVisibility(bt model.BodyType) {
	if m.bodyFormPanel == nil || m.BodyContent == nil {
		return
	}
	m.BodyContent.Hide()
	m.bodyFormPanel.Hide()
	if m.bodyFilePanel != nil {
		m.bodyFilePanel.Hide()
	}
	if m.bodyGraphQLPanel != nil {
		m.bodyGraphQLPanel.Hide()
	}
	switch bt {
	case model.BodyForm, model.BodyMultipart:
		m.bodyFormPanel.Show()
	case model.BodyFile:
		if m.bodyFilePanel != nil {
			m.bodyFilePanel.Show()
		}
	case model.BodyGraphQL:
		// GraphQL edits the query in BodyContent and the variables in the
		// dedicated panel beneath it — both visible at once (#70).
		m.BodyContent.Show()
		if m.bodyGraphQLPanel != nil {
			m.bodyGraphQLPanel.Show()
		}
	default:
		m.BodyContent.Show()
	}
}

// rebuildParamsRows discards the current Params editor rows and re-creates one
// per entry; needed after add/delete and after loadRequest swaps the backing
// slice.
func (m *MainUI) rebuildParamsRows() {
	m.paramsRows.RemoveAll()
	m.paramRows = m.paramRows[:0]
	if m.currentRequest != nil {
		for i := range m.currentRequest.Params {
			r := m.buildKVRow(&m.currentRequest.Params, i, m.rebuildParamsRows, m.syncURLFieldFromParams)
			m.paramRows = append(m.paramRows, r)
			m.paramsRows.Add(r.obj)
		}
	}
	m.paramsRows.Refresh()
}

// syncParamsRowsInPlace updates the existing Query row widgets to match
// m.currentRequest.Params without tearing them down — used while the user types
// in the URL field, where typically only param values/keys change, not the
// count. It returns false when the row count no longer matches, signalling the
// caller to fall back to a full rebuild (add/remove). Updates run under
// m.syncing so the entries' OnChanged handlers don't loop back into the URL.
func (m *MainUI) syncParamsRowsInPlace() bool {
	if m.currentRequest == nil || len(m.paramRows) != len(m.currentRequest.Params) {
		return false
	}
	prev := m.syncing
	m.syncing = true
	defer func() { m.syncing = prev }()
	for i, p := range m.currentRequest.Params {
		r := m.paramRows[i]
		if r.key.Text != p.Key {
			r.key.SetText(p.Key)
		}
		if r.val.Text != p.Value {
			r.val.SetText(p.Value)
		}
		if r.check.Checked != p.Enabled {
			r.check.SetChecked(p.Enabled)
		}
	}
	return true
}

// rebuildHeadersRows is the Headers-tab counterpart to rebuildParamsRows.
func (m *MainUI) rebuildHeadersRows() {
	m.headersRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Headers {
			r := m.buildKVRow(&m.currentRequest.Headers, i, m.rebuildHeadersRows, nil)
			m.headersRows.Add(r.obj)
		}
	}
	m.headersRows.Refresh()
}

// applyImpliedContentType keeps the Content-Type header in step with the body
// Type selector, but never clobbers a value the user set explicitly. It returns
// the (possibly new) header slice and whether anything changed.
//
//   - No Content-Type header: one is added (enabled) when the new type implies a
//     value (json/xml/text/form). none/multipart imply none, so nothing is added
//     — multipart's Content-Type, with its boundary, is set when the request is sent.
//   - A Content-Type header exists: it's only updated/removed when its current
//     value still equals the *previous* type's implied value — i.e. it was
//     auto-managed by this helper, not typed by the user. A custom value is left
//     untouched ("set explicitly").
func applyImpliedContentType(headers []model.KeyValue, oldType, newType model.BodyType) ([]model.KeyValue, bool) {
	oldCT := oldType.ContentType()
	newCT := newType.ContentType()
	for i := range headers {
		if !strings.EqualFold(strings.TrimSpace(headers[i].Key), "Content-Type") {
			continue
		}
		if strings.TrimSpace(headers[i].Value) != oldCT {
			return headers, false // explicit value — leave it alone
		}
		if newCT == "" {
			return append(headers[:i], headers[i+1:]...), true
		}
		headers[i].Value = newCT
		return headers, true
	}
	if newCT == "" {
		return headers, false
	}
	return append(headers, model.KeyValue{Enabled: true, Key: "Content-Type", Value: newCT}), true
}

// applyURLEdit handles a user edit of the URL field under two-way Query sync:
// it peels the query off into the bare base (kept in currentRequest.URL) and
// reparses it into the Query table, preserving any disabled rows. The URL field
// itself is left untouched so the caret/typing isn't disturbed; the send path
// still merges currentRequest.Params, so the base-only URL stays correct.
func (m *MainUI) applyURLEdit(s string) {
	base, query, frag := splitURLQuery(s)
	m.currentRequest.URL = withFragment(base, frag)
	m.currentRequest.Params = mergeQueryFromURL(m.currentRequest.Params, parseQueryParams(query))
	// Update rows in place when the param count is unchanged (the common case
	// while typing); only a genuine add/remove tears down and rebuilds (#53).
	if !m.syncParamsRowsInPlace() {
		m.rebuildParamsRows()
	}
}

// syncURLFieldFromParams rewrites the URL field to base + the query rebuilt from
// the Query table after a table edit. The syncing flag stops the field's
// OnChanged from reparsing the value straight back into the table.
func (m *MainUI) syncURLFieldFromParams() {
	if m.currentRequest == nil || m.syncing {
		return
	}
	m.syncing = true
	m.URL.SetText(displayURL(m.currentRequest.URL, m.currentRequest.Params))
	m.syncing = false
}

// kvRow bundles one KeyValue editor row's container with its editable widgets
// so callers can refresh the displayed values in place (see
// syncParamsRowsInPlace) instead of tearing the row down and rebuilding it.
type kvRow struct {
	obj   fyne.CanvasObject
	check *widget.Check
	key   *shortcutEntry
	val   *shortcutEntry
}

// buildKVRow renders one editable row of a KeyValue list. The row's widgets
// write back into list by index; the delete button removes that index and calls
// refresh to rebuild the row container.
// onChange (may be nil) fires after any edit that alters the list's contents —
// the Query tab passes syncURLFieldFromParams so edits flow back into the URL.
func (m *MainUI) buildKVRow(list *[]model.KeyValue, idx int, refresh func(), onChange func()) *kvRow {
	fire := func() {
		if onChange != nil {
			onChange()
		}
		m.refreshActiveTabDirty() // KV edits (params / headers / form) mark the tab dirty
	}
	kv := &(*list)[idx]
	// OnChanged assigned after SetChecked (like the entries below SetText) so a
	// rebuild doesn't fire callbacks while wiring rows.
	check := widget.NewCheck("", nil)
	check.SetChecked(kv.Enabled)
	check.OnChanged = func(b bool) {
		if idx < len(*list) {
			(*list)[idx].Enabled = b
		}
		fire()
	}
	keyEntry := m.newShortcutEntry()
	keyEntry.SetText(kv.Key)
	keyEntry.OnChanged = func(s string) {
		if idx < len(*list) {
			(*list)[idx].Key = s
		}
		fire()
	}
	valEntry := m.newShortcutEntry()
	valEntry.SetText(kv.Value)
	valEntry.OnChanged = func(s string) {
		if idx < len(*list) {
			(*list)[idx].Value = s
		}
		fire()
	}
	// Same affordance as the tab close button: a low-importance circle-xmark icon.
	delBtn := widget.NewButtonWithIcon("", themedIcon("circle-xmark"), func() {
		if idx < len(*list) {
			*list = slices.Delete(*list, idx, idx+1)
		}
		refresh()
		fire()
	})
	delBtn.Importance = widget.LowImportance
	obj := container.NewBorder(nil, nil, check, delBtn,
		container.NewGridWithColumns(2, keyEntry, valEntry))
	return &kvRow{obj: obj, check: check, key: keyEntry, val: valEntry}
}
