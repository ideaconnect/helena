package ui

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2/test"

	"github.com/idct/helena/internal/session"
)

// BenchmarkNewMainUI measures full main-UI construction — the dominant part of
// startup after collection loading. It is sensitive to the number of Fyne
// theme scopes created (each container.NewThemeOverride call mints a fresh
// scope: Fyne re-parses fonts per scope × text style and walks the wrapped
// subtree creating every renderer), so it guards the scope-reduction work.
func BenchmarkNewMainUI(b *testing.B) {
	test.NewApp()
	dir := writeTabTestCollection(b)
	sess, err := session.New(filepath.Join(b.TempDir(), "config.yml"))
	if err != nil {
		b.Fatalf("session.New: %v", err)
	}
	if err := sess.OpenCollection(dir); err != nil {
		b.Fatalf("OpenCollection: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewMainUI(sess)
	}
}
