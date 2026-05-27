package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/httpclient"
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
	Method      *widget.Select
	URL         *widget.Entry
	Send        *widget.Button
	Tree        *widget.Tree
	Request     *container.AppTabs
	Response    *container.AppTabs
	Status      *widget.Label

	responseRaw *widget.Entry

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

	m.Method = widget.NewSelect(methodNames(), nil)
	m.Method.SetSelected(string(model.GET))
	m.URL = widget.NewEntry()
	m.URL.SetPlaceHolder("https://{{base_url}}/path")
	m.Send = widget.NewButton("Send", nil)
	m.Send.Importance = widget.HighImportance

	m.Tree = m.buildTree()

	m.Request = container.NewAppTabs(
		container.NewTabItem("Params", placeholder("Query parameters — editor arrives in Phase 3.")),
		container.NewTabItem("Headers", placeholder("Request headers — editor arrives in Phase 3.")),
		container.NewTabItem("Body", placeholder("Request body & validation — arrives in Phase 3.")),
	)
	m.responseRaw = widget.NewMultiLineEntry()
	m.responseRaw.Wrapping = fyne.TextWrapOff
	m.responseRaw.SetPlaceHolder("Response body appears here after you press Send.")
	m.Response = container.NewAppTabs(
		container.NewTabItem("Pretty", placeholder("Formatted response (pretty JSON/XML) — Phase 4.")),
		container.NewTabItem("Raw", container.NewScroll(m.responseRaw)),
		container.NewTabItem("Headers", placeholder("Response headers — Phase 4.")),
	)
	m.Status = widget.NewLabel("Ready")

	m.Send.OnTapped = m.send

	envBtn := widget.NewButton("Environments…", m.editEnvironments)
	toolbar := container.NewHBox(
		widget.NewLabel("Workspace:"), m.Workspace,
		widget.NewLabel("Env:"), m.Environment, envBtn,
	)
	addressBar := container.NewBorder(nil, nil, m.Method, m.Send, m.URL)
	top := container.NewVBox(toolbar, addressBar)

	openBtn := widget.NewButton("Open…", m.openCollection)
	sidebarHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("Collections", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		openBtn)
	sidebar := container.NewBorder(sidebarHeader, nil, nil, nil, m.Tree)

	editor := container.NewVSplit(m.Request, m.Response)
	editor.SetOffset(0.5)
	body := container.NewHSplit(sidebar, editor)
	body.SetOffset(0.25)

	m.root = container.NewBorder(top, m.Status, nil, nil, body)

	m.refreshEnvironments()
	return m
}

// Root returns the assembled root canvas object.
func (m *MainUI) Root() fyne.CanvasObject { return m.root }

// SetWindow records the parent window used for dialogs.
func (m *MainUI) SetWindow(w fyne.Window) { m.win = w }

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
		if ci := m.sess.Tree().CollectionIndex(id); ci >= 0 {
			m.sess.SetActiveCollection(ci)
			m.refreshEnvironments()
		}
		r, ok := m.sess.Tree().Request(id)
		if !ok {
			return
		}
		if r.Method != "" {
			m.Method.SetSelected(string(r.Method))
		}
		m.URL.SetText(r.URL)
		m.Status.SetText("Loaded: " + r.Name)
	}
	return t
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

// send executes the request currently in the method/URL fields off the UI
// goroutine, resolving {{vars}} against the active environment, and shows the
// result in the Raw response tab.
func (m *MainUI) send() {
	if strings.TrimSpace(m.URL.Text) == "" {
		m.Status.SetText("Enter a URL first")
		return
	}
	req := model.Request{Method: model.Method(m.Method.Selected), URL: m.URL.Text}
	client := httpclient.New(model.DefaultSettings())
	resolver := m.sess.Resolver()

	m.Status.SetText("Sending…")
	m.Send.Disable()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		resp, err := client.Do(ctx, req, resolver)

		fyne.Do(func() {
			m.Send.Enable()
			m.Response.SelectIndex(1) // Raw
			if err != nil {
				m.Status.SetText("Error: " + err.Error())
				m.responseRaw.SetText(err.Error())
				return
			}
			m.Status.SetText(fmt.Sprintf("%s · %d bytes · %s",
				resp.Status, resp.Size, resp.Duration.Round(time.Millisecond)))
			m.responseRaw.SetText(string(resp.Body))
		})
	}()
}

func (m *MainUI) onEnvChanged(name string) {
	if name == noEnv {
		m.sess.SetActiveEnv("")
		return
	}
	m.sess.SetActiveEnv(name)
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
// collection's active environment (creating one if needed) and saves changes
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
		m.Status.SetText("Saved environment: " + env.Name)
	}, m.win)
	d.Resize(fyne.NewSize(540, 400))
	d.Show()
}

func methodNames() []string {
	out := make([]string, len(model.Methods))
	for i, mth := range model.Methods {
		out[i] = string(mth)
	}
	return out
}

func placeholder(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return container.NewPadded(l)
}
