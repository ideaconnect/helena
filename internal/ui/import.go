package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	fynestorage "fyne.io/fyne/v2/storage"

	"github.com/idct/helena/internal/importer"
	"github.com/idct/helena/internal/model"
	appstorage "github.com/idct/helena/internal/storage"
)

// actionImport runs the import flow: pick a spec file (OpenAPI/Swagger or
// WSDL — format is auto-detected) → parse → pick a parent directory → write
// the new collection there → open it in the active workspace.
func (m *MainUI) actionImport() {
	if m.win == nil {
		return
	}
	d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
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
	d.SetFilter(fynestorage.NewExtensionFileFilter([]string{".yaml", ".yml", ".json", ".wsdl", ".xml"}))
	d.Resize(fyne.NewSize(640, 480))
	d.Show()
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
