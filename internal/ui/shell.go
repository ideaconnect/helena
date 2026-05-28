package ui

import (
	"context"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/responsefmt"
	"github.com/idct/helena/internal/session"
)

// noEnv is the option shown when no environment is selected.
const noEnv = "No Environment"

// MainUI holds Helena's primary widgets and the session they are bound to.
type MainUI struct {
	sess *session.Session
	win  fyne.Window

	Workspace   *widget.Select
	Environment *widget.Select
	Method      *widget.Select
	URL         *widget.Entry
	urlPreview  *widget.Label
	Save        *widget.Button
	Send        *widget.Button
	Tree        *widget.Tree
	Request     *container.AppTabs
	Response    *container.AppTabs
	Status      *widget.Label

	paramsRows  *fyne.Container
	headersRows *fyne.Container
	BodyType    *widget.Select
	BodyContent *widget.Entry

	responseRaw *widget.Entry
	prettyText  *widget.Entry
	headersText *widget.Entry
	corsBanner  *canvas.Text

	currentRequest     *model.Request
	currentRequestID   string
	lastSelectedNodeID string
	loading            bool // suppress write-back during programmatic widget updates

	shortcuts []shortcutSpec

	root fyne.CanvasObject
}

// NewMainUI builds the main layout bound to sess and returns it ready to place
// in a window. Call SetWindow before showing so dialogs have a parent.
func NewMainUI(sess *session.Session) *MainUI {
	m := &MainUI{sess: sess}

	m.Workspace = widget.NewSelect(sess.WorkspaceNames(), m.onWorkspaceChanged)
	if names := sess.WorkspaceNames(); len(names) > 0 {
		m.Workspace.SetSelected(names[sess.ActiveIndex()])
	}

	m.Environment = widget.NewSelect([]string{noEnv}, m.onEnvChanged)
	m.Environment.SetSelected(noEnv)

	m.Method = widget.NewSelect(methodNames(), func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Method = model.Method(s)
		}
	})
	m.Method.SetSelected(string(model.GET))

	m.URL = widget.NewEntry()
	m.URL.SetPlaceHolder("https://{{base_url}}/path")
	m.URL.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.URL = s
		}
		m.updateURLPreview()
	}
	m.URL.OnSubmitted = func(_ string) { m.send() }

	m.urlPreview = widget.NewLabel("")
	m.urlPreview.TextStyle = fyne.TextStyle{Italic: true}
	m.urlPreview.Hide()

	m.Save = widget.NewButton("Save", m.saveRequest)
	m.Save.Disable()
	m.Send = widget.NewButton("Send", nil)
	m.Send.Importance = widget.HighImportance

	m.Tree = m.buildTree()

	// Request tabs: editable Params, Headers, and Body.
	m.paramsRows = container.NewVBox()
	addParamBtn := widget.NewButton("+ Add", m.addParam)
	paramsTab := container.NewBorder(nil, addParamBtn, nil, nil,
		container.NewVScroll(m.paramsRows))

	m.headersRows = container.NewVBox()
	addHeaderBtn := widget.NewButton("+ Add", m.addHeader)
	headersTab := container.NewBorder(nil, addHeaderBtn, nil, nil,
		container.NewVScroll(m.headersRows))

	m.BodyType = widget.NewSelect(bodyTypeNames(), func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Body.Type = model.BodyType(s)
		}
	})
	m.BodyType.SetSelected(string(model.BodyNone))
	m.BodyContent = widget.NewMultiLineEntry()
	m.BodyContent.Wrapping = fyne.TextWrapOff
	m.BodyContent.SetPlaceHolder("Body content — {{vars}} are resolved on Send. Validate / Format apply to JSON & XML.")
	m.BodyContent.OnChanged = func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Body.Content = s
		}
	}
	validateBtn := widget.NewButton("Validate", m.validateBody)
	formatBtn := widget.NewButton("Format", m.formatBody)
	bodyTopRow := container.NewHBox(widget.NewLabel("Type:"), m.BodyType, validateBtn, formatBtn)
	bodyTab := container.NewBorder(bodyTopRow, nil, nil, nil,
		container.NewVScroll(m.BodyContent))

	m.Request = container.NewAppTabs(
		container.NewTabItem("Params", paramsTab),
		container.NewTabItem("Headers", headersTab),
		container.NewTabItem("Body", bodyTab),
	)

	// Response tabs.
	m.responseRaw = widget.NewMultiLineEntry()
	m.responseRaw.Wrapping = fyne.TextWrapOff
	m.responseRaw.SetPlaceHolder("Raw response body appears here after you press Send.")
	m.prettyText = widget.NewMultiLineEntry()
	m.prettyText.Wrapping = fyne.TextWrapOff
	m.prettyText.SetPlaceHolder("Pretty-printed JSON / XML appears here for matching responses.")
	m.headersText = widget.NewMultiLineEntry()
	m.headersText.Wrapping = fyne.TextWrapOff
	m.headersText.SetPlaceHolder("Response headers appear here after you press Send.")
	m.Response = container.NewAppTabs(
		container.NewTabItem("Pretty", container.NewScroll(m.prettyText)),
		container.NewTabItem("Raw", container.NewScroll(m.responseRaw)),
		container.NewTabItem("Headers", container.NewScroll(m.headersText)),
	)
	m.corsBanner = canvas.NewText("", color.NRGBA{R: 0xEE, G: 0x90, B: 0x10, A: 0xFF})
	m.corsBanner.TextStyle.Bold = true
	m.corsBanner.Hide()
	m.Status = widget.NewLabel("Ready")

	m.Send.OnTapped = m.send

	wsBtn := widget.NewButton("Workspaces…", m.editWorkspaces)
	envBtn := widget.NewButton("Environments…", m.editEnvironments)
	settingsBtn := widget.NewButton("Settings…", m.editSettings)
	helpBtn := widget.NewButton("?", m.showShortcuts)
	toolbar := container.NewHBox(
		widget.NewLabel("Workspace:"), m.Workspace, wsBtn,
		widget.NewLabel("Env:"), m.Environment, envBtn,
		settingsBtn, helpBtn,
	)
	exportBtn := widget.NewButton("Export…", m.actionExport)
	saveSendBox := container.NewHBox(m.Save, exportBtn, m.Send)
	addressBar := container.NewBorder(nil, nil, m.Method, saveSendBox, m.URL)
	top := container.NewVBox(toolbar, addressBar, m.urlPreview)

	newColBtn := widget.NewButton("+ New", m.actionNewCollection)
	openBtn := widget.NewButton("Open…", m.openCollection)
	importBtn := widget.NewButton("Import…", m.actionImport)
	sidebarHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Collections", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(newColBtn, openBtn, importBtn))
	itemActions := container.NewHBox(
		widget.NewButton("+ Req", m.actionNewRequest),
		widget.NewButton("+ Folder", m.actionNewFolder),
		widget.NewButton("Rename", m.actionRename),
		widget.NewButton("Duplicate", m.actionDuplicate),
		widget.NewButton("Delete", m.actionDelete),
	)
	sidebarTop := container.NewVBox(sidebarHeader, itemActions)
	sidebar := container.NewBorder(sidebarTop, nil, nil, nil, m.Tree)

	responsePanel := container.NewBorder(m.corsBanner, nil, nil, nil, m.Response)
	editor := container.NewVSplit(m.Request, responsePanel)
	editor.SetOffset(0.5)
	body := container.NewHSplit(sidebar, editor)
	body.SetOffset(0.25)

	m.root = container.NewBorder(top, m.Status, nil, nil, body)

	m.refreshEnvironments()
	if id := sess.OpenRequest(); id != "" {
		m.Tree.Select(id)
	}
	return m
}

