//go:build linux

package drive

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Label names the block device holding root, e.g. "sda1": the longest
// /dev mount-point prefixing the ISO directory, as read from /proc/self/mounts.
func Label(root string) string {
	file, err := os.Open("/proc/self/mounts")
	if err != nil {
		return ""
	}
	defer file.Close()
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	bestMount, bestDevice := "", ""
	for scanner := bufio.NewScanner(file); scanner.Scan(); {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		device := unescapeMount(fields[0])
		mount := unescapeMount(fields[1])
		if !strings.HasPrefix(device, "/dev/") || !containsPath(abs, mount) {
			continue
		}
		if len(mount) > len(bestMount) {
			bestMount, bestDevice = mount, device
		}
	}
	if bestDevice == "" {
		return ""
	}
	return wholeDisk(strings.TrimPrefix(bestDevice, "/dev/"))
}

// hasVtoyLabel reports whether any lsblk line carries Ventoy's VTOYEFI
// partition label in any of its fields.
func hasVtoyLabel(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		for _, field := range strings.Fields(line) {
			if strings.EqualFold(field, "VTOYEFI") {
				return true
			}
		}
	}
	return false
}

// HasVtoyEFI reports whether the disk holding root carries a partition labeled
// VTOYEFI, Ventoy's dedicated bootstrap partition. It is the identifying mark
// of a Ventoy drive that has no ventoy configuration directory yet.
func HasVtoyEFI(root string) bool {
	disk := Label(root)
	if disk == "" {
		return false
	}
	out, err := exec.Command("lsblk", "-n", "-o", "LABEL", "/dev/"+disk).Output()
	if err != nil {
		return false
	}
	return hasVtoyLabel(string(out))
}

// unescapeMount expands the octal escapes a mounts table uses for whitespace.
func unescapeMount(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				out = append(out, byte(n))
				i += 3
				continue
			}
		}
		out = append(out, s[i])
	}
	return string(out)
}

// containsPath reports whether path lives under (or exactly at) base,
// honoring component boundaries so none of /media (say) matches /med.
func containsPath(path, base string) bool {
	if base == "/" {
		return true
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}
