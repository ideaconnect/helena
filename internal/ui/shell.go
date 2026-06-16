package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
	prettyview "github.com/ideaconnect/go-fyne-pretty-view/v2"

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

	// The chain fallback lets this request's URL / params / headers / body /
	// auth use {{chain.<alias>.response.json.token}}-style templates, scoped to
	// this request's own chain aliases (same map the pre-script saw).
	resolver := vars.New(e.envSnap, e.sess.SnapshotEnvOverlay()).WithFallback(chain.VarLookup(chainMap))
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
	Save        *ttwidget.Button
	Send        *widget.Button
	Tree        *widget.Tree
	// Sidebar toolbar: node-action icon buttons operating on the selected tree
	// node. rename / delete enable with any selection; clone with a folder or
	// request; add request / folder fall back to the active collection.
	sbAddReq    *ttwidget.Button
	sbAddFolder *ttwidget.Button
	sbRename    *ttwidget.Button
	sbClone     *ttwidget.Button
	sbDelete    *ttwidget.Button
	// Drag-and-drop reordering of the collections tree (see treedrag.go).
	treeRows      map[*treeRow]string // live row → bound node id, for drop hit-testing
	dragActive    bool
	dragSrcID     string
	dragLastAbs   fyne.Position
	dropIndicator *canvas.Rectangle // insert-between line
	dropInto      *canvas.Rectangle // into-container outline
	Request       *container.AppTabs
	Response      *container.AppTabs
	Status        *widget.Label

	paramsRows       *fyne.Container
	headersRows      *fyne.Container
	BodyType         *widget.Select
	BodyContent      *prettyview.PrettyView // editable raw body (json/xml/text); hidden for form types
	bodyFormRows     *fyne.Container        // KV rows bound to Body.Form (form-urlencoded / multipart)
	bodyFormPanel    *fyne.Container        // wrapper shown in place of BodyContent for form types
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

	pv          *prettyview.PrettyView // response body viewer (structured + raw + search)
	headersText *widget.Entry
	corsBanner  *canvas.Text

	currentRequest     *model.Request
	currentRequestID   string
	lastSelectedNodeID string
	loading            bool // suppress write-back during programmatic widget updates
	syncing            bool // suppress re-entrant URL<->Query sync (see query.go)

	// Editor tab strip. tabs is the ordered set of open requests;
	// activeTabIdx indexes the active one (-1 when none). tabBar holds the
	// requestTab widgets + the trailing newTabBtn; rebuilt by rebuildTabBar.
	// tabWidgets pools one requestTab per open tab so a drag gesture survives
	// the live reorder. tabOverflowBtn opens the "list all tabs" menu.
	// See tabs.go / tabstrip.go.
	tabs           []*openTab
	activeTabIdx   int
	tabBar         *fyne.Container
	tabWidgets     map[*openTab]*requestTab
	newTabBtn      *widget.Button
	tabOverflowBtn *widget.Button
	tabStripBg     *canvas.Rectangle // header-coloured band behind the tab strip

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
			m.refreshActiveTabLabel()
		}
	})
	m.Method.SetSelected(string(model.GET))

	m.URL = widget.NewEntry()
	m.URL.SetPlaceHolder("https://{{base_url}}/path")
	m.URL.OnChanged = func(s string) {
		if !m.loading && !m.syncing && m.currentRequest != nil {
			m.applyURLEdit(s)
		}
		m.updateURLPreview()
	}
	m.URL.OnSubmitted = func(_ string) { m.send() }

	m.urlPreview = widget.NewLabel("")
	m.urlPreview.TextStyle = fyne.TextStyle{Italic: true}
	m.urlPreview.Hide()

	m.Save = tipButton("floppy-disk", "Save", m.saveRequest)
	m.Save.Disable()
	// Send shows just the send-arrow icon by default;
	// abort mode swaps to text "Abort" with warning importance.
	m.Send = widget.NewButtonWithIcon("", themedIcon("location-arrow"), nil)
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
		if m.loading || m.currentRequest == nil {
			return
		}
		oldType := m.currentRequest.Body.Type
		newType := model.BodyType(s)
		m.currentRequest.Body.Type = newType
		// Re-highlight the editor for the new body type (JSON/XML get syntax
		// colors; everything else is raw). Reparse keeps the buffer bytes.
		if m.BodyContent != nil {
			m.BodyContent.Reparse(formatForBodyType(newType))
		}
		// Form-urlencoded / multipart edit Body.Form via a KV table; everything
		// else edits raw Content via the text editor. Swap which is shown.
		m.refreshBodyEditorVisibility(newType)
		if h, changed := applyImpliedContentType(m.currentRequest.Headers, oldType, newType); changed {
			m.currentRequest.Headers = h
			m.rebuildHeadersRows()
		}
	})
	m.BodyType.SetSelected(string(model.BodyNone))
	// Request body: the same go-fyne-pretty-view widget as the response viewer,
	// constructed editable (WithEditable) so the user types/pastes with live
	// syntax highlighting and a caret, with on-demand Reformat. It is virtualized
	// (scrolls itself — no enclosing VScroll). OnChanged is debounced, so the
	// authoritative bytes are pulled via syncBodyFromEditor at Save/Send/Validate/Format.
	m.BodyContent = prettyview.New(
		prettyview.WithEditable(),
		prettyview.WithLineNumbers(),
	)
	m.BodyContent.SetTheme(variantFor(sess.Settings().Theme), prettyview.Theme{})
	m.BodyContent.SetOnChanged(func(s string) {
		if !m.loading && m.currentRequest != nil {
			m.currentRequest.Body.Content = s
		}
	})
	validateBtn := widget.NewButton("Validate", m.validateBody)
	formatBtn := widget.NewButton("Format", m.formatBody)
	bodyTopRow := container.NewHBox(widget.NewLabel("Type:"), m.BodyType, validateBtn, formatBtn)
	// Structured Body.Form editor for form-urlencoded / multipart: a KV table
	// (reusing buildKVRow) shown in place of the raw text editor.
	m.bodyFormRows = container.NewVBox()
	addBodyFieldBtn := widget.NewButton("+ Add field", m.addBodyFormField)
	m.bodyFormPanel = container.NewBorder(nil, addBodyFieldBtn, nil, nil,
		container.NewVScroll(m.bodyFormRows))
	m.bodyFormPanel.Hide() // BodyNone default shows the text editor
	bodyStack := container.NewStack(m.BodyContent, m.bodyFormPanel)
	bodyTab := container.NewBorder(bodyTopRow, nil, nil, nil, bodyStack)

	m.Request = container.NewAppTabs(
		container.NewTabItem("Body", bodyTab),
		container.NewTabItem("Auth", m.buildAuthTab()),
		container.NewTabItem("Headers", headersTab),
		container.NewTabItem("Query", paramsTab),
		container.NewTabItem("Scripts", m.buildScriptsTab()),
		container.NewTabItem("Chain", m.buildChainTab()),
		container.NewTabItem("Docs", m.buildDocsTab()),
	)

	// Response body: one go-fyne-pretty-view widget renders JSON/XML/HTML/raw
	// with auto-detection, per-node fold, syntax highlighting, search and
	// soft-wrap, all viewport-virtualized so huge bodies stay cheap. It
	// subsumes the old Structured tree + Raw text viewer. Its built-in toolbar
	// (format selector, expand/collapse, wrap toggle, find box) sits above it;
	// Open is disabled — this is a response viewer, not a file editor.
	m.pv = prettyview.New()
	m.pv.SetTheme(variantFor(sess.Settings().Theme), prettyview.Theme{})
	pvToolbar := prettyview.NewToolbar(m.pv, prettyview.ToolbarConfig{
		ShowFormat: true, ShowExpandCollapse: true, ShowWrap: true, ShowSearch: true,
	})
	respBody := container.NewBorder(pvToolbar, nil, nil, nil, m.pv)
	m.headersText = widget.NewMultiLineEntry()
	m.headersText.Wrapping = fyne.TextWrapOff
	m.headersText.SetPlaceHolder("Response headers appear here after you press Send.")
	m.Response = container.NewAppTabs(
		container.NewTabItem("Body", respBody),
		container.NewTabItem("Headers", container.NewScroll(m.headersText)),
	)
	m.corsBanner = canvas.NewText("", theme.Color(theme.ColorNameWarning))
	m.corsBanner.TextStyle.Bold = true
	m.corsBanner.Hide()
	m.Status = widget.NewLabel("Ready")

	m.Send.OnTapped = m.sendOrAbort

	wsBtn := widget.NewButton("Workspaces…", m.editWorkspaces)
	envBtn := widget.NewButton("Variables…", m.editEnvironments)
	envMgrBtn := widget.NewButton("Manage…", m.manageEnvironments)
	settingsBtn := widget.NewButton("Settings…", m.editSettings)
	helpBtn := widget.NewButton("?", m.showShortcuts)
	toolbar := container.NewHBox(
		widget.NewLabel("Workspace:"), m.Workspace, wsBtn,
		widget.NewLabel("Environment:"), m.Environment, envBtn, envMgrBtn,
		settingsBtn, helpBtn,
	)
	exportBtn := tipButton("file-export", "Export…", m.actionExport)
	saveSendBox := container.NewHBox(m.Save, exportBtn, m.Send)
	addressBar := container.NewBorder(nil, nil, m.Method, saveSendBox, m.URL)

	// Editor tab strip above the address bar: one tab per open request (drag to
	// reorder) plus a trailing "+" that opens a blank scratch request, all
	// horizontally scrollable so many open tabs don't squeeze the bar. An
	// always-visible overflow "⋮" on the right lists every tab for quick jumps
	// when the strip overflows. rebuildTabBar renders it.
	m.activeTabIdx = -1
	m.tabBar = container.NewHBox()
	m.newTabBtn = widget.NewButtonWithIcon("", themedIcon("plus"), m.newScratchTab)
	m.newTabBtn.Importance = widget.LowImportance
	m.tabOverflowBtn = widget.NewButtonWithIcon("", theme.MoreVerticalIcon(), m.showTabMenu)
	m.tabOverflowBtn.Importance = widget.LowImportance
	m.rebuildTabBar() // hides the overflow button while no tabs are open
	// The strip sits on a header-coloured band with a separator line along its
	// bottom. The tabs stack on top: the active tab's content-coloured
	// background covers the line beneath it (so it connects to the editor),
	// while inactive tabs are transparent and let the band + line show — the
	// "tab merges into the content" look from Bruno.
	m.tabStripBg = canvas.NewRectangle(theme.Color(theme.ColorNameHeaderBackground))
	// A little leading space so the first tab isn't flush against the strip's
	// left edge (the tabs themselves are spaced by the HBox padding between them).
	tabScroll := container.New(layout.NewCustomPaddedLayout(0, 0, theme.Padding(), 0), container.NewHScroll(m.tabBar))
	stripContent := container.NewBorder(nil, nil, nil, m.tabOverflowBtn, tabScroll)
	tabRow := container.NewStack(
		m.tabStripBg,
		container.NewBorder(nil, widget.NewSeparator(), nil, nil, nil),
		// Top padding so the tabs don't touch the upper boundary — the band
		// still fills to the top, the tabs sit a few px below it.
		container.New(layout.NewCustomPaddedLayout(theme.Padding(), 0, 0, 0), stripContent),
	)

	// The tab strip + address bar + URL preview live in the editor column (not
	// the global top bar) so the sidebar runs floor-to-ceiling on the left and
	// the URL bar starts exactly where the request editor starts.
	editorTop := container.NewVBox(tabRow, addressBar, m.urlPreview)

	// Compact Font Awesome icon buttons with hover tooltips (fyne-tooltip), since
	// they are icon-only. Collection-level ops:
	newColBtn := tipButton("square-plus", "New collection", m.actionNewCollection)
	openBtn := tipButton("folder-open", "Open collection", m.openCollection)
	importBtn := tipButton("download", "Import", m.actionImport)

	// Node-action buttons operating on the selected tree node. Enable state is
	// reconciled by refreshSidebarActions (rename/delete need any selection;
	// clone needs a folder or request; add request/folder always on).
	m.sbAddReq = tipButton("file-circle-plus", "New request", m.actionNewRequest)
	m.sbAddFolder = tipButton("folder-plus", "New folder", m.actionNewFolder)
	m.sbRename = tipButton("pen-to-square", "Rename", m.actionRename)
	m.sbClone = tipButton("copy", "Duplicate", m.actionDuplicate)
	m.sbDelete = tipButton("trash-can", "Delete", m.actionDelete)
	m.refreshSidebarActions()

	// Toolbar line. Left-aligned: delete, a gap, then a cube group-indicator and
	// the collection ops (new / open / import). Right-aligned: a file
	// group-indicator and the request/folder ops (new request, clone, new
	// folder, rename). Wrapped in toolbarTheme so the icons are a chunky 24px.
	// The cube/file group indicators are bare icons (with their own tooltips);
	// pad them by the same InnerPadding a button insets its icon by, so they
	// share the buttons' footprint and line up with them (no button background).
	pad := theme.InnerPadding()
	cubeIcon := ttwidget.NewIcon(themedIcon("cube"))
	cubeIcon.SetToolTip("Collections")
	fileIcon := ttwidget.NewIcon(themedIcon("file"))
	fileIcon.SetToolTip("Requests")
	cubeIndicator := container.New(layout.NewCustomPaddedLayout(pad, pad, pad, pad), cubeIcon)
	fileIndicator := container.New(layout.NewCustomPaddedLayout(pad, pad, pad, pad), fileIcon)
	gap := canvas.NewRectangle(color.Transparent)
	gap.SetMinSize(fyne.NewSize(theme.Padding()*3, 1))
	leftGroup := container.NewHBox(m.sbDelete, gap, cubeIndicator, newColBtn, openBtn, importBtn)
	rightGroup := container.NewHBox(fileIndicator, m.sbAddReq, m.sbClone, m.sbAddFolder, m.sbRename)
	actionToolbar := container.NewThemeOverride(
		container.NewBorder(nil, nil, leftGroup, rightGroup),
		toolbarTheme{})

	// Drop-indicator overlays: a thin line for insert-between and an outline for
	// drop-into-container. Positioned imperatively during a drag (see
	// treedrag.go), hidden otherwise.
	m.dropIndicator = canvas.NewRectangle(theme.Color(theme.ColorNamePrimary))
	m.dropIndicator.Hide()
	m.dropInto = canvas.NewRectangle(color.Transparent)
	m.dropInto.StrokeColor = theme.Color(theme.ColorNamePrimary)
	m.dropInto.StrokeWidth = 2
	m.dropInto.CornerRadius = theme.Size(theme.SizeNameSelectionRadius)
	m.dropInto.Hide()
	dropLayer := container.NewWithoutLayout(m.dropIndicator, m.dropInto)
	// The header is padded for breathing room. The tree is wrapped in a
	// ThemeOverride (sidebarTheme) that shrinks the inline-icon + padding sizes
	// so its per-level indentation is shallower and its icons denser — scoped
	// to the sidebar so the rest of the app keeps full-size icons. The drop
	// layer stacks on top to draw the drag indicators.
	treeArea := container.NewStack(
		container.NewThemeOverride(m.Tree, sidebarTheme{}),
		dropLayer,
	)
	sidebar := container.NewBorder(container.NewPadded(actionToolbar), nil, nil, nil, treeArea)

	// The CORS banner sits above the response tabs (hidden until a warning
	// fires). Copy is handled in-widget: PrettyView has Ctrl+C / right-click
	// copy, and the Headers entry copies natively.
	responsePanel := container.NewBorder(m.corsBanner, nil, nil, nil, m.Response)
	// Request / response split with a thin-line divider.
	editor := thinVSplit(m.Request, responsePanel, 0.5)
	// Editor column carries its own address bar so the sidebar runs
	// full-height to the left of it (9.1).
	editorColumn := container.NewBorder(editorTop, nil, nil, nil, editor)
	// Sidebar / editor split, also a thin-line divider.
	body := thinHSplit(sidebar, editorColumn, 0.25)

	// Thin separator lines under the top bar and above the status bar, so the
	// whole window grid is divided by hairlines (VS Code / Bruno style). The root
	// is wrapped in rootTheme (padding 0) so the body sits flush against those
	// lines and the vertical split divider meets them with no gap; the status
	// label restores its own padding.
	header := container.NewVBox(toolbar, widget.NewSeparator())
	footer := container.NewVBox(widget.NewSeparator(), container.NewThemeOverride(m.Status, paneTheme{}))
	m.root = container.NewThemeOverride(
		container.NewBorder(header, footer, nil, nil, body),
		rootTheme{})

	m.refreshEnvironments()
	m.restoreTabs()
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