// Root returns the assembled root canvas object.
func (m *MainUI) Root() fyne.CanvasObject { return m.root }

// SetWindow records the parent window used for dialogs and registers the
// application keyboard shortcuts against its canvas.
func (m *MainUI) SetWindow(w fyne.Window) {
	m.win = w
	m.registerShortcuts()
}

func (m *MainUI) buildTree() *widget.Tree {
	t := widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID { return m.sess.Tree().ChildIDs(id) },
		func(id widget.TreeNodeID) bool { return m.sess.Tree().IsBranch(id) },
		func(bool) fyne.CanvasObject { return widget.NewLabel("template") },
		func(id widget.TreeNodeID, _ bool, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(m.sess.Tree().Label(id))
		},
	)
	t.OnSelected = func(id widget.TreeNodeID) {
		m.lastSelectedNodeID = id
		if ci := m.sess.Tree().CollectionIndex(id); ci >= 0 {
			m.sess.SetActiveCollection(ci)
			m.refreshEnvironments()
		}
		r, ok := m.sess.Tree().Request(id)
		if !ok {
			return
		}
		m.loadRequest(r, id)
		m.Status.SetText("Loaded: " + r.Name)
		m.sess.SetOpenRequest(id)
	}
	return t
}

// loadRequest populates every editor widget from req, with the loading flag set
// so write-back callbacks don't fire during the bulk-update.
func (m *MainUI) loadRequest(req *model.Request, id string) {
	m.loading = true
	defer func() { m.loading = false }()

	m.currentRequest = req
	m.currentRequestID = id
	if req == nil {
		m.Save.Disable()
		m.URL.SetText("")
		m.BodyContent.SetText("")
		m.paramsRows.RemoveAll()
		m.paramsRows.Refresh()
		m.headersRows.RemoveAll()
		m.headersRows.Refresh()
		m.urlPreview.Hide()
		return
	}
	m.Save.Enable()

	method := req.Method
	if method == "" {
		method = model.GET
	}
	m.Method.SetSelected(string(method))
	m.URL.SetText(req.URL)
	m.rebuildParamsRows()
	m.rebuildHeadersRows()

	bt := req.Body.Type
	if bt == "" {
		bt = model.BodyNone
	}
	m.BodyType.SetSelected(string(bt))
	m.BodyContent.SetText(req.Body.Content)
	m.updateURLPreview()
}

