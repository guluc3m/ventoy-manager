package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/guluc3m/ventoy-updater/internal/space"
)

// Drive describes one mounted Ventoy data partition this tool can update.
type Drive struct {
	Mount string // partition mount point, e.g. /run/media/user/Ventoy
	Dir   string // ventoy configuration directory on that partition
	Size  uint64 // partition size in bytes
	Free  uint64 // available space in bytes
	ISOs  int    // number of ISO files anywhere below the mount point
}

// Discover scans the conventional udisks mount locations for Ventoy drives.
func Discover(user string) []Drive {
	var bases []string
	if user != "" {
		bases = []string{filepath.Join("/run/media", user), filepath.Join("/media", user)}
	}
	return DiscoverFrom(bases)
}

// DiscoverFrom inspects every first-level directory of the given base paths and
// keeps those holding a ventoy configuration directory. The ventoy.json inside
// is optional: drives without one can still be managed, they just have no
// aliases to refresh.
func DiscoverFrom(bases []string) []Drive {
	var drives []Drive
	for _, base := range bases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			mount := filepath.Join(base, entry.Name())
			dir := filepath.Join(mount, "ventoy")
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				continue
			}
			d, err := describeDrive(mount, dir)
			if err != nil {
				continue
			}
			drives = append(drives, d)
		}
	}
	sort.Slice(drives, func(i, j int) bool { return drives[i].Mount < drives[j].Mount })
	return drives
}

// describeDrive fills in capacity, free space and ISO counts for one drive.
func describeDrive(mount, dir string) (Drive, error) {
	s, err := space.Free(mount)
	if err != nil {
		return Drive{}, err
	}
	return Drive{
		Mount: mount,
		Dir:   dir,
		Size:  uint64(s.Total),
		Free:  uint64(s.Free),
		ISOs:  countISOs(mount),
	}, nil
}

// countISOs walks root and counts files with an .iso extension, any case.
func countISOs(root string) int {
	count := 0
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".iso") {
			count++
		}
		return nil
	})
	return count
}

// selectVentoyDir resolves the drive to update when --ventoy-dir was omitted:
// the single mounted drive is used automatically, several trigger a prompt, and
// none falls back to a relative ventoy directory next to the working directory.
func selectVentoyDir(yes bool) (string, error) {
	drives := Discover(os.Getenv("USER"))
	switch len(drives) {
	case 0:
		return "ventoy", nil
	case 1:
		return drives[0].Dir, nil
	}
	if yes {
		mounts := make([]string, len(drives))
		for i, d := range drives {
			mounts[i] = d.Mount
		}
		return "", fmt.Errorf("multiple Ventoy drives are mounted (%s); pass --ventoy-dir to choose one",
			strings.Join(mounts, ", "))
	}
	return chooseDrive(drives, os.Stdin, os.Stdout)
}

// chooseDrive asks the user which of several mounted drives to update.
func chooseDrive(drives []Drive, in io.Reader, out io.Writer) (string, error) {
	mountWidth := 0
	for _, d := range drives {
		if n := len(d.Mount); n > mountWidth {
			mountWidth = n
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("86")).Render("Multiple Ventoy drives are mounted."))
	fmt.Fprintf(out, "\nWhich one should be updated? [%d-%d, enter for 1]\n\n", 1, len(drives))
	for i, d := range drives {
		fmt.Fprintf(out, "  %d) %-*s  %s total, %s free, %d ISO%s\n",
			i+1, mountWidth, d.Mount, formatSize(d.Size), formatSize(d.Free),
			d.ISOs, plural(d.ISOs))
	}
	reader := bufio.NewReader(in)
	for {
		fmt.Fprint(out, "\n> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("no drive selected: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return drives[0].Dir, nil
		}
		var n int
		if _, err := fmt.Sscanf(line, "%d", &n); err != nil || n < 1 || n > len(drives) {
			fmt.Fprintf(out, "Please enter a number between 1 and %d.\n", len(drives))
			continue
		}
		return drives[n-1].Dir, nil
	}
}

// formatSize renders a byte count in megabytes or gigabytes.
func formatSize(b uint64) string {
	const gib = 1 << 30
	gb := float64(b) / gib
	switch {
	case b >= 10*gib:
		return fmt.Sprintf("%.0f GB", gb)
	case b >= gib:
		return fmt.Sprintf("%.1f GB", gb)
	default:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	}
}

// plural returns an s for counts other than exactly one.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
