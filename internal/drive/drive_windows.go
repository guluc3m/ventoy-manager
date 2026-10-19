//go:build windows

package drive

import "path/filepath"

// Label names the volume holding root, e.g. "D:".
func Label(root string) string {
	return filepath.VolumeName(root)
}
