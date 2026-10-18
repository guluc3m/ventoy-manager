package iso

import "testing"

// TestParseCanonicalName verifies ISO metadata produces a stable alias.
func TestParseCanonicalName(t *testing.T) {
	i, ok := Parse("/ISO/ubuntu-26.04.1-desktop-amd64.iso")
	if !ok || i.CanonicalName() != "Ubuntu Desktop LTS 26.04.1" {
		t.Fatalf("got %#v, %v", i, ok)
	}
}

// TestDisplayName omits version data while retaining useful distro metadata.
func TestDisplayName(t *testing.T) {
	i, _ := Parse("/ISO/linuxmint-22.3-cinnamon-64bit.iso")
	if got := i.DisplayName(); got != "Linux Mint Cinnamon" {
		t.Fatalf("got %q", got)
	}
}
