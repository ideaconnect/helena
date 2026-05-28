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
	root := ocCollectionFile{Info: ocInfo{Name: c.Name, Type: "collection"}}
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
		ff := ocFolderFile{Info: ocInfo{Name: f.Name, Type: "folder", Seq: i + 1}}
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

func readRequestFile(path string) (ocRequestFile, error) {
	var rf ocRequestFile
	data, err := os.ReadFile(path)
	if err != nil {
		return rf, err
	}
	return rf, yaml.Unmarshal(data, &rf)
}

func readFolderFile(path string) (ocFolderFile, error) {
	var ff ocFolderFile
	data, err := os.ReadFile(path)
	if err != nil {
		return ff, err
	}
	return ff, yaml.Unmarshal(data, &ff)
}

func readCollectionFile(path string) (ocCollectionFile, error) {
	var cf ocCollectionFile
	data, err := os.ReadFile(path)
	if err != nil {
		return cf, err
	}
	return cf, yaml.Unmarshal(data, &cf)
}

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
			fols = append(fols, seqFol{ff.Info.Seq, model.Folder{
				ID:       model.NewID(),
				Name:     ff.Info.Name,
				Folders:  subFolders,
				Requests: subRequests,
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

func uniqueName(base string, used map[string]bool) string {
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}
