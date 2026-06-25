package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestFolderVarsResolveBetweenEnvAndRequest pins #81: folder variables resolve
// above the environment and below the request, inner folders override outer
// ones, disabled folder vars don't resolve, and ResolverForNode threads the
// ancestor folder scope.
func TestFolderVarsResolveBetweenEnvAndRequest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	req := model.Request{
		Name: "R", Method: model.GET, URL: "https://x/",
		Variables: []model.Variable{{Enabled: true, Key: "rkey", Value: "from-request"}},
	}
	col := model.Collection{
		Name: "C",
		Environments: []model.Environment{{
			Name:      "Dev",
			Variables: []model.Variable{{Enabled: true, Key: "k", Value: "from-env"}, {Enabled: true, Key: "eonly", Value: "env-only"}},
		}},
		Folders: []model.Folder{{
			Name: "Outer",
			Variables: []model.Variable{
				{Enabled: true, Key: "k", Value: "from-outer"},
				{Enabled: true, Key: "outer", Value: "outer-only"},
				{Enabled: false, Key: "off", Value: "nope"},
			},
			Folders: []model.Folder{{
				Name:      "Inner",
				Variables: []model.Variable{{Enabled: true, Key: "k", Value: "from-inner"}},
				Requests:  []model.Request{req},
			}},
		}},
	}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	s, _ := New("")
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	s.SetActiveEnv("Dev")

	nodeID := "0/f0/f0/r0" // Inner request
	loaded, _ := s.Tree().Request(nodeID)

	// {{k}}: inner folder (from-inner) overrides outer + env; request has no "k".
	// {{eonly}} from env, {{outer}} from outer folder, {{rkey}} from request.
	out, missing := s.ResolverForNode(nodeID, loaded).Resolve("{{k}}|{{eonly}}|{{outer}}|{{rkey}}")
	if len(missing) != 0 {
		t.Fatalf("unexpected missing: %v", missing)
	}
	if want := "from-inner|env-only|outer-only|from-request"; out != want {
		t.Errorf("resolve = %q; want %q", out, want)
	}

	// A disabled folder var does not resolve.
	if _, missing := s.ResolverForNode(nodeID, loaded).Resolve("{{off}}"); len(missing) != 1 {
		t.Errorf("disabled folder var should be unresolved; missing=%v", missing)
	}

	// SnapshotAncestorVars exposes the merged folder scope (inner wins).
	snap := s.SnapshotAncestorVars(nodeID)
	if snap["k"] != "from-inner" || snap["outer"] != "outer-only" {
		t.Errorf("ancestor snapshot = %v", snap)
	}

	// FolderVariables / SetFolderVariables on the Outer folder persist.
	if fv, ok := s.FolderVariables("0/f0"); !ok || len(fv) != 3 {
		t.Errorf("FolderVariables(0/f0) = %v, %v", fv, ok)
	}
	if err := s.SetFolderVariables("0/f0", []model.Variable{{Enabled: true, Key: "new", Value: "v"}}); err != nil {
		t.Fatalf("SetFolderVariables: %v", err)
	}
	if fv, _ := s.FolderVariables("0/f0"); len(fv) != 1 || fv[0].Key != "new" {
		t.Errorf("folder vars after set = %v", fv)
	}
	// A non-folder node is rejected.
	if _, ok := s.FolderVariables(nodeID); ok {
		t.Error("FolderVariables on a request node should be false")
	}
}
