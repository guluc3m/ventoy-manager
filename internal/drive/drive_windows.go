//go:build windows

package drive

import "path/filepath"

// Label names the volume holding root, e.g. "D:".
func Label(root string) string {
	return filepath.VolumeName(root)
}

// HasVtoyEFI cannot probe sibling partitions without shelling out to
// PowerShell; drives are recognized only by their ventoy configuration
// directory on this OS.
func HasVtoyEFI(root string) bool {
	return false
}
