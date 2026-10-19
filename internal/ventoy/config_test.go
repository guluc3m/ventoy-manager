package ventoy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSetAliasForPath verifies matching aliases receive the canonical name.
func TestSetAliasForPath(t *testing.T) {
	c := &Config{MenuAlias: []Alias{{Image: "/ISO/ubuntu-26.04.*-desktop-amd64.iso", Alias: "old"}}}
	if !c.SetAliasForPath("ISO/ubuntu-26.04.1-desktop-amd64.iso", "Ubuntu Desktop LTS 26.04.1") {
		t.Fatal("not updated")
	}
	if got := c.MenuAlias[0].Alias; got != "Ubuntu Desktop LTS 26.04.1" {
		t.Fatal(got)
	}
}

// TestLoadWithoutConfig verifies a drive missing ventoy.json loads as an empty
// configuration instead of failing.
func TestLoadWithoutConfig(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Exists() {
		t.Error("Exists = true without a ventoy.json")
	}
	if got := c.SearchRoot(); got != "/" {
		t.Errorf("SearchRoot = %q, want / for a drive without config", got)
	}
	if HasConfig(dir) {
		t.Error("HasConfig = true without a ventoy.json")
	}
	if _, err := os.Stat(filepath.Join(dir, "ventoy.json")); !os.IsNotExist(err) {
		t.Errorf("Load created a ventoy.json (stat err %v)", err)
	}
}

// TestLoadRejectsMissingDirectory keeps explicit --ventoy-dir typos from
// silently falling back to the partition root.
func TestLoadRejectsMissingDirectory(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("Load should fail when the ventoy directory does not exist")
	}
}

// TestSearchRoot verifies the configured Ventoy ISO root is read correctly.
func TestSearchRoot(t *testing.T) {
	c := &Config{raw: map[string]json.RawMessage{"control": json.RawMessage(`[{"VTOY_DEFAULT_SEARCH_ROOT":"/images/linux"}]`)}}
	if got := c.SearchRoot(); got != "/images/linux" {
		t.Fatalf("got %q", got)
	}
}
