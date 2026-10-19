//go:build darwin

package drive

import (
	"os/exec"
	"strings"
)

// Label names the BSD disk holding root, e.g. "disk3". It asks df for
// the device of the filesystem containing the ISO directory, drops the /dev
// prefix and the slice number, and reports the whole disk the way Ventoy's
// CLI expects it.
func Label(root string) string {
	out, err := exec.Command("df", root).Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return ""
	}
	fields := strings.Fields(lines[1])
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/dev/") {
		return ""
	}
	return wholeDisk(strings.TrimPrefix(fields[0], "/dev/"))
}
