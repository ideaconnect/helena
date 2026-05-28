package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	fynestorage "fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/importer"
	"github.com/idct/helena/internal/model"
	appstorage "github.com/idct/helena/internal/storage"
)

// actionImport opens a chooser dialog letting the user import from either a
// URL or a local file. The URL fetch obeys the user's TLS / timeout settings;
// the file path uses a standard open dialog. In both cases the parsed
// collection then flows through chooseImportDestination.
func (m *MainUI) actionImport() {
	if m.win == nil {
		return
	}

	urlEntry := widget.NewEntry()
	urlEntry.SetPlaceHolder("https://example.com/openapi.yaml")

	var d dialog.Dialog
	fetch := func() {
		url := strings.TrimSpace(urlEntry.Text)
		if url == "" {
			return
		}
		d.Hide()
		m.importFromURL(url)
	}
	urlEntry.OnSubmitted = func(_ string) { fetch() }
	fetchBtn := widget.NewButton("Fetch URL", fetch)
	fetchBtn.Importance = widget.HighImportance

	pickBtn := widget.NewButton("Pick a local file…", func() {
		d.Hide()
		m.importFromFile()
	})

	content := container.NewVBox(
		widget.NewLabelWithStyle("Import an OpenAPI / Swagger / WSDL spec",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("From URL:"),
		urlEntry,
		fetchBtn,
		widget.NewSeparator(),
		widget.NewLabel("Or:"),
		pickBtn,
	)
	d = dialog.NewCustom("Import", "Cancel", content, m.win)
	d.Resize(fyne.NewSize(480, 320))
	d.Show()
}

// importFromFile shows a file open dialog filtered to spec file extensions,
// reads the contents, and parses them via importer.From.
func (m *MainUI) importFromFile() {
	open := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if rc == nil {
			return // cancelled
		}
		defer func() { _ = rc.Close() }()

		data, err := io.ReadAll(rc)
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		c, err := importer.From(data)
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.chooseImportDestination(c)
	}, m.win)
	open.SetFilter(fynestorage.NewExtensionFileFilter([]string{".yaml", ".yml", ".json", ".wsdl", ".xml"}))
	open.Resize(fyne.NewSize(640, 480))
	open.Show()
}

// importFromURL fetches and parses a spec from url off the UI goroutine.
func (m *MainUI) importFromURL(url string) {
	m.Status.SetText("Fetching " + url + "…")
	settings := m.sess.Settings()
	go func() {
		c, err := importer.FromURL(url, settings)
		fyne.Do(func() {
			if err != nil {
				m.Status.SetText("Fetch failed: " + err.Error())
				dialog.ShowError(err, m.win)
				return
			}
			m.Status.SetText("Fetched " + url)
			m.chooseImportDestination(c)
		})
	}()
}

func (m *MainUI) chooseImportDestination(c model.Collection) {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if uri == nil {
			return // cancelled
		}
		sub := uniqueCollectionDir(uri.Path(), c.Name)
		dir := filepath.Join(uri.Path(), sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if err := appstorage.Save(c, dir); err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		if err := m.sess.OpenCollection(dir); err != nil {
			dialog.ShowError(err, m.win)
			return
		}
		m.Tree.Refresh()
		m.refreshEnvironments()
		m.Status.SetText("Imported: " + c.Name)
	}, m.win)
}

var importSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := importSlugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "imported-collection"
	}
	return s
}

// uniqueCollectionDir returns a subdirectory name under parent that doesn't yet
// exist on disk, derived from name's slug with a numeric suffix when needed.
func uniqueCollectionDir(parent, name string) string {
	base := slugify(name)
	candidate := base
	for i := 2; ; i++ {
		if _, err := os.Stat(filepath.Join(parent, candidate)); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}
