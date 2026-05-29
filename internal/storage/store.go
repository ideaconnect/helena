package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

const (
	collectionFile  = "opencollection.yml"
	folderFile      = "folder.yml"
	environmentsDir = "environments"
	ymlExt          = ".yml"
)

// Save writes the collection to dir using the OpenCollection YAML layout,
// creating directories as needed: opencollection.yml at the root, one file per
// environment under environments/, and request/folder files (folders become
// subdirectories with a folder.yml). After writing, orphan files and folders
// not produced by this save are removed.
func Save(c model.Collection, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Preserve any unknown top-level fields from a prior load.
	root := ocCollectionFile{
		Info: ocInfo{Name: c.Name, Type: "collection"},
		Auth: authToFile(c.Auth),
	}
	if existing, err := readCollectionFile(filepath.Join(dir, collectionFile)); err == nil {
		root.Extra = existing.Extra
		// Merge Info.Extra (unknown info fields) too.
		root.Info.Extra = existing.Info.Extra
		root.Info.Tags = existing.Info.Tags
	}
	if err := writeYAML(filepath.Join(dir, collectionFile), root); err != nil {
		return err
	}

	envDir := filepath.Join(dir, environmentsDir)
	envKeep := map[string]bool{}
	if len(c.Environments) > 0 {
		if err := os.MkdirAll(envDir, 0o755); err != nil {
			return err
		}
		used := map[string]bool{}
		for i, e := range c.Environments {
			name := uniqueName(slug(e.Name, fmt.Sprintf("env-%d", i+1)), used)
			envKeep[name+ymlExt] = true
			ef := envToFile(e, i+1)
			// Preserve unknown fields from a prior environment file with the same slug.
			if prev, err := readEnvFile(filepath.Join(envDir, name+ymlExt)); err == nil {
				ef.Extra = prev.Extra
				ef.Info.Extra = prev.Info.Extra
			}
			if err := writeYAML(filepath.Join(envDir, name+ymlExt), ef); err != nil {
				return err
			}
		}
	}
	if err := sweepDir(envDir, envKeep); err != nil {
		return err
	}

	return saveItems(dir, c.Folders, c.Requests)
}

// saveItems writes the requests and folders of one container (collection root
// or folder) into dir, recursing into subfolders. For each item it preserves
// the Extra catch-alls from any existing file with the same slug so unknown
// fields round-trip, then sweeps orphan entries.
func saveItems(dir string, folders []model.Folder, requests []model.Request) error {
	used := map[string]bool{
		strings.TrimSuffix(collectionFile, ymlExt): true,
		strings.TrimSuffix(folderFile, ymlExt):     true,
		environmentsDir:                            true,
	}
	keep := map[string]bool{
		collectionFile:  true,
		folderFile:      true,
		environmentsDir: true,
	}

	for i, r := range requests {
		name := uniqueName(slug(r.Name, fmt.Sprintf("request-%d", i+1)), used)
		keep[name+ymlExt] = true
		rf := requestToFile(r, i+1)
		// Preserve unknown fields from a prior request file with the same slug.
		if prev, err := readRequestFile(filepath.Join(dir, name+ymlExt)); err == nil {
			rf.Extra = prev.Extra
			rf.Info.Extra = prev.Info.Extra
			rf.Info.Tags = prev.Info.Tags
			if rf.HTTP != nil && prev.HTTP != nil {
				rf.HTTP.Extra = prev.HTTP.Extra
			}
			// Scripts: if the user cleared both hooks, scriptsToFile returns
			// nil — but the on-disk scripts block may have carried sibling
			// keys (e.g. a tests: block from another tool) under Scripts.Extra.
			// Synthesize an empty DTO carrying that Extra so the round-trip
			// preserves it (invariant 1).
			if prev.Scripts != nil && len(prev.Scripts.Extra) > 0 {
				if rf.Scripts == nil {
					rf.Scripts = &ocScripts{}
				}
				rf.Scripts.Extra = prev.Scripts.Extra
			}
			// Chain: pair entries by alias and copy the prior file's Extra
			// catch-all into the matching new entry so any per-entry keys
			// other tools authored survive the round-trip.
			if len(prev.Chain) > 0 && len(rf.Chain) > 0 {
				byAlias := make(map[string]map[string]yaml.Node, len(prev.Chain))
				for _, p := range prev.Chain {
					if p.Alias != "" && len(p.Extra) > 0 {
						byAlias[p.Alias] = p.Extra
					}
				}
				for i := range rf.Chain {
					if e, ok := byAlias[rf.Chain[i].Alias]; ok {
						rf.Chain[i].Extra = e
					}
				}
			}
		}
		if err := writeYAML(filepath.Join(dir, name+ymlExt), rf); err != nil {
			return err
		}
	}
	for i, f := range folders {
		name := uniqueName(slug(f.Name, fmt.Sprintf("folder-%d", i+1)), used)
		keep[name] = true
		sub := filepath.Join(dir, name)
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return err
		}
		ff := ocFolderFile{
			Info: ocInfo{Name: f.Name, Type: "folder", Seq: i + 1},
			Auth: authToFile(f.Auth),
		}
		if prev, err := readFolderFile(filepath.Join(sub, folderFile)); err == nil {
			ff.Extra = prev.Extra
			ff.Info.Extra = prev.Info.Extra
		}
		if err := writeYAML(filepath.Join(sub, folderFile), ff); err != nil {
			return err
		}
		if err := saveItems(sub, f.Folders, f.Requests); err != nil {
			return err
		}
	}
	return sweepDir(dir, keep)
}

