//go:build darwin

// Command harness validates, on a real signed + sandboxed macOS build, the
// full "Option A" file-access loop Helena needs for the Mac App Store:
//
//	pick a folder via the native NSOpenPanel (powerbox)
//	 -> mint an app-scope security-scoped bookmark
//	 -> persist it to a container-internal sidecar file
//	 -> (quit + relaunch)
//	 -> resolve the bookmark + startAccessingSecurityScopedResource
//	 -> exercise Load (os.ReadDir) AND both Save strategies
//
// It answers the one thing a plain `go build` cannot: a dev build gets an
// unrestricted sandbox that masks every failure, so this must run codesigned
// with the App Sandbox entitlements. See README.md.
//
// Run it TWICE from a terminal:
//
//	./HelenaSandboxHarness.app/Contents/MacOS/harness   # first run: pick a folder
//	./HelenaSandboxHarness.app/Contents/MacOS/harness   # relaunch: the real test
package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// AppKit must be driven from the process main thread. Locking in init() keeps
// the main goroutine pinned to the main OS thread for the whole program (the
// standard Go+AppKit idiom), so our NSOpenPanel calls are main-thread-safe.
func init() { runtime.LockOSThread() }

func main() {
	fmt.Println("== Helena macOS App Sandbox validation harness ==")
	cfg, err := os.UserConfigDir()
	if err != nil {
		fmt.Println("os.UserConfigDir:", err)
		return
	}
	// Under the sandbox this resolves INSIDE the container
	// (~/Library/Containers/tech.idct.helena.harness/Data/...); if it doesn't,
	// the app isn't actually sandboxed — re-check the signing/entitlements.
	fmt.Printf("config dir (os.UserConfigDir): %s\n", cfg)
	side := filepath.Join(cfg, "helena-harness", "bookmark.b64")

	if raw, err := os.ReadFile(side); err == nil {
		relaunch(string(raw))
	} else {
		firstRun(side)
	}
	networkCheck()
	fmt.Println("\n(done)")
}

// firstRun presents the panel, mints + saves the bookmark, and runs the
// same-session Load/Save test while the fresh powerbox grant is still held.
func firstRun(side string) {
	fmt.Println("\n[FIRST RUN] no saved bookmark — presenting NSOpenPanel.")
	fmt.Println("            Pick a NEW empty folder OUTSIDE ~/Library/Containers")
	fmt.Println("            (e.g. make ~/Documents/helena-sbtest and choose it).")

	path, b64, err := pickAndBookmark()
	switch {
	case err != nil:
		fmt.Println("PICK: FAIL:", err)
		return
	case path == "":
		fmt.Println("PICK: cancelled — re-run to try again.")
		return
	}
	fmt.Println("PICK: ok ->", path)
	fmt.Printf("bookmark: %d base64 chars\n", len(b64))

	if err := os.MkdirAll(filepath.Dir(side), 0o700); err != nil {
		fmt.Println("mkdir sidecar dir: FAIL:", err)
		return
	}
	if err := os.WriteFile(side, []byte(b64), 0o600); err != nil {
		fmt.Println("save bookmark: FAIL:", err)
		return
	}
	fmt.Println("bookmark saved ->", side)

	fmt.Println("\n-- SAME-SESSION Load/Save test (fresh powerbox grant held) --")
	runLoadSaveTests(path)

	fmt.Println("\n>>> Now QUIT and run the harness AGAIN — the relaunch run is the real test.")
}

// relaunch resolves the persisted bookmark (no fresh powerbox grant) and runs
// the Load/Save test under ONLY the bookmark scope — this is what happens on
// every Helena launch after the first.
func relaunch(b64 string) {
	fmt.Println("\n[RELAUNCH] resolving the saved bookmark (no fresh powerbox grant)…")
	path, stale, err := resolveAndStart(b64)
	if err != nil {
		fmt.Println("RESOLVE+START: FAIL:", err)
		fmt.Println("  If this says the entitlement is missing, the app-scope bookmark")
		fmt.Println("  entitlement or signing is wrong — see README.md.")
		return
	}
	defer stopAccess()
	fmt.Printf("RESOLVE+START: ok -> %s (stale=%v)\n", path, stale)
	if stale {
		fmt.Println("  note: bookmark is STALE — the real code must re-mint + persist it here.")
	}
	fmt.Println("\n-- ACROSS-RELAUNCH Load/Save test (ONLY the bookmark scope held) --")
	runLoadSaveTests(path)
}

// runLoadSaveTests exercises, against dir treated as a Helena collection dir:
//   - LOAD:   os.ReadDir(dir)                         (in-scope read)
//   - SAVE-I: stage a temp dir INSIDE dir             (in-scope write)
//   - SAVE-P: stage a sibling temp dir in dir's PARENT (Helena's CURRENT
//     storage.Save pattern — a write OUTSIDE the bookmarked scope)
//
// The decisive signal is on the RELAUNCH run: if SAVE-P fails but SAVE-I
// passes, the storage.Save blocker is confirmed and Save must be restructured
// to stage inside the collection dir.
func runLoadSaveTests(dir string) {
	ents, err := os.ReadDir(dir)
	report("LOAD   os.ReadDir(collectionDir)", err)
	if err == nil {
		fmt.Printf("         (%d entries)\n", len(ents))
	}

	report("SAVE-I stage temp INSIDE collection dir", tryStage(filepath.Join(dir, ".harness-save-inside")))
	report("SAVE-P stage sibling in PARENT (Helena's current Save)", tryStage(dir+".harness-save-sibling"))

	fmt.Println("   interpretation: on the RELAUNCH run, SAVE-P FAIL + SAVE-I PASS")
	fmt.Println("   == confirmed: storage.Save must stage temp dirs inside the collection dir.")
}

// tryStage mimics storage.Save's staging: create a temp dir at p and write an
// opencollection.yml into it, then clean up. Returns the first error.
func tryStage(p string) error {
	if err := os.MkdirAll(p, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(p)
	return os.WriteFile(filepath.Join(p, "opencollection.yml"), []byte("name: harness\n"), 0o644)
}

func report(label string, err error) {
	if err == nil {
		fmt.Printf("  %-52s PASS\n", label)
	} else {
		fmt.Printf("  %-52s FAIL: %v\n", label, err)
	}
}

// networkCheck validates the com.apple.security.network.client entitlement —
// Helena is an HTTP client, so an outbound request must succeed under the sandbox.
func networkCheck() {
	fmt.Println("\n-- network.client entitlement check --")
	c := &http.Client{Timeout: 8 * time.Second}
	resp, err := c.Get("https://example.com")
	if err != nil {
		fmt.Println("  HTTPS GET example.com  FAIL:", err)
		return
	}
	_ = resp.Body.Close()
	fmt.Printf("  HTTPS GET example.com  PASS (%s)\n", resp.Status)
}