func (m *MainUI) saveRequest() {
	if m.currentRequest == nil {
		m.Status.SetText("No request selected")
		return
	}
	// Drop incomplete (empty-key) rows on save so we don't write noise to YAML.
	m.currentRequest.Params = pruneEmptyKV(m.currentRequest.Params)
	m.currentRequest.Headers = pruneEmptyKV(m.currentRequest.Headers)
	m.rebuildParamsRows()
	m.rebuildHeadersRows()

	if err := m.sess.SaveActiveCollection(); err != nil {
		m.Status.SetText("Save failed: " + err.Error())
		if m.win != nil {
			dialog.ShowError(err, m.win)
		}
		return
	}
	m.Status.SetText("Saved: " + m.currentRequest.Name)
}

func (m *MainUI) updateURLPreview() {
	if m.URL == nil || m.urlPreview == nil {
		return // called during construction before widgets exist
	}
	if m.URL.Text == "" {
		m.urlPreview.Hide()
		return
	}
	resolved, missing := m.sess.Resolver().Resolve(m.URL.Text)
	if resolved == m.URL.Text {
		m.urlPreview.Hide()
		return
	}
	if len(missing) > 0 {
		m.urlPreview.SetText("⚠ Unresolved: " + strings.Join(missing, ", "))
	} else {
		m.urlPreview.SetText("→ " + resolved)
	}
	m.urlPreview.Show()
}

