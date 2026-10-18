package app

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
)

// Space is the capacity of, and bytes still available on, the drive holding
// the ISOs. Free counts what unprivileged writes can actually use.
type Space struct {
	Total, Free int64
}

// freeSpace stats the filesystem containing path.
func freeSpace(path string) (Space, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Space{}, err
	}
	return Space{Total: int64(st.Blocks) * int64(st.Bsize), Free: int64(st.Bavail) * int64(st.Bsize)}, nil
}

// fileSize reports the size of an existing file, or zero when it is missing.
func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

// spaceEffect totals the bytes the selected updates will download and the
// bytes their deletions will free. Rows without a known replacement size
// contribute nothing to the download total.
func spaceEffect(updates []Update) (add, freed int64) {
	for _, u := range updates {
		if u.Err != nil || !u.Selected {
			continue
		}
		add += u.LatestSize
		if u.DeleteOld {
			freed += u.CurrentSize
		}
	}
	return add, freed
}

// humanBytes renders a byte count in binary units, e.g. "4.2 GiB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value)
}

type segment struct {
	size  int64
	color lipgloss.Color
}

// usageBar draws one bar of fixed width from sized, colored segments; any
// remainder of the drive stays as a faint track. Every nonzero segment gets
// at least one cell so small changes stay visible, and oversubscribed bars
// are clipped to the width.
func usageBar(total int64, segments ...segment) string {
	const width = 40
	var b strings.Builder
	if total <= 0 {
		return strings.Repeat("░", width)
	}
	filled := 0
	for _, seg := range segments {
		cells := 0
		if seg.size > 0 {
			if cells = int(seg.size * int64(width) / total); cells == 0 {
				cells = 1
			}
		}
		if remaining := width - filled; cells > remaining {
			cells = remaining
		}
		if cells > 0 {
			b.WriteString(lipgloss.NewStyle().Foreground(seg.color).Render(strings.Repeat("█", cells)))
			filled += cells
		}
	}
	b.WriteString(strings.Repeat("░", width-filled))
	return b.String()
}
