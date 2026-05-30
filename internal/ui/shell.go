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

	"github.com/idct/helena/assets"
	"github.com/idct/helena/internal/auth"
	"github.com/idct/helena/internal/chain"
	"github.com/idct/helena/internal/httpclient"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/responsefmt"
	"github.com/idct/helena/internal/scripting"
	"github.com/idct/helena/internal/session"
	"github.com/idct/helena/internal/vars"
)

// sessionEnvBridge adapts *session.Session to scripting.EnvBridge so the
// scripting package stays decoupled from internal/session.
//
// `base` is a snapshot of the active environment captured on the UI
// goroutine at Send entry; the script reads through it without
// touching live session state, which avoids racing against UI-thread
// mutations of s.cols / s.activeCol / Environment.Variables during a
// long-running script. `s` is still consulted for overlay writes and
// reads — the overlay carries its own RWMutex so live access is safe.
//
// Get returns overlay-over-snapshot. Set writes only to the session's
// in-memory overlay (invariant 9 — script-set vars never touch disk).
type sessionEnvBridge struct {
	s    *session.Session
	base map[string]string
}

func (b sessionEnvBridge) Get(name string) (string, bool) {
	if v, ok := b.s.EnvOverlay(name); ok {
		return v, true
	}
	v, ok := b.base[name]
	return v, ok
}

func (b sessionEnvBridge) Set(name, value string) { b.s.SetEnvOverlay(name, value) }

// nilFinder is a chain.RequestFinder used when no collection is loaded —
// every lookup returns false. Lets chain.Resolve still walk the leaf's
// (empty) chain without crashing.
type nilFinder struct{}

func (nilFinder) FindRequestByPath(string) (model.Request, bool) {
	return model.Request{}, false
}

func (nilFinder) FindRequestByID(string) (model.Request, bool) {
	return model.Request{}, false
}

// chainExecutor is the single execution path for both chain steps and
// the leaf: deep-copy the request's KV slices, run the pre-script with
// the supplied chain map bound as `chain.<alias>`, build the resolver
// from the captured env snapshot + a fresh overlay snapshot, call
// client.Do, then run the post-script. The captured fields close over
// the per-Send constants (rt, client, envSnap, sess) so successive
// ExecuteOnce calls don't repeat the boilerplate.
type chainExecutor struct {
	rt      *scripting.Runtime
	client  *httpclient.Client
	envSnap map[string]string
	sess    *session.Session
}

func (e chainExecutor) ExecuteOnce(ctx context.Context, r model.Request, chainMap map[string]chain.View) (chain.View, []string, error) {
	// Deep-copy slices to insulate the parent request's data from
	// per-step writeback or chain-time mutations.
	r.Params = append([]model.KeyValue(nil), r.Params...)
	r.Headers = append([]model.KeyValue(nil), r.Headers...)
	r.Body.Form = append([]model.KeyValue(nil), r.Body.Form...)

	scriptChain := chainViewToScripting(chainMap)
	var console []string

	preRes, preErr := e.rt.RunPreRequest(ctx, r.Scripts.PreRequest, &r, scriptChain)
	console = append(console, preRes.Console...)
	if preErr != nil {
		return chain.View{}, console, fmt.Errorf("pre-script: %w", preErr)
	}

	resolver := vars.New(e.envSnap, e.sess.SnapshotEnvOverlay())
	resp, err := e.client.Do(ctx, r, resolver)
	if err != nil {
		return chain.View{}, console, err
	}

	// view.Request reflects what actually went on the wire: the
	// resolved URL (with {{vars}} substituted and query params merged)
	// and the encoded body bytes (URL-encoded form for form-urlencoded,
	// multipart envelope for multipart, raw bytes otherwise). Both
	// come from httpclient via Response so scripts reading
	// chain.<alias>.request.{url,body} see the wire form, not the
	// pre-resolution template.
	view := chain.View{
		Request: chain.RequestView{Method: string(r.Method), URL: resp.RequestURL, Body: resp.RequestBody},
		Response: chain.ResponseView{
			StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Headers,
			Body: resp.Body, Size: resp.Size, Duration: resp.Duration, CORSWarning: resp.CORSWarning,
		},
	}

	postRes, postErr := e.rt.RunPostResponse(ctx, r.Scripts.PostResponse, r,
		scripting.ResponseInput{StatusCode: resp.StatusCode, Status: resp.Status, Headers: resp.Headers, Body: resp.Body},
		scriptChain)
	console = append(console, postRes.Console...)
	if postErr != nil {
		return view, console, fmt.Errorf("post-script: %w", postErr)
	}
	return view, console, nil
}

