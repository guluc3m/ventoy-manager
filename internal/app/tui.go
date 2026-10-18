package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/guluc3m/ventoy-updater/internal/source"
)

type model struct {
	dir       string
	updates   []Update
	space     Space
	deleteOld bool
	cursor    int
	spinner   spinner.Model
	applying  bool
	done, err string
	// Download progress state, fed by the apply goroutine through progressCh.
	progressCh   chan downloadProgress
	fileName     string
	fileDone     int64
	fileTotal    int64 // zero while the server does not advertise a size
	finishedSize int64 // bytes of already completed downloads
	overallSize  int64 // expected bytes across all selected downloads
}
type appliedMsg struct{ err error }

// downloadProgress carries one throttled byte-count update for the current file.
type downloadProgress struct {
	fileName  string
	fileDone  int64
	fileTotal int64
}

// newModel builds the initial interactive update-selection screen.
func newModel(dir string, updates []Update, space Space) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	deleteOld := len(updates) > 0 && updates[0].DeleteOld
	return model{dir: dir, updates: updates, space: space, deleteOld: deleteOld, spinner: s}
}

// Init supplies no initial asynchronous command for the model.
func (m model) Init() tea.Cmd { return nil }

// Update handles keystrokes, progress ticks, and completion messages.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.applying {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		switch result := msg.(type) {
		case appliedMsg:
			m.applying = false
			m.progressCh = nil
			if result.err != nil {
				m.err = result.err.Error()
			} else {
				m.done = "Updates installed; ventoy.json aliases refreshed."
			}
			// Applying is the final phase: leave the TUI so the user gets their
			// shell back, and let Run print the outcome on the normal screen.
			return m, tea.Quit
		case downloadProgress:
			if result.fileName != m.fileName {
				m.finishedSize += m.fileDone
				m.fileName = result.fileName
				m.fileDone = 0
				m.fileTotal = 0
			}
			m.fileDone = result.fileDone
			m.fileTotal = result.fileTotal
			return m, listenProgress(m.progressCh)
		}
		return m, cmd
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.updates)-1 {
				m.cursor++
			}
			return m, nil
		case " ":
			i := m.cursor
			if i >= 0 && i < len(m.updates) && m.updates[i].Err == nil {
				m.updates[i].Selected = !m.updates[i].Selected
			}
			return m, nil
		case "a":
			allSelected := true
			for _, u := range m.updates {
				if u.Err == nil && !u.Selected {
					allSelected = false
					break
				}
			}
			for i := range m.updates {
				if m.updates[i].Err == nil {
					m.updates[i].Selected = !allSelected
				}
			}
			return m, nil
		case "d":
			i := m.cursor
			if i >= 0 && i < len(m.updates) && m.updates[i].Err == nil {
				m.updates[i].DeleteOld = !m.updates[i].DeleteOld
			}
			return m, nil
		case "D":
			m.deleteOld = !m.deleteOld
			for i := range m.updates {
				if m.updates[i].Err == nil {
					m.updates[i].DeleteOld = m.deleteOld
				}
			}
			return m, nil
		case "enter":
			selected := false
			for _, u := range m.updates {
				selected = selected || u.Selected
			}
			if !selected {
				return m, nil
			}
			m.applying = true
			m.startProgress()
			return m, tea.Batch(m.spinner.Tick, listenProgress(m.progressCh), m.applyCmd())
		}
	}
	return m, nil
}

// startProgress computes the expected byte total and opens the progress channel.
func (m *model) startProgress() {
	for _, u := range m.updates {
		if u.Selected && u.Err == nil {
			m.overallSize += u.LatestSize
		}
	}
	m.progressCh = make(chan downloadProgress, 64)
}

// applyCmd runs the downloads in the background, emitting throttled progress.
func (m model) applyCmd() tea.Cmd {
	ch := m.progressCh
	return func() tea.Msg {
		err := apply(m.dir, m.updates, func(name string, done, total int64) {
			select {
			case ch <- downloadProgress{name, done, total}:
			default: // the listener may already be gone once apply finishes
			}
		})
		return appliedMsg{err}
	}
}

// listenProgress yields the next queued progress event, or nil when idle.
func listenProgress(ch chan downloadProgress) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		return <-ch
	}
}

// View renders the current terminal interface.
func (m model) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("Ventoy ISO updater")
	if m.done != "" {
		return title + "\n\n" + m.done
	}
	if m.err != "" {
		return title + "\n\nUpdate failed: " + m.err
	}
	if m.applying {
		return title + "\n\n" + m.progressView()
	}
	status := ""
	if len(m.updates) > 0 {
		current := m.updates[m.cursor]
		if current.Err != nil {
			status = "Unavailable: " + current.Err.Error()
		}
	}
	globalDeletion := "off"
	if m.deleteOld {
		globalDeletion = "on"
	}
	view := fmt.Sprintf("%s\n\n%s", title, m.tableView())
	if space := m.spaceView(); space != "" {
		view += "\n\n" + space
	}
	if status != "" {
		view += "\n\n" + status
	}
	return view + "\n\n" + lipgloss.NewStyle().Faint(true).Render(strings.Join([]string{"↑/↓ select row", "space toggle update", "a toggle select all", "D delete all: " + globalDeletion, "d toggle selected deletion", "enter apply", "q quit"}, " • "))
}

