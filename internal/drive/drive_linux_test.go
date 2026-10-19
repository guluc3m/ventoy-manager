//go:build linux

package drive

import "testing"

// TestUnescapeMount verifies octal escapes in mounts-table names expand.
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
