package ventoy

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Config struct {
	MenuAlias []Alias `json:"menu_alias"`
	raw       map[string]json.RawMessage
}
type Alias struct {
	Image string `json:"image,omitempty"`
	Dir   string `json:"dir,omitempty"`
	Alias string `json:"alias"`
}

// SearchRoot returns Ventoy's configured image search root. Ventoy paths are
// always POSIX-style paths rooted at the data partition, not host paths.
func (c *Config) SearchRoot() string {
	var controls []map[string]string
	if raw := c.raw["control"]; raw != nil && json.Unmarshal(raw, &controls) == nil {
		for _, control := range controls {
			if root, ok := control["VTOY_DEFAULT_SEARCH_ROOT"]; ok {
				root = path.Clean("/" + strings.TrimPrefix(root, "/"))
				if root != "/" && !strings.HasPrefix(root, "/../") {
					return root
				}
			}
		}
	}
	return "/"
}

// Load reads and validates the Ventoy configuration in dir.
func Load(dir string) (*Config, error) {
	b, err := os.ReadFile(filepath.Join(dir, "ventoy.json"))
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("invalid ventoy.json: %w", err)
	}
	var aliases []Alias
	if b := raw["menu_alias"]; b != nil {
		if err := json.Unmarshal(b, &aliases); err != nil {
			return nil, err
		}
	}
	return &Config{MenuAlias: aliases, raw: raw}, nil
}

// SetAliasForPath updates every menu image glob that currently matches the old ISO.
// SetAliasForPath assigns canonical to every image alias that matches oldISO.
func (c *Config) SetAliasForPath(oldISO, canonical string) bool {
	changed := false
	for n := range c.MenuAlias {
		a := &c.MenuAlias[n]
		if a.Image == "" {
			continue
		}
		matched, _ := filepath.Match(a.Image, "/"+filepath.ToSlash(filepath.Clean(oldISO)))
		if matched && a.Alias != canonical {
			a.Alias, changed = canonical, true
		}
	}
	return changed
}

// Save atomically persists the configuration to dir/ventoy.json.
func (c *Config) Save(dir string) error {
	b, err := json.MarshalIndent(c.MenuAlias, "", "    ")
	if err != nil {
		return err
	}
	c.raw["menu_alias"] = b
	all, err := json.MarshalIndent(c.raw, "", "    ")
	if err != nil {
		return err
	}
	target := filepath.Join(dir, "ventoy.json")
	tmp, err := os.CreateTemp(dir, ".ventoy.json-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(append(all, '\n')); err == nil {
		err = tmp.Chmod(0644)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, target)
}