// loadErrorReport formats the active workspace's collection load failures into
// a user-facing diagnostic, or "" when every collection loaded. A failed
// collection is dropped from the sidebar; this is how the user learns why
// rather than seeing it silently disappear (#108).
func (m *MainUI) loadErrorReport() string {
	errs := m.sess.LoadErrors()
	if len(errs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("These collections could not be loaded and were left out of the sidebar:\n")
	for _, e := range errs {
		fmt.Fprintf(&b, "\n• %s\n    %v", e.Dir, e.Err)
	}
	return b.String()
}

// SurfaceLoadErrors shows a non-transient dialog when one or more collections
// failed to load on the last reload. Safe to call with no window or no errors
// (a no-op in both cases).
func (m *MainUI) SurfaceLoadErrors() {
	report := m.loadErrorReport()
	if report == "" || m.win == nil {
		return
	}
	dialog.ShowInformation("Some collections could not be loaded", report, m.win)
}

func (m *MainUI) buildTree() *widget.Tree {
	t := widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID { return m.sess.Tree().ChildIDs(id) },
		func(id widget.TreeNodeID) bool { return m.sess.Tree().IsBranch(id) },
		func(bool) fyne.CanvasObject {
			return newTreeRow(m.dragTreeNode, m.dropTreeNode, func() bool { return m.dragActive })
		},
		func(id widget.TreeNodeID, _ bool, o fyne.CanvasObject) {
			row := o.(*treeRow)
			if r, ok := m.sess.Tree().Request(id); ok {
				row.setRequest(id, string(r.Method), r.Name)
			} else {
				row.setBranch(id, m.sess.Tree().Label(id))
			}
			if m.treeRows == nil {
				m.treeRows = map[*treeRow]string{}
			}
			m.treeRows[row] = id // registry for drag-drop hit-testing
		},
	)
	t.OnSelected = func(id widget.TreeNodeID) {
		m.lastSelectedNodeID = id
		m.refreshSidebarActions() // enable/disable the toolbar's node actions
		if _, ok := m.sess.Tree().Request(id); ok {
			// Request node — open or focus its tab. activateTab makes the
			// owning collection active and refreshes environments, so the
			// collection-activation below is only for folder/collection rows.
			m.openOrActivate(id)
			return
		}
		if ci := m.sess.Tree().CollectionIndex(id); ci >= 0 {
			m.sess.SetActiveCollection(ci)
			m.refreshEnvironments()
		}
	}
	return t
}

