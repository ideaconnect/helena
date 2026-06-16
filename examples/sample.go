// Package examples embeds Helena's bundled sample collection so a downloaded
// binary — which has no access to the source tree — can still materialize and
// load it from a "Load sample" action.
package examples

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// sampleFS holds the bundled httpbin sample collection (an OpenCollection
// directory). The external smoke test loads httpbin/ from disk; this embed
// makes the same files reachable at runtime in a shipped binary.
//
//go:embed httpbin
var sampleFS embed.FS

// SampleName is the on-disk subdirectory the sample is written to.
const SampleName = "httpbin"

// WriteSample materializes the embedded sample collection into
// destDir/<SampleName> (creating directories as needed) and returns the
// collection directory. Existing files are overwritten, so a repeated load
// always yields the pristine sample.
func WriteSample(destDir string) (string, error) {
	root := filepath.Join(destDir, SampleName)
	err := fs.WalkDir(sampleFS, SampleName, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(SampleName, p)
		if err != nil {
			return err
		}
		target := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := sampleFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return "", err
	}
	return root, nil
}
