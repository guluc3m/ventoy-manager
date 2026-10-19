package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/guluc3m/ventoy-updater/internal/drive"
	"github.com/guluc3m/ventoy-updater/internal/space"
)

// Drive describes one mounted Ventoy data partition this tool can update.
type Drive struct {
	Mount string // partition mount point, e.g. /run/media/user/Ventoy
	Name  string // whole-disk device name, e.g. "sda", without partition digits
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

// hasVtoyEFI is swapped out in tests to fake the partition probe below.
var hasVtoyEFI = drive.HasVtoyEFI

// DiscoverFrom inspects every first-level directory of the given base paths and
// keeps those that look like Ventoy drives: either they carry a ventoy
// configuration directory, or their disk holds Ventoy's VTOYEFI bootstrap
// partition. The ventoy.json inside is optional: drives without one can still
// be managed, they just have no aliases to refresh, and a fresh drive without
// ISOs is recognized as well.
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
				// No configuration directory: still a Ventoy when its disk
				// carries the VTOYEFI partition, e.g. a freshly installed one.
				if !hasVtoyEFI(mount) {
					continue
				}
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
		Name:  drive.Label(mount),
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
// The second result reports whether the directory belongs to a discovered
// Ventoy drive rather than the fallback path.
func selectVentoyDir(yes bool) (string, bool, error) {
	drives := Discover(os.Getenv("USER"))
	switch len(drives) {
	case 0:
		return "ventoy", false, nil
	case 1:
		return drives[0].Dir, true, nil
	}
	if yes {
		mounts := make([]string, len(drives))
		for i, d := range drives {
			mounts[i] = d.Mount
		}
		return "", false, fmt.Errorf("multiple Ventoy drives are mounted (%s); pass --ventoy-dir to choose one",
			strings.Join(mounts, ", "))
	}
	dir, err := chooseDrive(drives, os.Stdin, os.Stdout)
	if err != nil {
		return "", false, err
	}
	return dir, true, nil
}

// chooseDrive asks which of several mounted drives to update, via bubbletea:
// arrows or a number pick a drive, enter confirms, q or esc cancels.
func chooseDrive(drives []Drive, in io.Reader, out io.Writer) (string, error) {
	p := tea.NewProgram(drivePicker{drives: drives, chosen: -1},
		tea.WithInput(in), tea.WithOutput(out))
	final, err := p.Run()
	if err != nil {
		return "", fmt.Errorf("no drive selected: %w", err)
	}
	m, ok := final.(drivePicker)
	if !ok || m.chosen < 0 {
		return "", errors.New("no drive selected")
	}
	return drives[m.chosen].Dir, nil
}

// drivePicker is the bubbletea model behind the drive selection prompt.
type drivePicker struct {
	drives []Drive
	cursor int
	chosen int // index into drives once confirmed, -1 until then
}

func (m drivePicker) Init() tea.Cmd { return nil }

func (m drivePicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.drives)-1 {
			m.cursor++
		}
	case "enter", "ctrl+j":
		if m.chosen < 0 {
			m.chosen = m.cursor
		}
		return m, tea.Quit
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	default:
		if n, err := strconv.Atoi(key.String()); err == nil && n >= 1 && n <= len(m.drives) {
			if m.chosen < 0 {
				m.chosen = n - 1
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m drivePicker) View() string {
	names := make([]string, len(m.drives))
	width := 0
	for i, d := range m.drives {
		names[i] = d.Name
		if names[i] == "" {
			names[i] = d.Mount
		}
		if n := len(names[i]); n > width {
			width = n
		}
	}
	var b strings.Builder
	fmt.Fprintln(&b)
	b.WriteString(lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("86")).Render("Multiple Ventoy drives are mounted."))
	fmt.Fprintf(&b, "\n\nWhich one would you like to manage?")
	for i, d := range m.drives {
		row := fmt.Sprintf("%d) %-*s  %s total, %s free, %d ISO%s",
			i+1, width, names[i], formatSize(d.Size), formatSize(d.Free),
			d.ISOs, plural(d.ISOs))
		if i == m.cursor {
			b.WriteString("> " + lipgloss.NewStyle().Bold(true).Render(row))
		} else {
			b.WriteString("  " + row)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\n↑/↓ or a number selects, enter confirms, q cancels")
	return b.String()
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