func (m *MainUI) validateBody() {
	if m.currentRequest == nil {
		return
	}
	body := []byte(m.currentRequest.Body.Content)
	switch m.currentRequest.Body.Type {
	case model.BodyJSON:
		if _, err := responsefmt.PrettyJSON(body); err != nil {
			m.Status.SetText("JSON invalid: " + err.Error())
		} else {
			m.Status.SetText("JSON is valid")
		}
	case model.BodyXML:
		if _, err := responsefmt.PrettyXML(body); err != nil {
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
	if err != nil {
		m.Status.SetText("Format failed: " + err.Error())
		return
	}
	m.currentRequest.Body.Content = formatted
	m.BodyContent.SetText(formatted)
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

func (m *MainUI) rebuildParamsRows() {
	m.paramsRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Params {
			m.paramsRows.Add(m.buildKVRow(&m.currentRequest.Params, i, m.rebuildParamsRows))
		}
	}
	m.paramsRows.Refresh()
}

func (m *MainUI) rebuildHeadersRows() {
	m.headersRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Headers {
			m.headersRows.Add(m.buildKVRow(&m.currentRequest.Headers, i, m.rebuildHeadersRows))
		}
	}
	m.headersRows.Refresh()
}

// buildKVRow renders one editable row of a KeyValue list. The row's widgets
// write back into list by index; the delete button removes that index and calls
// refresh to rebuild the row container.
func (m *MainUI) buildKVRow(list *[]model.KeyValue, idx int, refresh func()) fyne.CanvasObject {
	kv := &(*list)[idx]
	check := widget.NewCheck("", func(b bool) {
		if idx < len(*list) {
			(*list)[idx].Enabled = b
		}
	})
	check.SetChecked(kv.Enabled)
	keyEntry := widget.NewEntry()
	keyEntry.SetText(kv.Key)
	keyEntry.OnChanged = func(s string) {
		if idx < len(*list) {
			(*list)[idx].Key = s
		}
	}
	valEntry := widget.NewEntry()
	valEntry.SetText(kv.Value)
	valEntry.OnChanged = func(s string) {
		if idx < len(*list) {
			(*list)[idx].Value = s
		}
	}
	delBtn := widget.NewButton("×", func() {
		if idx < len(*list) {
			*list = slices.Delete(*list, idx, idx+1)
		}
		refresh()
	})
	return container.NewBorder(nil, nil, check, delBtn,
		container.NewGridWithColumns(2, keyEntry, valEntry))
}

func (m *MainUI) onWorkspaceChanged(name string) {
	for i, n := range m.sess.WorkspaceNames() {
		if n == name {
			m.sess.SetActive(i)
			break
		}
	}
	if m.Tree != nil {
		m.Tree.Refresh()
	}
}

func (m *MainUI) openCollection() {
	if m.win == nil {
		return
	}
	dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
		switch {
		case err != nil:
			dialog.ShowError(err, m.win)
		case u == nil:
			// cancelled
		default:
			if err := m.sess.OpenCollection(u.Path()); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			m.Tree.Refresh()
			m.Status.SetText("Opened collection: " + u.Name())
		}
	}, m.win)
}

// send executes the active edited request (or the bare method/URL if nothing is
// selected) off the UI goroutine, resolving {{vars}} against the active env.
func (m *MainUI) send() {
	if strings.TrimSpace(m.URL.Text) == "" {
		m.Status.SetText("Enter a URL first")
		return
	}
	var req model.Request
	if m.currentRequest != nil {
		req = *m.currentRequest
	} else {
		req = model.Request{Method: model.Method(m.Method.Selected), URL: m.URL.Text}
	}
	client := httpclient.New(m.sess.Settings())
	resolver := m.sess.Resolver()

	m.Status.SetText("Sending…")
	m.Send.Disable()
	m.corsBanner.Hide()

	go func() {
		resp, err := client.Do(context.Background(), req, resolver)

		fyne.Do(func() {
			m.Send.Enable()
			if err != nil {
				m.Response.SelectIndex(1) // Raw shows the error
				m.Status.SetText("Error: " + err.Error())
				m.responseRaw.SetText(err.Error())
				m.prettyText.SetText("")
				m.headersText.SetText("")
				return
			}
			m.responseRaw.SetText(string(resp.Body))
			m.headersText.SetText(responsefmt.FormatHeaders(resp.Headers))

			pretty := ""
			ct := resp.Headers.Get("Content-Type")
			switch {
			case responsefmt.IsJSON(ct):
				if p, perr := responsefmt.PrettyJSON(resp.Body); perr == nil {
					pretty = p
				}
			case responsefmt.IsXML(ct):
				if p, perr := responsefmt.PrettyXML(resp.Body); perr == nil {
					pretty = p
				}
			}
			m.prettyText.SetText(pretty)
			if pretty != "" {
				m.Response.SelectIndex(0) // Pretty
			} else {
				m.Response.SelectIndex(1) // Raw
			}

			m.Status.SetText(fmt.Sprintf("%s · %s · %s",
				resp.Status,
				responsefmt.HumanSize(resp.Size),
				responsefmt.HumanDuration(resp.Duration)))
			if resp.CORSWarning != "" {
				m.corsBanner.Text = "⚠ CORS: " + resp.CORSWarning
				m.corsBanner.Refresh()
				m.corsBanner.Show()
			}
		})
	}()
}

func (m *MainUI) onEnvChanged(name string) {
	if name == noEnv {
		m.sess.SetActiveEnv("")
	} else {
		m.sess.SetActiveEnv(name)
	}
	m.updateURLPreview()
}

func (m *MainUI) refreshEnvironments() {
	m.Environment.Options = append([]string{noEnv}, m.sess.CollectionEnvironmentNames()...)
	sel := m.sess.ActiveEnvName()
	if sel == "" {
		sel = noEnv
	}
	m.Environment.SetSelected(sel)
	m.Environment.Refresh()
}

