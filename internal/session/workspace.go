package session

import (
	"fmt"
	"slices"
	"strings"

	"github.com/idct/helena/internal/config"
)

// AddWorkspace creates a new workspace and persists.
func (s *Session) AddWorkspace(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("workspace name cannot be empty")
	}
	s.cfg.Workspaces = append(s.cfg.Workspaces, config.Workspace{Name: name})
	return s.persist()
}

// RenameWorkspace renames the workspace at index i and persists.
func (s *Session) RenameWorkspace(i int, name string) error {
	if i < 0 || i >= len(s.cfg.Workspaces) {
		return fmt.Errorf("invalid workspace index %d", i)
	}
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("workspace name cannot be empty")
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
