package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

// Workspace records a named workspace and the on-disk collection directories it
// references. Collections themselves live in OpenCollection YAML folders; only
// their paths are persisted here.
type Workspace struct {
	Name        string   `yaml:"name"`
	Collections []string `yaml:"collections,omitempty"`
}

// UIOpenRequest identifies the currently open request by collection directory
// path plus the in-collection node path (e.g. "f0/r1"). Storing by path rather
// than index keeps restoration stable across collection reordering.
type UIOpenRequest struct {
	Collection string `yaml:"collection,omitempty"`
	NodePath   string `yaml:"path,omitempty"`
}

// UIState holds restorable session state: which collection/environment/request
// the user had open, and the last window size.
type UIState struct {
	ActiveCollection string            `yaml:"activeCollection,omitempty"`
	ActiveEnv        map[string]string `yaml:"activeEnv,omitempty"` // collection dir -> env name
	OpenRequest      *UIOpenRequest    `yaml:"openRequest,omitempty"`
	WindowWidth      int               `yaml:"windowWidth,omitempty"`
	WindowHeight     int               `yaml:"windowHeight,omitempty"`
}

// Config is Helena's persisted application state.
type Config struct {
	Workspaces []Workspace    `yaml:"workspaces"`
	Active     int            `yaml:"active"`
	Settings   model.Settings `yaml:"settings"`
	UI         UIState        `yaml:"ui,omitempty"`
}

// Default returns a Config with a single empty "Default" workspace.
func Default() Config {
	return Config{
		Workspaces: []Workspace{{Name: "Default"}},
		Active:     0,
		Settings:   model.DefaultSettings(),
	}
}

// DefaultPath returns the standard config file location (…/helena/config.yml).
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "helena", "config.yml"), nil
}

// Load reads the config at path. A missing file (or an empty path) yields the
// default config rather than an error.
func Load(path string) (Config, error) {
	if path == "" {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if len(c.Workspaces) == 0 {
		c.Workspaces = Default().Workspaces
	}
	if c.Active < 0 || c.Active >= len(c.Workspaces) {
		c.Active = 0
	}
	return c, nil
}

// Save writes the config to path, creating parent directories as needed.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