// refreshSidebarActions reconciles the node-action toolbar's enable state with
// the current selection: rename / clone / delete need a selected node (clone
// only a request), while add request / folder stay enabled (they fall back to
// the active collection). Called from the tree's OnSelected and after deletions
// that clear the selection.
func (m *MainUI) refreshSidebarActions() {
	sel := m.lastSelectedNodeID
	enableButton(m.sbRename, sel != "")
	enableButton(m.sbDelete, sel != "")
	// Clone duplicates a request or a folder (a node id contains "/"); whole
	// collections aren't duplicable, so a collection selection leaves it off.
	enableButton(m.sbClone, strings.Contains(sel, "/"))
}

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
		m.rebuildBodyFormRows()
		m.refreshBodyEditorVisibility(model.BodyNone)
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
	// Two-way Query sync: fold any query already in the stored URL into Params so
	// it shows in the table, keep currentRequest.URL as the bare base, and render
	// base + the params query in the field. (SetText here is under m.loading, so
	// its OnChanged won't re-run applyURLEdit.)
	if base, urlQuery, frag := splitURLQuery(req.URL); urlQuery != "" {
		req.URL = withFragment(base, frag)
		req.Params = append(parseQueryParams(urlQuery), req.Params...)
	}
	m.URL.SetText(displayURL(req.URL, req.Params))
	m.rebuildParamsRows()
	m.rebuildHeadersRows()

	bt := req.Body.Type
	if bt == "" {
		bt = model.BodyNone
	}
	m.BodyType.SetSelected(string(bt))
	m.BodyContent.SetData([]byte(req.Body.Content), formatForBodyType(bt))
	m.rebuildBodyFormRows()
	m.refreshBodyEditorVisibility(bt)
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
	m.syncBodyFromEditor()
	// A scratch tab isn't in any collection yet — saving means choosing a
	// destination, which commitScratchTab then converts into a tree-backed tab.
	if t := m.activeTab(); t != nil && t.scratch {
		m.saveScratchTabAs(t)
		return
	}
	// Drop incomplete (empty-key) rows on save so we don't write noise to YAML.
	m.currentRequest.Params = pruneEmptyKV(m.currentRequest.Params)
	m.currentRequest.Headers = pruneEmptyKV(m.currentRequest.Headers)
	m.currentRequest.Body.Form = pruneEmptyKV(m.currentRequest.Body.Form)
	cleanedChain, halfFilledChain := pruneEmptyChain(m.currentRequest.Chain)
	m.currentRequest.Chain = cleanedChain
	m.rebuildParamsRows()
	m.rebuildHeadersRows()
	m.rebuildBodyFormRows()
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
	// {{chain.<alias>...}} vars only resolve at Send time (from chained-request
	// results), so don't flag them as unresolved in the live preview.
	missing = slices.DeleteFunc(missing, func(n string) bool {
		return strings.HasPrefix(n, "chain.")
	})
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

