package ui

import (
	"context"
	"fmt"
	"image/color"
	"net/http"
	"slices"
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

	"github.com/idct/helena/internal/model"
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
	paramRows        []*kvRow // widget handles for each Query row, for in-place updates (#53)
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
	// errorBanner is a persistent, user-dismissible failure indicator above the
	// response tabs (#51). Unlike the transient status line it stays until the
	// next successful response or an explicit dismiss, and is visible whichever
	// response sub-tab is active.
	errorBanner      *fyne.Container
	errorBannerLabel *widget.Label
	// emptyState is the first-run panel shown in the sidebar when no collection
	// is loaded, offering starter actions (#58). refreshEmptyState toggles it.
	emptyState *fyne.Container

	helpBtn    *ttwidget.Button // anchors the Help popup menu (#61)
	appVersion string           // build version for the About entry; set via SetVersion

	currentRequest     *model.Request
	currentRequestID   string
	urlBaseline        urlBaseline // stored vs folded URL/Params, for the open→save no-op (#101)
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

	// httpTransport is the per-session connection pool, reused across sends so
	// repeated requests to one host skip TCP+TLS re-handshakes (#52). Rebuilt
	// only when the TLS-affecting setting changes. Accessed on the UI thread.
	httpTransport         *http.Transport
	httpTransportInsecure bool

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
	// Save-to-file sits at the trailing edge of the response toolbar row, for
	// large or binary bodies that copy can't handle (#66).
	saveRespBtn := tipButton("download", "Save response to file", m.saveResponseToFile)
	respHeader := container.NewBorder(nil, nil, nil, saveRespBtn, pvToolbar)
	respBody := container.NewBorder(respHeader, nil, nil, nil, m.pv)
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

	m.errorBannerLabel = widget.NewLabel("")
	m.errorBannerLabel.Wrapping = fyne.TextWrapWord
	m.errorBannerLabel.Importance = widget.DangerImportance
	errDismiss := widget.NewButtonWithIcon("", themedIcon("circle-xmark"), m.hideErrorBanner)
	errDismiss.Importance = widget.LowImportance
	m.errorBanner = container.NewBorder(nil, nil, nil, errDismiss, m.errorBannerLabel)
	m.errorBanner.Hide()

	m.Status = widget.NewLabel("Ready")

	m.Send.OnTapped = m.sendOrAbort

	wsBtn := widget.NewButton("Workspaces…", m.editWorkspaces)
	// Variables (table-list) and Manage-environments (gears) are icon buttons.
	varsBtn := tipButton("table-list", "Variables", m.editEnvironments)
	envMgrBtn := tipButton("gears", "Manage environments", m.manageEnvironments)
	// Settings (cog) and Help (question mark) are icon buttons (#127/#128).
	settingsBtn := tipButtonRes(theme.SettingsIcon(), "Settings", m.editSettings)
	m.helpBtn = tipButtonRes(theme.HelpIcon(), "Help", m.showHelpMenu)

	// Group indicators replace the "Workspace:" / "Environment:" text labels:
	// cubes for the workspace picker, folder-tree for the environment picker
	// (icon-only with tooltips, centred against the combo boxes).
	indicator := func(icon, tip string) fyne.CanvasObject {
		ic := ttwidget.NewIcon(themedIcon(icon))
		ic.SetToolTip(tip)
		return container.NewCenter(ic)
	}
	// A vertical separator divides the workspace group from the environment
	// group, with extra margin on each side (beyond the HBox spacing) so the two
	// groups read as clearly distinct.
	sepMargin := theme.Padding() * 2
	groupSep := container.New(layout.NewCustomPaddedLayout(0, 0, sepMargin, sepMargin), widget.NewSeparator())
	// Widen the environment dropdown so "No Environment" shows in full — the
	// Select's own min-width plus the 24px toolbar arrow + padding otherwise
	// truncates it. A transparent min-size backing sets the floor; the Select
	// fills the stack.
	envMinW := fyne.MeasureText(noEnv, theme.TextSize(), fyne.TextStyle{}).Width +
		theme.Size(theme.SizeNameInlineIcon) + theme.InnerPadding()*2 + theme.Padding()*4
	envFloor := canvas.NewRectangle(color.Transparent)
	envFloor.SetMinSize(fyne.NewSize(envMinW, 1))
	envBox := container.NewStack(envFloor, m.Environment)
	leading := container.NewHBox(
		indicator("cubes", "Workspace"), m.Workspace, wsBtn,
		groupSep,
		indicator("folder-tree", "Environment"), envBox, varsBtn, envMgrBtn,
	)
	// Settings + Help are pushed to the trailing edge (#129).
	trailing := container.NewHBox(settingsBtn, m.helpBtn)
	// toolbarTheme restores normal theme padding (the root theme zeroes it) so
	// the HBox spaces its controls AND bumps inline icons to 24px, so the top
	// bar's icon buttons + indicators match the sidebar action toolbar; NewPadded
	// gives the bar a margin.
	toolbar := container.NewThemeOverride(
		container.NewPadded(container.NewBorder(nil, nil, leading, trailing, nil)),
		toolbarTheme{},
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
	// First-run empty state: shown over the (empty) tree when no collection is
	// loaded, giving a new user somewhere to start (#58).
	emptyHeading := widget.NewLabelWithStyle("No collections yet", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	emptyHint := widget.NewLabel("Open or import a collection,\nor load the bundled sample to explore.")
	emptyHint.Alignment = fyne.TextAlignCenter
	emptyHint.Wrapping = fyne.TextWrapWord
	m.emptyState = container.NewCenter(container.NewVBox(
		emptyHeading,
		emptyHint,
		widget.NewButtonWithIcon("Open collection…", themedIcon("folder-open"), m.openCollection),
		widget.NewButtonWithIcon("Import…", themedIcon("download"), m.actionImport),
		widget.NewButtonWithIcon("Load sample", themedIcon("cube"), m.loadSample),
	))
	m.emptyState.Hide()
	treeArea := container.NewStack(
		container.NewThemeOverride(m.Tree, sidebarTheme{}),
		dropLayer,
		m.emptyState,
	)
	sidebar := container.NewBorder(container.NewPadded(actionToolbar), nil, nil, nil, treeArea)

	// The CORS banner sits above the response tabs (hidden until a warning
	// fires). Copy is handled in-widget: PrettyView has Ctrl+C / right-click
	// copy, and the Headers entry copies natively.
	responsePanel := container.NewBorder(container.NewVBox(m.errorBanner, m.corsBanner), nil, nil, nil, m.Response)
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
	m.refreshEmptyState()
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
	m.updateWindowTitle()
}

// windowTitleFor composes the window title from the build version and the
// active workspace, e.g. "Helena — Default" (dev) or "Helena 1.2.0 — Default"
// for a released build. The version suffix mirrors cmd/helena's windowTitle.
func (m *MainUI) windowTitleFor() string {
	title := "Helena"
	if m.appVersion != "" && m.appVersion != "dev" {
		title += " " + m.appVersion
	}
	names := m.sess.WorkspaceNames()
	if i := m.sess.ActiveIndex(); i >= 0 && i < len(names) {
		title += " — " + names[i]
	}
	return title
}

// updateWindowTitle refreshes the window title so it reflects the active
// workspace. A no-op until SetWindow has supplied the window.
func (m *MainUI) updateWindowTitle() {
	if m.win != nil {
		m.win.SetTitle(m.windowTitleFor())
	}
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

// loadRequest populates every editor widget from req, with the loading flag set
// so write-back callbacks don't fire during the bulk-update.
// urlBaseline snapshots a loaded request's URL/Params so an unedited save can
// restore the exact stored form instead of the normalized base+Params fold
// (#101). origURL/origParams are the stored values; foldURL/foldParams are the
// post-load fold. If currentRequest still equals the fold at save time the
// URL/Params were untouched, so the original is written back byte-identically.
type urlBaseline struct {
	origURL    string
	origParams []model.KeyValue
	foldURL    string
	foldParams []model.KeyValue
}

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
		m.paramRows = m.paramRows[:0]
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
	//
	// The fold is a *display* convenience: snapshot the stored URL/Params and the
	// post-fold pair so an unedited save can write the byte-identical original
	// back instead of the normalized fold (#101) — see saveRequest.
	m.urlBaseline.origURL = req.URL
	m.urlBaseline.origParams = append([]model.KeyValue(nil), req.Params...)
	if base, urlQuery, frag := splitURLQuery(req.URL); urlQuery != "" {
		req.URL = withFragment(base, frag)
		req.Params = append(parseQueryParams(urlQuery), req.Params...)
	}
	m.urlBaseline.foldURL = req.URL
	m.urlBaseline.foldParams = append([]model.KeyValue(nil), req.Params...)
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
	m.currentRequest.Headers = pruneEmptyKV(m.currentRequest.Headers)
	m.currentRequest.Body.Form = pruneEmptyKV(m.currentRequest.Body.Form)
	cleanedChain, halfFilledChain := pruneEmptyChain(m.currentRequest.Chain)
	m.currentRequest.Chain = cleanedChain

	// If the URL + Params are untouched since load, persist the exact stored form
	// so opening then saving a request is byte-identical (#101); otherwise write
	// the normalized fold. The check is a value comparison (not an edited-flag),
	// so no missed edit path can ever discard the user's changes.
	pristineURL := m.currentRequest.URL == m.urlBaseline.foldURL &&
		slices.Equal(m.currentRequest.Params, m.urlBaseline.foldParams)
	var restoreURL func()
	if pristineURL {
		fURL, fParams := m.currentRequest.URL, m.currentRequest.Params
		m.currentRequest.URL = m.urlBaseline.origURL
		m.currentRequest.Params = m.urlBaseline.origParams
		restoreURL = func() { m.currentRequest.URL, m.currentRequest.Params = fURL, fParams }
	} else {
		m.currentRequest.Params = pruneEmptyKV(m.currentRequest.Params)
	}
	m.rebuildHeadersRows()
	m.rebuildBodyFormRows()
	m.rebuildChainRows()

	err := m.sess.SaveActiveCollection()
	if restoreURL != nil {
		restoreURL() // back to the working fold so the live editor stays consistent
	}
	m.rebuildParamsRows()
	if err != nil {
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
	m.refreshEmptyState()
	m.updateWindowTitle()
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
			m.refreshEmptyState()
			m.Status.SetText("Opened collection: " + u.Name())
		}
	}, m.win)
}