// editEnvironments opens a simple "key = value" editor for the active
// collection's active environment, creating one if needed, and saves changes
// back to the collection's YAML on disk.
func (m *MainUI) editEnvironments() {
	if m.win == nil {
		return
	}
	if m.sess.ActiveCollection() < 0 {
		m.Status.SetText("Open a collection first")
		return
	}
	if m.sess.ActiveEnvName() == "" {
		if names := m.sess.CollectionEnvironmentNames(); len(names) > 0 {
			m.sess.SetActiveEnv(names[0])
		} else {
			m.sess.AddEnvironment("Default")
			m.sess.SetActiveEnv("Default")
		}
	}
	env := m.sess.ActiveEnvironment()
	if env == nil {
		m.Status.SetText("No environment to edit")
		return
	}

	entry := widget.NewMultiLineEntry()
	entry.SetText(session.FormatEnvVars(env.Variables))
	entry.SetMinRowsVisible(10)
	content := container.NewBorder(
		widget.NewLabel("One per line:  key = value   (prefix with # to disable)"),
		nil, nil, nil, entry,
	)

	d := dialog.NewCustomConfirm("Environment: "+env.Name, "Save", "Cancel", content, func(ok bool) {
		if !ok {
			return
		}
		secret := map[string]bool{}
		for _, v := range env.Variables {
			if v.Secret {
				secret[v.Key] = true
			}
		}
		parsed := session.ParseEnvVars(entry.Text)
		for i := range parsed {
			if secret[parsed[i].Key] {
				parsed[i].Secret = true
			}
		}
		m.sess.SetActiveEnvironmentVariables(parsed)
		if err := m.sess.SaveActiveCollection(); err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.refreshEnvironments()
		m.updateURLPreview()
		m.Status.SetText("Saved environment: " + env.Name)
	}, m.win)
	d.Resize(fyne.NewSize(540, 400))
	d.Show()
}

// editSettings opens the Theme / SSL / CORS / redirects / timeout dialog and
// persists changes via the session.
func (m *MainUI) editSettings() {
	if m.win == nil {
		return
	}
	s := m.sess.Settings()

	insecure := widget.NewCheck("Allow invalid / self-signed TLS certificates", nil)
	insecure.SetChecked(s.InsecureSkipVerify)
	corsWarn := widget.NewCheck("Warn when a browser would block the response (CORS)", nil)
	corsWarn.SetChecked(s.CORSWarning)
	follow := widget.NewCheck("Follow redirects automatically", nil)
	follow.SetChecked(s.FollowRedirects)
	timeoutEntry := widget.NewEntry()
	timeoutEntry.SetText(strconv.Itoa(s.TimeoutSeconds))
	timeoutEntry.SetPlaceHolder("0 = no timeout")

	themeSelect := widget.NewSelect([]string{"System", "Light", "Dark"}, nil)
	themeSelect.SetSelected(themeName(s.Theme))

	form := widget.NewForm(
		widget.NewFormItem("Theme", themeSelect),
		widget.NewFormItem("TLS", insecure),
		widget.NewFormItem("CORS", corsWarn),
		widget.NewFormItem("Redirects", follow),
		widget.NewFormItem("Timeout (s)", timeoutEntry),
	)

	dlg := dialog.NewCustomConfirm("Settings", "Save", "Cancel", form, func(ok bool) {
		if !ok {
			return
		}
		t, err := strconv.Atoi(strings.TrimSpace(timeoutEntry.Text))
		if err != nil || t < 0 {
			t = 0
		}
		newTheme := themeFromName(themeSelect.Selected)
		m.sess.SetSettings(model.Settings{
			InsecureSkipVerify: insecure.Checked,
			CORSWarning:        corsWarn.Checked,
			FollowRedirects:    follow.Checked,
			TimeoutSeconds:     t,
			Theme:              newTheme,
		})
		ApplyTheme(fyne.CurrentApp(), newTheme)
		m.Status.SetText("Settings saved")
	}, m.win)
	dlg.Resize(fyne.NewSize(520, 320))
	dlg.Show()
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

func pruneEmptyKV(kvs []model.KeyValue) []model.KeyValue {
	out := kvs[:0]
	for _, kv := range kvs {
		if strings.TrimSpace(kv.Key) != "" {
			out = append(out, kv)
		}
	}
	return out
}
