package ui

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"github.com/idct/helena/internal/model"
	appstorage "github.com/idct/helena/internal/storage"
)

// actionNewCollection prompts for a collection name, then a parent directory
// on disk; it writes an empty OpenCollection YAML there and opens it in the
// active workspace. Reuses slugify / uniqueCollectionDir from import.go.
func (m *MainUI) actionNewCollection() {
	if m.win == nil {
		return
	}
	m.promptName("New collection", "Collection name", "New Collection", func(name string) {
		dialog.ShowFolderOpen(func(parent fyne.ListableURI, err error) {
			if err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			if parent == nil {
				return // cancelled
			}
			sub := uniqueCollectionDir(parent.Path(), name)
			dir := filepath.Join(parent.Path(), sub)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				dialog.ShowError(err, m.win)
				return
			}
			c := model.Collection{Name: name}
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
			m.refreshEmptyState()
			m.Status.SetText("Created: " + name)
		}, m.win)
	})
}
