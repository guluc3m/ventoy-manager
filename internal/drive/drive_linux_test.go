//go:build linux

package drive

import "testing"

// TestUnescapeMount verifies octal escapes in mounts-table names expand.
// TestHasVtoyLabel picks Ventoy's bootstrap partition out of lsblk output.
func TestHasVtoyLabel(t *testing.T) {
	cases := map[string]bool{
		"Ventoy\nVTOYEFI\n":              true,
		"vtoyefi\n":                      true,
		"  sdb2  VTOYEFI\nsdb1 Ventoy\n": true,
		"Ventoy\n":                       false,
		"":                               false,
		"VENTOY-EFI\n":                   false,
		" Ventoy\nDATA\n":                false,
	}
	for out, want := range cases {
		if got := hasVtoyLabel(out); got != want {
			t.Errorf("hasVtoyLabel(%q) = %v, want %v", out, got, want)
		}
	}
}

func TestUnescapeMount(t *testing.T) {
	if got := unescapeMount(`my\040drive\134`); got != `my drive\` {
		t.Errorf("unescapeMount = %q, want %q", got, `my drive\`)
	}
}

// TestContainsPath verifies component-boundary matching so /media never
// claims /med, and that the root mount holds everything.
func TestContainsPath(t *testing.T) {
	if !containsPath("/run/media/user/Ventoy/ISOs", "/run/media/user") {
		t.Error("nested path must match its mount")
	}
	if containsPath("/media/script", "/media/space") {
		t.Error("sibling prefixes must not match")
	}
	if containsPath("/strangers", "/media") {
		t.Error("unrelated prefixes must not match")
	}
}
