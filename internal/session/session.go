// Package session ties persisted config to collections loaded from disk and
// exposes them for the UI: workspace switching, a tree navigation model, and
// the active collection/environment used to resolve {{variables}}.
package session

import (
	"github.com/idct/helena/internal/config"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
	"github.com/idct/helena/internal/vars"
)

// Session is the in-memory application state for the active workspace.
type Session struct {
	cfgPath   string
	cfg       config.Config
	cols      []model.Collection // collections loaded for the active workspace
	activeCol int                // index into cols, or -1 when none
	activeEnv map[int]string     // collection index -> active environment name
}

// New loads the config at cfgPath (empty path = defaults, no persistence) and
// the collections of the active workspace.
func New(cfgPath string) (*Session, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	s := &Session{cfgPath: cfgPath, cfg: cfg, activeEnv: map[int]string{}}
	s.reload()
	return s, nil
}

func (s *Session) reload() {
	s.cols = nil
	for _, dir := range s.activeWorkspace().Collections {
		c, err := storage.Load(dir)
		if err != nil {
			continue // skip collections that no longer load; surfaced in UI later
		}
		s.cols = append(s.cols, c)
	}
	if len(s.cols) > 0 {
		s.activeCol = 0
	} else {
		s.activeCol = -1
	}
}

func (s *Session) activeWorkspace() config.Workspace {
	if s.cfg.Active < 0 || s.cfg.Active >= len(s.cfg.Workspaces) {
		return config.Workspace{}
	}
	return s.cfg.Workspaces[s.cfg.Active]
}

// WorkspaceNames returns the names of all workspaces, in order.
func (s *Session) WorkspaceNames() []string {
	out := make([]string, len(s.cfg.Workspaces))
	for i, w := range s.cfg.Workspaces {
		out[i] = w.Name
	}
	return out
}

// ActiveIndex returns the index of the active workspace.
func (s *Session) ActiveIndex() int { return s.cfg.Active }

// SetActive switches the active workspace, reloads its collections, and persists.
func (s *Session) SetActive(i int) {
	if i < 0 || i >= len(s.cfg.Workspaces) || i == s.cfg.Active {
		return
	}
	s.cfg.Active = i
	s.reload()
	_ = s.persist()
}

// Collections returns the collections loaded for the active workspace.
func (s *Session) Collections() []model.Collection { return s.cols }

// OpenCollection loads an OpenCollection directory, adds it to the active
// workspace, makes it the active collection, and persists the change.
func (s *Session) OpenCollection(dir string) error {
	c, err := storage.Load(dir)
	if err != nil {
		return err
	}
	w := &s.cfg.Workspaces[s.cfg.Active]
	w.Collections = append(w.Collections, dir)
	s.cols = append(s.cols, c)
	s.activeCol = len(s.cols) - 1
	return s.persist()
}

func (s *Session) persist() error {
	if s.cfgPath == "" {
		return nil
	}
	return config.Save(s.cfgPath, s.cfg)
}

// Tree returns a navigation model over the currently loaded collections.
func (s *Session) Tree() *Tree { return &Tree{cols: s.cols} }

// ActiveCollection returns the index of the active collection, or -1.
func (s *Session) ActiveCollection() int { return s.activeCol }

// SetActiveCollection sets which collection the environment selector and
// resolver apply to (-1 for none).
func (s *Session) SetActiveCollection(i int) {
	if i >= -1 && i < len(s.cols) {
		s.activeCol = i
	}
}

// CollectionEnvironmentNames lists the environment names of the active collection.
func (s *Session) CollectionEnvironmentNames() []string {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	envs := s.cols[s.activeCol].Environments
	out := make([]string, len(envs))
	for i, e := range envs {
		out[i] = e.Name
	}
	return out
}

// ActiveEnvName returns the active environment name for the active collection.
func (s *Session) ActiveEnvName() string {
	if s.activeEnv == nil {
		return ""
	}
	return s.activeEnv[s.activeCol]
}

// SetActiveEnv sets the active environment (by name) for the active collection.
// An empty name means "no environment".
func (s *Session) SetActiveEnv(name string) {
	if s.activeEnv == nil {
		s.activeEnv = map[int]string{}
	}
	s.activeEnv[s.activeCol] = name
}

// ActiveEnvironment returns a pointer to the active environment, or nil.
func (s *Session) ActiveEnvironment() *model.Environment {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	name := s.ActiveEnvName()
	for i := range s.cols[s.activeCol].Environments {
		if s.cols[s.activeCol].Environments[i].Name == name {
			return &s.cols[s.activeCol].Environments[i]
		}
	}
	return nil
}

// AddEnvironment appends a new environment to the active collection.
func (s *Session) AddEnvironment(name string) {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return
	}
	s.cols[s.activeCol].Environments = append(s.cols[s.activeCol].Environments,
		model.Environment{ID: model.NewID(), Name: name})
}

// SetActiveEnvironmentVariables replaces the active environment's variables.
func (s *Session) SetActiveEnvironmentVariables(variables []model.Variable) {
	if e := s.ActiveEnvironment(); e != nil {
		e.Variables = variables
	}
}

// Resolver builds a variable resolver from the active collection's active
// environment (enabled variables only).
func (s *Session) Resolver() *vars.Resolver {
	return vars.New(s.activeEnvVars())
}

func (s *Session) activeEnvVars() map[string]string {
	m := map[string]string{}
	e := s.ActiveEnvironment()
	if e == nil {
		return m
	}
	for _, v := range e.Variables {
		if v.Enabled {
			m[v.Key] = v.Value
		}
	}
	return m
}

// SaveActiveCollection writes the active collection back to its source directory.
func (s *Session) SaveActiveCollection() error {
	if s.activeCol < 0 || s.activeCol >= len(s.cols) {
		return nil
	}
	dirs := s.activeWorkspace().Collections
	if s.activeCol >= len(dirs) {
		return nil
	}
	return storage.Save(s.cols[s.activeCol], dirs[s.activeCol])
}
