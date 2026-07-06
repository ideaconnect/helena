package ui

import (
	"context"
	"net/url"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/idct/helena/internal/updatecheck"
)

// fetchLatestRelease is the update-check fetch, indirected through a package var
// so tests can substitute a fake without a network call. Production points it at
// the real GitHub fetch.
var fetchLatestRelease = updatecheck.LatestGitHubRelease

// buildStatusBar assembles the bottom bar: the transient status line on the
// leading edge (m.Status, still driven by SetText everywhere) and, on the
// trailing edge, the current version plus an opt-in "Check for updates" control.
// The version segment needs no network; the check runs only when clicked, so the
// app still makes no background/automatic requests (the no-phone-home guarantee).
func (m *MainUI) buildStatusBar() fyne.CanvasObject {
	m.statusVersion = widget.NewLabel(currentVersionText(m.appVersion))

	m.updateStatus = widget.NewLabel("")
	m.updateStatus.Hide()

	m.updateLink = widget.NewHyperlink("", nil)
	m.updateLink.Hide()

	m.updateCheckBtn = widget.NewButton("Check for updates", m.checkForUpdates)
	m.updateCheckBtn.Importance = widget.LowImportance

	trailing := container.NewHBox(
		m.statusVersion,
		m.updateCheckBtn,
		m.updateStatus,
		m.updateLink,
	)

	// Windows: also offer a direct Store link. The Store shows and installs the
	// latest version there; there is no stable public API for the Store's current
	// version number, so we link rather than scrape it. Other platforms get the
	// GitHub check only.
	if runtime.GOOS == "windows" {
		storeURL, _ := url.Parse(updatecheck.StorePageURL) // const is a valid URL
		trailing.Add(widget.NewHyperlink("Microsoft Store", storeURL))
	}

	return container.NewBorder(nil, nil, m.Status, trailing)
}

// currentVersionText renders the running build's version for the status bar. A
// local build reports "dev"; a released build reports its tag (e.g. "v0.4.0").
func currentVersionText(v string) string {
	if v == "" || v == "dev" {
		return "dev build"
	}
	return v
}

// checkForUpdates fetches the latest GitHub release off the UI goroutine and
// reports whether a newer version exists. It runs ONLY on an explicit click —
// nothing here is automatic — so it does not breach the no-background-traffic
// guarantee. The fetch and comparison happen off-thread; only the final widget
// updates marshal back via fyne.Do.
func (m *MainUI) checkForUpdates() {
	m.updateCheckBtn.Disable()
	m.updateLink.Hide()
	m.updateStatus.SetText("Checking…")
	m.updateStatus.Show()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fyne.Do(func() {
					m.updateCheckBtn.Enable()
					m.updateStatus.SetText("Update check failed")
				})
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		rel, err := fetchLatestRelease(ctx, updatecheck.DefaultClient())

		fyne.Do(func() { m.applyUpdateResult(rel, err) })
	}()
}

// applyUpdateResult renders the outcome of a check into the status-bar widgets.
// It runs on the UI goroutine (via fyne.Do, or directly in tests) and is the
// synchronous, testable core of checkForUpdates.
func (m *MainUI) applyUpdateResult(rel updatecheck.Release, err error) {
	m.updateCheckBtn.Enable()
	if err != nil {
		m.updateStatus.SetText("Update check failed")
		return
	}
	switch updatecheck.Compare(m.appVersion, rel.Tag) {
	case updatecheck.StatusUpdateAvailable:
		m.updateStatus.SetText("Update available: " + rel.Tag)
		_ = m.updateLink.SetURLFromString(rel.HTMLURL)
		m.updateLink.SetText("Download")
		m.updateLink.Show()
	case updatecheck.StatusUpToDate:
		m.updateStatus.SetText("Up to date (" + rel.Tag + ")")
	case updatecheck.StatusAhead:
		m.updateStatus.SetText("Ahead of latest (" + rel.Tag + ")")
	default: // StatusUnknown — e.g. a dev build with no comparable version
		m.updateStatus.SetText("Latest release: " + rel.Tag)
	}
}