// sanitizeTimeoutSeconds parses a user-entered request-timeout field. A blank,
// non-numeric, or non-positive entry is rejected (valid=false) and the supplied
// fallback is returned instead of silently becoming 0 — which means an
// UNLIMITED timeout and is the same unsafe zero-value the config defaults guard
// against. Helena exposes no explicit "unlimited" control, so a parse failure
// must never produce one. The fallback (the user's current timeout) is itself
// floored to the default when non-positive, so the result is always > 0.
func sanitizeTimeoutSeconds(text string, fallback int) (seconds int, valid bool) {
	if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n > 0 {
		return n, true
	}
	if fallback > 0 {
		return fallback, false
	}
	return model.DefaultSettings().TimeoutSeconds, false
}

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
			m.bodyFormRows.Add(m.buildKVRow(&m.currentRequest.Body.Form, i, m.rebuildBodyFormRows, nil))
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
	if bt == model.BodyForm || bt == model.BodyMultipart {
		m.BodyContent.Hide()
		m.bodyFormPanel.Show()
	} else {
		m.bodyFormPanel.Hide()
		m.BodyContent.Show()
	}
}

// rebuildParamsRows discards the current Params editor rows and re-creates one
// per entry; needed after add/delete and after loadRequest swaps the backing
// slice.
func (m *MainUI) rebuildParamsRows() {
	m.paramsRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Params {
			m.paramsRows.Add(m.buildKVRow(&m.currentRequest.Params, i, m.rebuildParamsRows, m.syncURLFieldFromParams))
		}
	}
	m.paramsRows.Refresh()
}

