package config

import (
	"log"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/idct/helena/internal/model"
)

// CurrentSchemaVersion is the config-file schema version this build writes and
// migrates up to. Bump it whenever the on-disk shape changes incompatibly and
// add a matching entry to migrations. A file with no version is treated as
// version 0 (pre-versioning) and migrated forward.
const CurrentSchemaVersion = 1

// warnf is the sink for non-fatal config warnings (e.g. a future schema
// version). Package-level so tests can capture it; defaults to the logger.
var warnf = log.Printf

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

// UIOpenTab identifies one open editor tab by collection directory plus the
// target's persistent Request.ID. Anchoring by ID (not node path) keeps tab
// restoration stable across request reordering / insertion / deletion within
// the collection. Scratch tabs (unsaved, not in any collection) are not
// persisted, so every UIOpenTab references an on-disk request.
type UIOpenTab struct {
	Collection string `yaml:"collection,omitempty"`
	RequestID  string `yaml:"requestId,omitempty"`
}

// UIState holds restorable session state: which collection/environment/request
// the user had open, the open editor tabs, and the last window size.
type UIState struct {
	ActiveCollection string            `yaml:"activeCollection,omitempty"`
	ActiveEnv        map[string]string `yaml:"activeEnv,omitempty"` // collection dir -> env name
	OpenRequest      *UIOpenRequest    `yaml:"openRequest,omitempty"`
	OpenTabs         []UIOpenTab       `yaml:"openTabs,omitempty"`
	ActiveTab        int               `yaml:"activeTab,omitempty"`
	WindowWidth      int               `yaml:"windowWidth,omitempty"`
	WindowHeight     int               `yaml:"windowHeight,omitempty"`
}

// Config is Helena's persisted application state.
type Config struct {
	Version    int            `yaml:"version"`
	Workspaces []Workspace    `yaml:"workspaces"`
	Active     int            `yaml:"active"`
	Settings   model.Settings `yaml:"settings"`
	UI         UIState        `yaml:"ui,omitempty"`
	// Variables are global variables (#83): app-wide, persisted here in the
	// config, and the lowest-precedence resolver scope (every collection-,
	// environment-, and request-level value of the same name overrides them).
	Variables []model.Variable `yaml:"variables,omitempty"`
}

// Default returns a Config with a single empty "Default" workspace.
func Default() Config {
	return Config{
		Version:    CurrentSchemaVersion,
		Workspaces: []Workspace{{Name: "Default"}},
		Active:     0,
		Settings:   model.DefaultSettings(),
	}
}

// migrations maps a target schema version to the step that upgrades a config
// from the previous version to it. migrate() runs them in ascending order.
var migrations = map[int]func(*Config){
	1: migrateTo1,
}

// migrateTo1 upgrades a pre-versioning (v0) config to v1. Legacy configs may
// carry the old unsafe TimeoutSeconds=0 (which meant an unlimited timeout
// before the #97 guard); normalize it to the default so the upgrade can't
// silently leave a request without a timeout.
func migrateTo1(c *Config) {
	if c.Settings.TimeoutSeconds <= 0 {
		c.Settings.TimeoutSeconds = model.DefaultSettings().TimeoutSeconds
	}
}

// migrate brings a freshly-loaded config up to CurrentSchemaVersion by running
// each registered step in order. A config from a future build (Version >
// current) is left untouched and a warning is emitted — its known fields load
// fine, but newer keys it carries would be dropped if the app re-saves it, so
// we surface that rather than silently truncating.
func migrate(c Config) Config {
	if c.Version > CurrentSchemaVersion {
		warnf("config: file is schema version %d but this build understands %d; "+
			"loading best-effort — fields newer than v%d may be lost if you save",
			c.Version, CurrentSchemaVersion, CurrentSchemaVersion)
		return c
	}
	for c.Version < CurrentSchemaVersion {
		next := c.Version + 1
		if m := migrations[next]; m != nil {
			m(&c)
		}
		c.Version = next
	}
	return c
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
	// Seed defaults before unmarshalling so a file that omits a key — most
	// importantly the whole `settings:` block — keeps the safe defaults instead
	// of silently dropping to zero values (TimeoutSeconds=0 = no timeout,
	// FollowRedirects/CORSWarning=false). YAML only overwrites the keys it
	// actually contains. Workspaces is cleared so the file fully owns the list.
	c := Default()
	c.Workspaces = nil
	// Reset Version to 0 so a file that omits the key is recognised as a
	// pre-versioning config and migrated forward, rather than inheriting the
	// seeded current version.
	c.Version = 0
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if len(c.Workspaces) == 0 {
		c.Workspaces = Default().Workspaces
	}
	if c.Active < 0 || c.Active >= len(c.Workspaces) {
		c.Active = 0
	}
	return migrate(c), nil
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