// chainViewToScripting bridges chain.View to scripting.ChainView so
// the scripting package stays unaware of internal/chain. They carry
// the same data with different field names by design (chain.View has
// display fields the scripting surface deliberately doesn't expose).
func chainViewToScripting(chainMap map[string]chain.View) map[string]scripting.ChainView {
	if len(chainMap) == 0 {
		return nil
	}
	out := make(map[string]scripting.ChainView, len(chainMap))
	for alias, v := range chainMap {
		out[alias] = scripting.ChainView{
			Request: scripting.ChainRequestView{Method: v.Request.Method, URL: v.Request.URL, Body: v.Request.Body},
			Response: scripting.ResponseInput{
				StatusCode: v.Response.StatusCode, Status: v.Response.Status,
				Headers: v.Response.Headers, Body: v.Response.Body,
			},
		}
	}
	return out
}

// noEnv is the option shown when no environment is selected.
const noEnv = "No Environment"

// MainUI holds Helena's primary widgets and the session they are bound to.
type MainUI struct {
	sess *session.Session
	win  fyne.Window

	Workspace   *widget.Select
	Environment *widget.Select
	Method      *methodPicker
	URL         *widget.Entry
	urlPreview  *widget.Label
	Save        *widget.Button
	Send        *widget.Button
	Tree        *widget.Tree
	Request     *container.AppTabs
	Response    *container.AppTabs
	Status      *widget.Label

	paramsRows       *fyne.Container
	headersRows      *fyne.Container
	BodyType         *widget.Select
	BodyContent      *widget.Entry
	docsEditor       *widget.Entry
	docsPreview      *widget.RichText
	preScriptEditor  *widget.Entry
	postScriptEditor *widget.Entry
	scriptConsole    *widget.Entry
	chainRows        *fyne.Container

	authType                                                          *widget.Select
	authBasicUsername, authBasicPassword                              *widget.Entry
	authBearerToken                                                   *widget.Entry
	authAPIKeyName, authAPIKeyValue                                   *widget.Entry
	authAPIKeyPlacement                                               *widget.Select
	authOAuth2Grant                                                   *widget.Select
	authOAuth2TokenURL, authOAuth2AuthURL                             *widget.Entry
	authOAuth2ClientID, authOAuth2ClientSecret, authOAuth2Scope       *widget.Entry
	authOAuth2RedirectURI, authOAuth2Audience                         *widget.Entry
	authOAuth2UsePKCE                                                 *widget.Check
	authOAuth2ClearTokens                                             *widget.Button
	authInheritLabel                                                  *widget.Label
	authNonePanel, authInheritPanel                                   *fyne.Container
	authBasicPanel, authBearerPanel, authAPIKeyPanel, authOAuth2Panel *widget.Form
	authFormsStack                                                    *fyne.Container

	responseRaw *widget.Entry
	prettyText  *widget.Entry
	headersText *widget.Entry
	corsBanner  *canvas.Text

	currentRequest     *model.Request
	currentRequestID   string
	lastSelectedNodeID string
	loading            bool // suppress write-back during programmatic widget updates

	// Sidebar action buttons. addReqBtn / addFolderBtn need to disable
	// when there's no valid parent (no active collection); refreshTreeActions
	// keeps them in sync with the tree selection + active collection.
	addReqBtn    *widget.Button
	addFolderBtn *widget.Button

	// sendCancel is non-nil while a Send goroutine is in flight; the
	// Send button doubles as Abort in that state. Set + cleared on the
	// UI thread only; the cancel func itself is goroutine-safe.
	sendCancel context.CancelFunc

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

	m.Method = newMethodPicker(func(s string) {
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
	// Send shows just the send-diagonal-solid icon by default;
	// abort mode swaps to text "Abort" with warning importance.
	m.Send = widget.NewButtonWithIcon("", assets.Icon("send-diagonal-solid"), nil)
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
		container.NewTabItem("Auth", m.buildAuthTab()),
		container.NewTabItem("Headers", headersTab),
		container.NewTabItem("Body", bodyTab),
		container.NewTabItem("Scripts", m.buildScriptsTab()),
		container.NewTabItem("Chain", m.buildChainTab()),
		container.NewTabItem("Docs", m.buildDocsTab()),
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

	m.Send.OnTapped = m.sendOrAbort

	wsBtn := widget.NewButton("Workspaces…", m.editWorkspaces)
	envBtn := widget.NewButton("Variables…", m.editEnvironments)
	settingsBtn := widget.NewButton("Settings…", m.editSettings)
	helpBtn := widget.NewButton("?", m.showShortcuts)
	toolbar := container.NewHBox(
		widget.NewLabel("Workspace:"), m.Workspace, wsBtn,
		widget.NewLabel("Environment:"), m.Environment, envBtn,
		settingsBtn, helpBtn,
	)
	exportBtn := widget.NewButton("Export…", m.actionExport)
	saveSendBox := container.NewHBox(m.Save, exportBtn, m.Send)
	addressBar := container.NewBorder(nil, nil, m.Method, saveSendBox, m.URL)
	// The address bar + URL preview live in the editor column (not the
	// global top bar) so the sidebar runs floor-to-ceiling on the left
	// and the URL bar starts exactly where the request editor starts.
	editorTop := container.NewVBox(addressBar, m.urlPreview)

	newColBtn := widget.NewButton("+ New", m.actionNewCollection)
	openBtn := widget.NewButton("Open…", m.openCollection)
	importBtn := widget.NewButton("Import…", m.actionImport)
	sidebarHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Collections", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(newColBtn, openBtn, importBtn))
	m.addReqBtn = widget.NewButton("+ Req", m.actionNewRequest)
	m.addFolderBtn = widget.NewButton("+ Folder", m.actionNewFolder)
	// Rename / Duplicate / Delete moved to per-row tree icons (9.12).
	// Only the add affordances live in the sidebar header now; the
	// keyboard shortcuts still call actionRename / actionDelete /
	// actionDuplicate against the last-selected node.
	itemActions := container.NewHBox(
		m.addReqBtn,
		m.addFolderBtn,
	)
	sidebarTop := container.NewVBox(sidebarHeader, itemActions)
	sidebar := container.NewBorder(sidebarTop, nil, nil, nil, m.Tree)

	responsePanel := container.NewBorder(m.corsBanner, nil, nil, nil, m.Response)
	editor := container.NewVSplit(m.Request, responsePanel)
	editor.SetOffset(0.5)
	// Editor column carries its own address bar so the sidebar runs
	// full-height to the left of it (9.1).
	editorColumn := container.NewBorder(editorTop, nil, nil, nil, editor)
	body := container.NewHSplit(sidebar, editorColumn)
	body.SetOffset(0.25)

	m.root = container.NewBorder(toolbar, m.Status, nil, nil, body)

	m.refreshEnvironments()
	m.refreshTreeActions()
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
	actions := rowActions{
		onRename:    m.renameNode,
		onDelete:    m.deleteNode,
		onDuplicate: m.duplicateNode,
	}
	t := widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID { return m.sess.Tree().ChildIDs(id) },
		func(id widget.TreeNodeID) bool { return m.sess.Tree().IsBranch(id) },
		func(bool) fyne.CanvasObject { return newTreeRow(actions) },
		func(id widget.TreeNodeID, _ bool, o fyne.CanvasObject) {
			row := o.(*treeRow)
			if r, ok := m.sess.Tree().Request(id); ok {
				row.setRequest(id, string(r.Method), r.Name)
			} else {
				row.setBranch(id, m.sess.Tree().Label(id))
			}
		},
	)
	t.OnSelected = func(id widget.TreeNodeID) {
		m.lastSelectedNodeID = id
		if ci := m.sess.Tree().CollectionIndex(id); ci >= 0 {
			m.sess.SetActiveCollection(ci)
			m.refreshEnvironments()
		}
		m.refreshTreeActions()
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

// refreshTreeActions enables/disables the sidebar +Req / +Folder
// buttons based on whether parentForNew() can produce a valid parent
// (i.e., either the user has selected a tree node or there's an
// active collection to add to). Called from Tree.OnSelected,
// workspace switches, and the collection open / close paths.
func (m *MainUI) refreshTreeActions() {
	if m.addReqBtn == nil || m.addFolderBtn == nil {
		return // called before sidebar built
	}
	if m.parentForNew() == "" {
		m.addReqBtn.Disable()
		m.addFolderBtn.Disable()
	} else {
		m.addReqBtn.Enable()
		m.addFolderBtn.Enable()
	}
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
		if m.docsEditor != nil {
			m.docsEditor.SetText("")
		}
		m.refreshDocsPreview()
		m.loadAuthTab(nil)
		m.loadScriptsTab(nil)
		m.loadChainTab(nil)
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
	if m.docsEditor != nil {
		m.docsEditor.SetText(req.Docs)
	}
	m.refreshDocsPreview()
	m.loadAuthTab(req)
	m.loadScriptsTab(req)
	m.loadChainTab(req)
	m.updateURLPreview()
}

// saveRequest writes the currently edited request back to disk through the
// session, pruning empty-key rows so the YAML stays clean.
func (m *MainUI) saveRequest() {
	if m.currentRequest == nil {
		m.Status.SetText("No request selected")
		return
	}
	// Drop incomplete (empty-key) rows on save so we don't write noise to YAML.
	m.currentRequest.Params = pruneEmptyKV(m.currentRequest.Params)
	m.currentRequest.Headers = pruneEmptyKV(m.currentRequest.Headers)
	cleanedChain, halfFilledChain := pruneEmptyChain(m.currentRequest.Chain)
	m.currentRequest.Chain = cleanedChain
	m.rebuildParamsRows()
	m.rebuildHeadersRows()
	m.rebuildChainRows()

	if err := m.sess.SaveActiveCollection(); err != nil {
		m.Status.SetText("Save failed: " + err.Error())
		if m.win != nil {
			dialog.ShowError(err, m.win)
		}
		return
	}
	status := "Saved: " + m.currentRequest.Name
	if halfFilledChain > 0 {
		// Half-filled chain rows are dropped on save (the runner couldn't
		// use them anyway). Surface the count so the user notices the
		// lost intent and can refill them on the next edit.
		status += fmt.Sprintf(" · dropped %d incomplete chain row(s)", halfFilledChain)
	}
	m.Status.SetText(status)
}

// updateURLPreview shows the resolved URL beneath the entry whenever the raw
// text differs from its substituted form, surfacing unresolved {{vars}} as a
// warning so users notice missing environment values before sending.
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

// rebuildParamsRows discards the current Params editor rows and re-creates one
// per entry; needed after add/delete and after loadRequest swaps the backing
// slice.
func (m *MainUI) rebuildParamsRows() {
	m.paramsRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Params {
			m.paramsRows.Add(m.buildKVRow(&m.currentRequest.Params, i, m.rebuildParamsRows))
		}
	}
	m.paramsRows.Refresh()
}

// rebuildHeadersRows is the Headers-tab counterpart to rebuildParamsRows.
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

// onWorkspaceChanged is the Workspace dropdown's selection handler; it tells
// the session which workspace is now active and refreshes the tree so the
// sidebar shows that workspace's collections.
func (m *MainUI) onWorkspaceChanged(name string) {
	for i, n := range m.sess.WorkspaceNames() {
		if n == name {
			m.sess.SetActive(i)
			break
		}
	}
	// A workspace switch invalidates the selected node and the active
	// collection. Reset selection state + refresh dependent widgets so
	// the +Req / +Folder buttons and the Variables dialog read the
	// new workspace's session, not a stale closure.
	m.lastSelectedNodeID = ""
	if m.Tree != nil {
		m.Tree.UnselectAll()
		m.Tree.Refresh()
	}
	m.refreshTreeActions()
	m.refreshEnvironments()
}

// openCollection shows a folder picker and asks the session to load whatever
// directory the user chooses.
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
			m.refreshTreeActions()
			m.refreshEnvironments()
			m.Status.SetText("Opened collection: " + u.Name())
		}
	}, m.win)
}