// sweepDir removes .yml files and folder-style subdirectories in dir that
// aren't in keep. Other entries (the environments/ directory, random files the
// user added) are left untouched.
func sweepDir(dir string, keep map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if keep[name] {
			continue
		}
		full := filepath.Join(dir, name)
		if e.IsDir() {
			// Only delete subdirectories that look like collection folders.
			if _, err := os.Stat(filepath.Join(full, folderFile)); err == nil {
				if err := os.RemoveAll(full); err != nil {
					return err
				}
			}
			continue
		}
		if strings.HasSuffix(name, ymlExt) {
			if err := os.Remove(full); err != nil {
				return err
			}
		}
	}
	return nil
}

// readRequestFile loads a request DTO from path, preserving Extra fields for
// later re-marshalling.
func readRequestFile(path string) (ocRequestFile, error) {
	var rf ocRequestFile
	data, err := os.ReadFile(path)
	if err != nil {
		return rf, err
	}
	return rf, yaml.Unmarshal(data, &rf)
}

// readFolderFile loads a folder DTO from path.
func readFolderFile(path string) (ocFolderFile, error) {
	var ff ocFolderFile
	data, err := os.ReadFile(path)
	if err != nil {
		return ff, err
	}
	return ff, yaml.Unmarshal(data, &ff)
}

// readCollectionFile loads the collection-root DTO from path.
func readCollectionFile(path string) (ocCollectionFile, error) {
	var cf ocCollectionFile
	data, err := os.ReadFile(path)
	if err != nil {
		return cf, err
	}
	return cf, yaml.Unmarshal(data, &cf)
}

// readEnvFile loads an environment DTO from path.
func readEnvFile(path string) (ocEnvironmentFile, error) {
	var ef ocEnvironmentFile
	data, err := os.ReadFile(path)
	if err != nil {
		return ef, err
	}
	return ef, yaml.Unmarshal(data, &ef)
}

// Load reads a collection from an OpenCollection YAML directory. Item IDs are
// not part of the format, so fresh ones are assigned on load.
func Load(dir string) (model.Collection, error) {
	c := model.Collection{ID: model.NewID()}

	data, err := os.ReadFile(filepath.Join(dir, collectionFile))
	if err != nil {
		return c, fmt.Errorf("read %s: %w", collectionFile, err)
	}
	var root ocCollectionFile
	if err := yaml.Unmarshal(data, &root); err != nil {
		return c, fmt.Errorf("parse %s: %w", collectionFile, err)
	}
	c.Name = root.Info.Name
	if root.Auth != nil {
		c.Auth = fileToAuth(root.Auth)
	} else {
		// Collection root has no parent to inherit from, so the default is None.
		c.Auth = model.Auth{Type: model.AuthNone}
	}

	envs, err := loadEnvironments(filepath.Join(dir, environmentsDir))
	if err != nil {
		return c, err
	}
	c.Environments = envs

	folders, requests, err := loadItems(dir)
	if err != nil {
		return c, err
	}
	c.Folders = folders
	c.Requests = requests
	return c, nil
}