// rebuildHeadersRows is the Headers-tab counterpart to rebuildParamsRows.
func (m *MainUI) rebuildHeadersRows() {
	m.headersRows.RemoveAll()
	if m.currentRequest != nil {
		for i := range m.currentRequest.Headers {
			m.headersRows.Add(m.buildKVRow(&m.currentRequest.Headers, i, m.rebuildHeadersRows, nil))
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
	m.rebuildParamsRows()
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

// buildKVRow renders one editable row of a KeyValue list. The row's widgets
// write back into list by index; the delete button removes that index and calls
// refresh to rebuild the row container.
// onChange (may be nil) fires after any edit that alters the list's contents —
// the Query tab passes syncURLFieldFromParams so edits flow back into the URL.
func (m *MainUI) buildKVRow(list *[]model.KeyValue, idx int, refresh func(), onChange func()) fyne.CanvasObject {
	fire := func() {
		if onChange != nil {
			onChange()
		}
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
	keyEntry := widget.NewEntry()
	keyEntry.SetText(kv.Key)
	keyEntry.OnChanged = func(s string) {
		if idx < len(*list) {
			(*list)[idx].Key = s
		}
		fire()
	}
	valEntry := widget.NewEntry()
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
	return container.NewBorder(nil, nil, check, delBtn,
		container.NewGridWithColumns(2, keyEntry, valEntry))
}

// onWorkspaceChanged is the Workspace dropdown's selection handler; it tells
// the session which workspace is now active and refreshes the tree so the
// sidebar shows that workspace's collections.
func (m *MainUI) onWorkspaceChanged(name string) {
	prev := m.sess.ActiveIndex()
	for i, n := range m.sess.WorkspaceNames() {
		if n == name {
			m.sess.SetActive(i)
			break
		}
	}
	if m.sess.ActiveIndex() == prev {
		// No actual switch — including the initial SetSelected during
		// construction, when the editor widgets don't exist yet. Bail before
		// touching them.
		return
	}
	// A workspace switch reloads the collections, invalidating every open
	// tab's cached node ID + live pointer (and the active collection). Close
	// all tabs and clear the editor so nothing stale survives; reset selection
	// state + refresh dependent widgets so the Variables dialog reads the new
	// workspace's session, not a stale closure.
	m.closeAllTabs()
	m.lastSelectedNodeID = ""
	if m.Tree != nil {
		m.Tree.UnselectAll()
		m.Tree.Refresh()
	}
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
	m.Send.SetIcon(themedIcon("location-arrow"))
	m.Send.SetText("")
	m.Send.Importance = widget.HighImportance
	m.Send.Refresh()
}

// setAbortButton switches the Send button into its in-flight "abort" look: a
// stop icon in warning importance. It stays icon-only (no "Abort" text) so the
// button keeps the same size as the default Send state — otherwise the wider
// text button reflows the address bar, which flickers when a quick Send toggles
// the button on and straight back off.
func (m *MainUI) setAbortButton() {
	m.Send.SetIcon(themedIcon("circle-stop"))
	m.Send.SetText("")
	m.Send.Importance = widget.WarningImportance
	m.Send.Refresh()
}

// snapshotRequest returns a copy of req with every slice-backed field detached
// from the original's backing arrays, so the off-UI send/chain worker never
// shares mutable state with the live m.currentRequest the user keeps editing.
// (ChainStep / KeyValue are all-value structs, so a shallow element copy fully
// detaches them.)
func snapshotRequest(req model.Request) model.Request {
	req.Params = append([]model.KeyValue(nil), req.Params...)
	req.Headers = append([]model.KeyValue(nil), req.Headers...)
	req.Body.Form = append([]model.KeyValue(nil), req.Body.Form...)
	req.Chain = append([]model.ChainStep(nil), req.Chain...)
	return req
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
		// The body editor's OnChanged is debounced; pull its live bytes now so a
		// Send fired before the settle still carries the latest body.
		m.syncBodyFromEditor()
		// Snapshot the live request on the UI goroutine, detaching its
		// slice-backed fields (Params/Headers/Body.Form/Chain) from
		// m.currentRequest's backing arrays — the user can keep editing those
		// tabs while the send/chain runs for ~ScriptTimeout on the worker, and
		// a shared backing array would be an unsynchronized read/write (race).
		req = snapshotRequest(*m.currentRequest)
		// Flatten any Inherit on the in-memory request copy via the session's
		// ancestor walk so httpclient sees the concrete auth.
		req.Auth = m.sess.EffectiveAuth(m.currentRequestID)
	} else {
		req = model.Request{Method: model.Method(m.Method.Selected()), URL: m.URL.Text}
	}
	// Snapshot env vars + auth state on the UI goroutine so the worker
	// goroutine can run for ~ScriptTimeout without racing against UI
	// mutations of s.cols / s.activeCol / Environment.Variables. The leaf's
	// slice fields are detached above; chain step targets come from the
	// SnapshotChainFinder copy below, so nothing the worker touches is shared.
	envSnap := m.sess.SnapshotActiveEnvVars()

	client := httpclient.New(m.sess.Settings())
	// If auth puts a key in a custom header, strip it on a host-changing
	// redirect so the credential doesn't leak to the new host (Go already
	// handles Authorization/Cookie).
	if k := req.Auth.APIKey; req.Auth.Type == model.AuthAPIKey && k != nil && k.Name != "" &&
		(k.Placement == model.APIKeyHeader || k.Placement == "") {
		client.SetCrossHostStripHeaders([]string{k.Name})
	}
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
	// Abort mode: a warning-importance stop icon, icon-only so the button
	// stays the same size as the default Send state (see setAbortButton).
	m.setAbortButton()

	// Capture the pre-script's view of method+URL so we can flag any
	// mutation in the status line later.
	originalMethod, originalURL := req.Method, req.URL

	// Snapshot the overlay BEFORE the chain runs so we can roll back
	// any helena.env.set writes the chain landed if it then errored.
	// Failing chains shouldn't leak partial state into the next Send.
	preOverlay := m.sess.SnapshotEnvOverlay()

	// Capture the tab that initiated this Send so the async completion
	// attaches the response to it and only repaints the shared panel if it is
	// still active (the user may switch tabs mid-Send). nil when no tab is
	// open (a bare-URL Send).
	initTab := m.activeTab()

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
			status := "Chain error: " + chainErr.Error()
			if ctx.Err() == context.Canceled {
				status = "Aborted"
			}
			resp := &tabResponse{isError: true, errText: chainErr.Error(), status: status, console: chainConsole}
			fyne.Do(func() {
				m.resetSendButton()
				m.deliverResponse(initTab, resp)
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

		// Build the tab response off the UI goroutine (pure formatting, no
		// widget access); the fyne.Do block only resets the button + delivers.
		var resp *tabResponse
		if view.Response.StatusCode == 0 {
			// No HTTP completed → pre-script or HTTP failure.
			errText := ""
			if leafErr != nil {
				errText = leafErr.Error()
			}
			status := "Error: " + errText
			if ctx.Err() == context.Canceled {
				status = "Aborted"
			}
			resp = &tabResponse{isError: true, errText: errText, status: status, console: consoleLines}
		} else {
			// HTTP completed → render response. leafErr (if any) is a
			// post-script error; surfaced as a status-line suffix.
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
			cors := ""
			if view.Response.CORSWarning != "" {
				cors = "⚠ CORS: " + view.Response.CORSWarning
			}
			resp = &tabResponse{
				rawBody:     string(view.Response.Body),
				headersText: responsefmt.FormatHeaders(view.Response.Headers),
				status:      status,
				cors:        cors,
				console:     consoleLines,
			}
		}

		fyne.Do(func() {
			m.resetSendButton()
			m.deliverResponse(initTab, resp)
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
			_ = m.sess.AddEnvironment("Default")
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
		t, validTimeout := sanitizeTimeoutSeconds(timeoutEntry.Text, m.sess.Settings().TimeoutSeconds)
		newTheme := themeFromName(themeSelect.Selected)
		m.sess.SetSettings(model.Settings{
			InsecureSkipVerify: insecure.Checked,
			CORSWarning:        corsWarn.Checked,
			FollowRedirects:    follow.Checked,
			TimeoutSeconds:     t,
			Theme:              newTheme,
		})
		ApplyTheme(fyne.CurrentApp(), newTheme)
		// PrettyView memoizes its palette on the Fyne theme variant, which
		// ApplyTheme's SetTheme(Light/Dark) does not flip — so force a rebuild
		// at the new variant, or the body keeps the old light/dark colors.
		m.pv.SetTheme(variantFor(newTheme), prettyview.Theme{})
		m.BodyContent.SetTheme(variantFor(newTheme), prettyview.Theme{})
		m.refreshThemedCanvas()
		if validTimeout {
			m.Status.SetText("Settings saved")
		} else {
			m.Status.SetText(fmt.Sprintf("Settings saved (invalid timeout ignored, kept %ds)", t))
		}
	}, m.win)
	dlg.Resize(fyne.NewSize(520, 320))
	dlg.Show()
}

// refreshThemedCanvas re-tints the raw canvas objects whose colours are set
// imperatively rather than read from the theme on every paint. Fyne's
// SetTheme repaints theme-driven widgets but not bare canvas.Text /
// canvas.Rectangle, so without this a Light↔Dark switch leaves the CORS
// banner (and the active-tab highlight) showing the old variant's colour.
// Method chips use fixed brand colours (see method.go) so they need no
// re-tint. Called from editSettings after the theme swap.
func (m *MainUI) refreshThemedCanvas() {
	if m.corsBanner != nil {
		m.corsBanner.Color = theme.Color(theme.ColorNameWarning)
		m.corsBanner.Refresh()
	}
	if m.tabStripBg != nil {
		m.tabStripBg.FillColor = theme.Color(theme.ColorNameHeaderBackground)
		m.tabStripBg.Refresh()
	}
	m.rebuildTabBar() // repaints each tab's selection-coloured highlight
	if m.Tree != nil {
		m.Tree.Refresh()
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
