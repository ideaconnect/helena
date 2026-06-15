package session

import (
	"fmt"
	"slices"
	"strings"

	"github.com/idct/helena/internal/config"
)

// workspaceNameTaken reports whether name (trimmed) already names a workspace
// other than the one at exceptIdx (pass -1 to consider all). Uniqueness is
// required because the workspace dropdown is keyed by display name — two
// identically-named workspaces collapse and the second can never be selected.
func (s *Session) workspaceNameTaken(name string, exceptIdx int) bool {
	name = strings.TrimSpace(name)
	for i, w := range s.cfg.Workspaces {
		if i == exceptIdx {
			continue
		}
		if strings.TrimSpace(w.Name) == name {
			return true
		}
	}
	return false
}

// AddWorkspace creates a new workspace and persists. The name must be non-empty
// and unique (the dropdown selects by name).
func (s *Session) AddWorkspace(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}
	if s.workspaceNameTaken(name, -1) {
		return fmt.Errorf("a workspace named %q already exists", strings.TrimSpace(name))
	}
	s.cfg.Workspaces = append(s.cfg.Workspaces, config.Workspace{Name: name})
	return s.persist()
}

// RenameWorkspace renames the workspace at index i and persists. The new name
// must be non-empty and not collide with another workspace.
func (s *Session) RenameWorkspace(i int, name string) error {
	if i < 0 || i >= len(s.cfg.Workspaces) {
		return fmt.Errorf("invalid workspace index %d", i)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}
	if s.workspaceNameTaken(name, i) {
		return fmt.Errorf("a workspace named %q already exists", strings.TrimSpace(name))
	}
	s.cfg.Workspaces[i].Name = name
	return s.persist()
}

// DeleteWorkspace removes the workspace at index i, reloads if the active
// workspace was affected, and persists. The last remaining workspace cannot be
// deleted (the app always needs one).
func (s *Session) DeleteWorkspace(i int) error {
	if i < 0 || i >= len(s.cfg.Workspaces) {
		return fmt.Errorf("invalid workspace index %d", i)
	}
	if len(s.cfg.Workspaces) <= 1 {
		return fmt.Errorf("cannot delete the last workspace")
	}
	s.cfg.Workspaces = slices.Delete(s.cfg.Workspaces, i, i+1)
	if s.cfg.Active >= len(s.cfg.Workspaces) {
		s.cfg.Active = len(s.cfg.Workspaces) - 1
	}
	if s.cfg.Active < 0 {
		s.cfg.Active = 0
	}
	s.reload()
	return s.persist()
}