// send executes the active edited request (or the bare method/URL if nothing is
// selected) off the UI goroutine, resolving {{vars}} against the active env.
// The pre-request script runs before httpclient builds the *http.Request so
// the script can mutate URL / method / body / headers / params; the
// post-response script runs once the response body is read so it can write
// extracted values into the session env overlay.
// sendOrAbort is the Send button's OnTapped handler. When a Send is in
// flight (sendCancel != nil), it cancels the in-flight context — the
// goroutine drains and the fyne.Do path resets the button via
// resetSendButton. Otherwise it starts a fresh Send. Both branches run
// on the UI thread; cancel() is idempotent so a double-tap is harmless.
func (m *MainUI) sendOrAbort() {
	if m.sendCancel != nil {
		m.sendCancel()
		return
	}
	m.send()
}

// resetSendButton restores the Send button to its default appearance
// and clears the abort state. UI-thread only — every Send teardown
// (success, error, panic, abort) routes through this helper inside a
// fyne.Do block.
func (m *MainUI) resetSendButton() {
	m.sendCancel = nil
	m.Send.SetIcon(assets.Icon("send-diagonal-solid"))
	m.Send.SetText("")
	m.Send.Importance = widget.HighImportance
	m.Send.Refresh()
}

func (m *MainUI) send() {
	if m.sendCancel != nil {
		// A Send is already in flight — Enter / URL OnSubmitted reach
		// this path directly (bypassing sendOrAbort), so guard here to
		// avoid leaking the in-flight cancel func when the field gets
		// overwritten. The button-tap dispatch goes through
		// sendOrAbort and would Abort instead.
		return
	}
	if strings.TrimSpace(m.URL.Text) == "" {
		m.Status.SetText("Enter a URL first")
		return
	}
	var req model.Request
	if m.currentRequest != nil {
		req = *m.currentRequest
		// Flatten any Inherit on the in-memory request copy via the session's
		// ancestor walk so httpclient sees the concrete auth.
		req.Auth = m.sess.EffectiveAuth(m.currentRequestID)
	} else {
		req = model.Request{Method: model.Method(m.Method.Selected()), URL: m.URL.Text}
	}
	// Snapshot env vars + auth state on the UI goroutine so the worker
	// goroutine can run for ~ScriptTimeout without racing against UI
	// mutations of s.cols / s.activeCol / Environment.Variables. The
	// per-Send deep-copy of slice fields happens inside ExecuteOnce so
	// chain steps get the same insulation.
	envSnap := m.sess.SnapshotActiveEnvVars()

	client := httpclient.New(m.sess.Settings())
	client.SetOAuth2Resolver(auth.NewOAuth2Resolver(
		m.sess.TokenCache(),
		nil, // default http.Client; settings-derived TLS/timeout intentionally not applied to the token endpoint
		m.sess.ActiveCollectionDir(),
		newAuthCodeStarter(),
	))
	rt := scripting.New(sessionEnvBridge{s: m.sess, base: envSnap})
	exec := chainExecutor{rt: rt, client: client, envSnap: envSnap, sess: m.sess}
	// Snapshot the active collection on the UI goroutine so the
	// chain runner reads from a frozen-at-Send-entry copy with
	// pre-flattened Auth — never races against UI-thread tree edits
	// and never sends a chain step with AuthInherit.
	var finder chain.RequestFinder = nilFinder{}
	if snap := m.sess.SnapshotChainFinder(); snap != nil {
		finder = snap
	}

	m.Status.SetText("Sending…")
	m.corsBanner.Hide()

	// Build the cancellable context on the UI thread so the click
	// handler (sendOrAbort) can call cancel() to abort an in-flight
	// Send. The button text swap signals the toggled mode to the user.
	ctx, cancel := context.WithCancel(context.Background())
	m.sendCancel = cancel
	// In abort mode the icon goes away and the button shows "Abort"
	// in warning-importance text — visually distinct from the default
	// icon-only Send state.
	m.Send.SetIcon(nil)
	m.Send.SetText("Abort")
	m.Send.Importance = widget.WarningImportance
	m.Send.Refresh()

	// Capture the pre-script's view of method+URL so we can flag any
	// mutation in the status line later.
	originalMethod, originalURL := req.Method, req.URL

	// Snapshot the overlay BEFORE the chain runs so we can roll back
	// any helena.env.set writes the chain landed if it then errored.
	// Failing chains shouldn't leak partial state into the next Send.
	preOverlay := m.sess.SnapshotEnvOverlay()

	go func() {
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("Send panic: %v", r)
				m.sess.RestoreEnvOverlay(preOverlay)
				fyne.Do(func() {
					m.resetSendButton()
					m.Status.SetText(msg)
				})
			}
		}()

		// Per-step progress feedback: chain.Resolve fires this once
		// before each ExecuteOnce on the worker goroutine; fyne.Do
		// marshals the status update to the UI thread.
		progress := func(step, total int, _, name string) {
			fyne.Do(func() {
				m.Status.SetText(fmt.Sprintf("Chain step %d/%d: %s", step, total, name))
			})
		}
		chainMap, chainConsole, chainErr := chain.Resolve(ctx, req, finder, exec, progress)
		if chainErr != nil {
			m.sess.RestoreEnvOverlay(preOverlay)
			fyne.Do(func() {
				m.resetSendButton()
				if ctx.Err() == context.Canceled {
					m.Status.SetText("Aborted")
				} else {
					m.Status.SetText("Chain error: " + chainErr.Error())
				}
				m.responseRaw.SetText(chainErr.Error())
				m.prettyText.SetText("")
				m.headersText.SetText("")
				m.Response.SelectIndex(1)
				m.setScriptConsole(chainConsole)
			})
			return
		}

		// If the chain fired, swap the status back to a generic
		// "Sending…" so the user knows the leaf is now in flight and
		// not stuck on the last chain step.
		if len(chainMap) > 0 {
			fyne.Do(func() { m.Status.SetText("Sending: " + req.Name) })
		}

		view, leafConsole, leafErr := exec.ExecuteOnce(ctx, req, chainMap)
		consoleLines := append(chainConsole, leafConsole...)

		fyne.Do(func() {
			m.resetSendButton()
			m.setScriptConsole(consoleLines)
			// No HTTP completed → pre-script or HTTP failure; show the error.
			if view.Response.StatusCode == 0 {
				m.Response.SelectIndex(1)
				if ctx.Err() == context.Canceled {
					m.Status.SetText("Aborted")
				} else {
					m.Status.SetText("Error: " + leafErr.Error())
				}
				m.responseRaw.SetText(leafErr.Error())
				m.prettyText.SetText("")
				m.headersText.SetText("")
				return
			}
			// HTTP completed → render response. leafErr (if any) is a
			// post-script error; surfaced as a status-line suffix.
			m.responseRaw.SetText(string(view.Response.Body))
			m.headersText.SetText(responsefmt.FormatHeaders(view.Response.Headers))

			pretty := ""
			ct := view.Response.Headers.Get("Content-Type")
			switch {
			case responsefmt.IsJSON(ct):
				if p, perr := responsefmt.PrettyJSON(view.Response.Body); perr == nil {
					pretty = p
				}
			case responsefmt.IsXML(ct):
				if p, perr := responsefmt.PrettyXML(view.Response.Body); perr == nil {
					pretty = p
				}
			}
			m.prettyText.SetText(pretty)
			if pretty != "" {
				m.Response.SelectIndex(0)
			} else {
				m.Response.SelectIndex(1)
			}

			status := fmt.Sprintf("%s · %s · %s",
				view.Response.Status,
				responsefmt.HumanSize(view.Response.Size),
				responsefmt.HumanDuration(view.Response.Duration))
			if view.Request.Method != string(originalMethod) || view.Request.URL != originalURL {
				status += fmt.Sprintf(" · sent %s %s", view.Request.Method, view.Request.URL)
			}
			if leafErr != nil {
				status += " · " + leafErr.Error()
			}
			m.Status.SetText(status)
			if view.Response.CORSWarning != "" {
				m.corsBanner.Text = "⚠ CORS: " + view.Response.CORSWarning
				m.corsBanner.Refresh()
				m.corsBanner.Show()
			}
		})
	}()
}

// onEnvChanged is the Environment dropdown's selection handler; it stores the
// chosen environment on the session and re-runs the URL preview so its
// resolution reflects the new variables.
func (m *MainUI) onEnvChanged(name string) {
	if name == noEnv {
		m.sess.SetActiveEnv("")
	} else {
		m.sess.SetActiveEnv(name)
	}
	m.updateURLPreview()
}

// refreshEnvironments reseeds the Environment dropdown from the active
// collection's environment list and restores the previously selected name.
// Tolerates being called before the Environment widget is built (e.g.
// during Workspace dropdown's initial selection inside NewMainUI).
func (m *MainUI) refreshEnvironments() {
	if m.Environment == nil {
		return
	}
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
