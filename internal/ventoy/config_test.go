package ventoy

import (
	"encoding/json"
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

// TestSearchRoot verifies the configured Ventoy ISO root is read correctly.
func TestSearchRoot(t *testing.T) {
	c := &Config{raw: map[string]json.RawMessage{"control": json.RawMessage(`[{"VTOY_DEFAULT_SEARCH_ROOT":"/images/linux"}]`)}}
	if got := c.SearchRoot(); got != "/images/linux" {
		t.Fatalf("got %q", got)
	}
}
