//go:build !linux && !darwin && !windows

package drive

// Label has no platform-specific resolution on this OS; the header
// simply omits the drive name.
func Label(root string) string {
	return ""
}
