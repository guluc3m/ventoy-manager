package space

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Space is the capacity of, and bytes still available on, the drive holding
// the ISOs. Free counts what unprivileged writes can actually use.
type Space struct {
	Total, Free int64
}

// HumanBytes renders a byte count in binary units, e.g. "4.2 GiB".
func HumanBytes(n int64) string {
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

// Segment is one sized, colored band of a usage bar.
type Segment struct {
	Size  int64
	Color lipgloss.Color
}

// UsageBar draws one bar of fixed width from sized, colored segments laid
// out proportionally; the remaining drive space stays as a faint track.
// Cells come off cumulative boundaries, so rounding can only keep or shrink
// the filled length as segments change — marking an ISO for deletion never
// grows the bar.
func UsageBar(total int64, segments ...Segment) string {
	const width = 40
	var b strings.Builder
	if total <= 0 {
		return strings.Repeat("░", width)
	}
	filled := 0
	var cum int64
	for _, seg := range segments {
		cum += seg.Size
		end := int(cum * width / total)
		if end > width {
			end = width
		}
		if end > filled {
			b.WriteString(lipgloss.NewStyle().Foreground(seg.Color).Render(strings.Repeat("█", end-filled)))
			filled = end
		}
	}
	b.WriteString(strings.Repeat("░", width-filled))
	return b.String()
}