// progressView renders the download screen: a byte-based overall bar across all
// selected ISOs, plus the current file with its own bar and transferred size.
func (m model) progressView() string {
	barStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("86"))
	render := func(done, total int64) string {
		const width = 28
		filled := 0
		if total > 0 {
			filled = int(done * width / total)
			if filled > width {
				filled = width
			}
		}
		return barStyle.Render(strings.Repeat("█", filled) + strings.Repeat("░", width-filled))
	}
	lines := []string{m.spinner.View() + " Downloading selected ISOs…"}
	if m.overallSize > 0 {
		done := m.finishedSize + m.fileDone
		if done > m.overallSize {
			done = m.overallSize
		}
		lines = append(lines, fmt.Sprintf("  %s %3d%%  %s / %s total", render(done, m.overallSize),
			done*100/m.overallSize, humanBytes(done), humanBytes(m.overallSize)))
	}
	if m.fileName != "" {
		sizes := humanBytes(m.fileDone)
		if m.fileTotal > 0 {
			sizes += " / " + humanBytes(m.fileTotal)
		}
		lines = append(lines, fmt.Sprintf("  %s %s  %s", render(m.fileDone, m.fileTotal), truncate(m.fileName, 36), sizes))
	}
	return strings.Join(lines, "\n")
}

// tableView renders metadata-aware columns without embedding ANSI sequences in cell data.
func (m model) tableView() string {
	headers := []string{"Update", "Distribution", "Current", "Available", "Age", "Delete"}
	widths := []int{8, 29, 13, 13, 11, 8}
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	var lines []string
	for i, header := range headers {
		lines = append(lines, headerStyle.Render(cell(header, widths[i])))
	}
	output := "  " + strings.Join(lines, "") + "\n"
	for i, u := range m.updates {
		update, deletion := "[ ]", "[ ]"
		if u.Selected {
			update = "[x]"
		}
		if u.DeleteOld {
			deletion = "[x]"
		}
		available, color := u.Latest.Image.Version, lipgloss.Color("42")
		age := ""
		if u.Err == nil {
			age = source.AgeSuffix(u.Latest.Published, time.Now())
		}
		if u.Err != nil {
			available, color = "Unavailable", lipgloss.Color("196")
		}
		prefix := "  "
		if i == m.cursor {
			prefix = "› "
		}
		fields := []string{update, u.Current.DisplayName(), u.Current.Version, available, age, deletion}
		line := prefix
		for n, field := range fields {
			styled := cell(field, widths[n])
			switch n {
			case 3:
				styled = lipgloss.NewStyle().Foreground(color).Render(styled)
			case 4:
				if field != "" {
					styled = lipgloss.NewStyle().Foreground(ageColor(u.Latest.Published)).Render(styled)
				}
			}
			line += styled
		}
		output += line + "\n"
	}
	return strings.TrimSuffix(output, "\n")
}

// spaceView renders projected drive usage after the selected updates in one
// bar: cyan is the space that stays, and the net change is red when the
// projection grows or green when it shrinks. It updates live as rows are
// toggled, and the change always takes at least one cell so MiB-scale deltas
// on large drives remain visible.
func (m model) spaceView() string {
	s := m.space
	if s.Total <= 0 {
		return ""
	}
	const (
		used  = lipgloss.Color("86")
		green = lipgloss.Color("42")
		red   = lipgloss.Color("196")
	)
	add, freed := spaceEffect(m.updates)
	kept := s.Total - s.Free
	delta := add - freed
	after := kept + delta
	if after < 0 {
		after = 0
	}
	dim := lipgloss.NewStyle().Faint(true)
	header := dim.Render(fmt.Sprintf("Drive  %s total · %s used · %s free", humanBytes(s.Total), humanBytes(kept), humanBytes(s.Free)))
	bar := usageBar(s.Total, segment{kept, used}, segment{delta, red})
	color := red
	if delta < 0 {
		color = green
		bar = usageBar(s.Total, segment{after, used}, segment{-delta, green})
	}
	line := bar + " " + humanBytes(after)
	if delta != 0 {
		line += lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("  (%s%s)", sign(delta), humanBytes(abs64(delta))))
	}
	lines := []string{header, line}
	if add > s.Free {
		lines = append(lines, lipgloss.NewStyle().Foreground(red).
			Render("⚠ Not enough free space for the downloads; deselect rows or keep the old ISOs."))
	}
	return strings.Join(lines, "\n")
}

// sign renders the sign of a byte delta.
func sign(delta int64) string {
	if delta < 0 {
		return "−"
	}
	return "+"
}

// abs64 returns the absolute value of a byte delta.
func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// ageColor shades the age column so older releases stand out more: red after
// a year, yellow after a month, white after a day, and gray for fresh releases.
func ageColor(published time.Time) lipgloss.Color {
	switch age := time.Since(published); {
	case age >= 180*24*time.Hour:
		return lipgloss.Color("196")
	case age >= 30*24*time.Hour:
		return lipgloss.Color("220")
	case age >= 24*time.Hour:
		return lipgloss.Color("255")
	default:
		return lipgloss.Color("245")
	}
}

// cell constrains a visible table value to its allotted terminal width.
func cell(value string, width int) string {
	return lipgloss.NewStyle().Align(lipgloss.Left).Width(width).MaxWidth(width).Padding(0, 1).Render(value)
}