// loadEnvironments reads every .yml file under dir as an environment file,
// sorted by their info.seq, returning nil when the directory is absent.
func loadEnvironments(dir string) ([]model.Environment, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type seqEnv struct {
		seq int
		env model.Environment
	}
	var collected []seqEnv
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ymlExt) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var ef ocEnvironmentFile
		if err := yaml.Unmarshal(b, &ef); err != nil {
			return nil, fmt.Errorf("parse environment %s: %w", e.Name(), err)
		}
		collected = append(collected, seqEnv{ef.Info.Seq, fileToEnv(ef)})
	}
	sort.SliceStable(collected, func(i, j int) bool { return collected[i].seq < collected[j].seq })
	var out []model.Environment
	for _, x := range collected {
		out = append(out, x.env)
	}
	return out, nil
}

// loadItems walks one container directory: subdirectories that contain a
// folder.yml become folders (recursing), .yml files of type http (or with no
// type) become requests. Both lists are ordered by their info.seq.
func loadItems(dir string) ([]model.Folder, []model.Request, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}

	type seqReq struct {
		seq int
		req model.Request
	}
	type seqFol struct {
		seq int
		fol model.Folder
	}
	var reqs []seqReq
	var fols []seqFol

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if name == environmentsDir {
				continue
			}
			sub := filepath.Join(dir, name)
			ffData, err := os.ReadFile(filepath.Join(sub, folderFile))
			if err != nil {
				if os.IsNotExist(err) {
					continue // not an OpenCollection folder
				}
				return nil, nil, err
			}
			var ff ocFolderFile
			if err := yaml.Unmarshal(ffData, &ff); err != nil {
				return nil, nil, fmt.Errorf("parse %s in %s: %w", folderFile, name, err)
			}
			subFolders, subRequests, err := loadItems(sub)
			if err != nil {
				return nil, nil, err
			}
			folderAuth := model.Auth{Type: model.AuthInherit}
			if ff.Auth != nil {
				folderAuth = fileToAuth(ff.Auth)
			}
			fols = append(fols, seqFol{ff.Info.Seq, model.Folder{
				ID:       model.NewID(),
				Name:     ff.Info.Name,
				Folders:  subFolders,
				Requests: subRequests,
				Auth:     folderAuth,
			}})
			continue
		}

		if name == collectionFile || name == folderFile || !strings.HasSuffix(name, ymlExt) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		var rf ocRequestFile
		if err := yaml.Unmarshal(b, &rf); err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", name, err)
		}
		if rf.Info.Type != "" && rf.Info.Type != "http" {
			continue
		}
		reqs = append(reqs, seqReq{rf.Info.Seq, fileToRequest(rf)})
	}

	sort.SliceStable(reqs, func(i, j int) bool { return reqs[i].seq < reqs[j].seq })
	sort.SliceStable(fols, func(i, j int) bool { return fols[i].seq < fols[j].seq })

	var folders []model.Folder
	for _, x := range fols {
		folders = append(folders, x.fol)
	}
	var requests []model.Request
	for _, x := range reqs {
		requests = append(requests, x.req)
	}
	return folders, requests, nil
}

// writeYAML marshals v and writes it to path with 0644 permissions.
func writeYAML(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slug turns a display name into a filesystem-friendly base name, falling back
// to fallback when the name has no usable characters.
func slug(name, fallback string) string {
	s := nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return fallback
	}
	return s
}

// uniqueName returns base, or base with a `-2`, `-3`, … suffix that has not
// yet been used. The picked name is recorded in used so further calls keep
// producing fresh names.
func uniqueName(base string, used map[string]bool) string {
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}
