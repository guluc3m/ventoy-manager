package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/guluc3m/ventoy-updater/internal/drive"
	"github.com/guluc3m/ventoy-updater/internal/iso"
	"github.com/guluc3m/ventoy-updater/internal/source"
	"github.com/guluc3m/ventoy-updater/internal/space"
	"github.com/guluc3m/ventoy-updater/internal/ventoy"
)

// managerMode distinguishes the manager table from the download-catalog screen.
type managerMode int

const (
	modeManager managerMode = iota
	modeCatalog
)

type model struct {
	dir, root string
	space     space.Space
	rows      []Update
	cursor    int
	spinner   spinner.Model
	resolver  *source.Resolver

	// mode selects the visible screen: the ISO table or the download catalog.
	mode          managerMode
	catalog       []iso.Image
	catalogSel    map[int]bool
	catalogCursor int

	// Apply state, fed by the apply goroutine through progressCh.
	showUnrecognized bool // whether unrecognized ISO files are listed
	applying         bool
	progressCh       chan downloadProgress
	cancel           func() // stops the running download batch
	fileName         string
	fileDone         int64
	fileTotal        int64 // zero while the server does not advertise a size
	finishedSize     int64 // bytes of already completed downloads
	overallSize      int64 // expected bytes across all selected downloads
	done, err        string
	notice           string // informational note, e.g. a cancelled batch
	width            int    // terminal width, for right-aligned footer rows

	// Resolution state, fed by the check/add goroutine through resCh.
	checking bool
	resCh    chan resolveResult
}

// downloadProgress carries one throttled byte-count update for the current file.
type downloadProgress struct {
	fileName  string
	fileDone  int64
	fileTotal int64
}

// resolveResult is one upstream-latest answer keyed by its row index.
type resolveResult struct {
	index     int
	candidate source.Candidate
	err       error
}

// resolveClosed tells the model the upstream-check goroutine has finished.
type resolveClosed struct{}

// resolveJob pairs a row index with the distribution skeleton to resolve.
type resolveJob struct {
	index int
	img   iso.Image
}

type appliedMsg struct{ err error }

// newModel builds the interactive manager; every drive ISO starts listed but
// unchecked, because upstream checks happen only on request.
func newModel(dir, root string, rows []Update, capacity space.Space) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return model{dir: dir, root: root, rows: rows, space: capacity, spinner: s,
		resolver: source.New(), catalogSel: map[int]bool{}}
}

// Init supplies no initial asynchronous command for the model.
func (m model) Init() tea.Cmd { return nil }

// Update handles keystrokes, progress ticks, and completion messages.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = size.Width // remember for right-aligned footer rows
	}
	if m.applying {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		switch result := msg.(type) {
		case tea.KeyMsg:
			if result.String() == "x" || result.String() == "esc" {
				if m.cancel != nil {
					m.cancel()
				}
				return m, nil
			}
		case appliedMsg:
			m.applying = false
			m.progressCh = nil
			m.cancel = nil
			switch {
			case errors.Is(result.err, errCancelled):
				// Return to the table instead of leaving: the batch is
				// reconsiderable, only some downloads may already exist.
				m.notice = "Download cancelled."
				return m, nil
			case result.err != nil:
				m.err = result.err.Error()
			default:
				m.done = "ISOs updated."
				if ventoy.HasConfig(m.dir) {
					m.done = "ISOs updated; ventoy.json aliases refreshed."
				}
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
	if m.checking {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		switch result := msg.(type) {
		case resolveResult:
			if result.index < len(m.rows) {
				m.rows[result.index].Latest = result.candidate
				m.rows[result.index].LatestSize = result.candidate.Size
				m.rows[result.index].Checked = true
				m.rows[result.index].Err = result.err
			}
			return m, listenResolve(m.resCh)
		case resolveClosed:
			m.checking = false
			m.resCh = nil
			m.rebuildRows()
			return m, nil
		}
		if _, ok := msg.(tea.KeyMsg); !ok {
			return m, cmd
		}
		return m, nil
	}
	if m.mode == modeCatalog {
		return m.catalogUpdate(msg)
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		return m.managerKeys(key.String())
	}
	return m, nil
}

// managerKeys implements the main table: selection, keep/delete flags, the
// on-demand update check, the download screen, and applying.
func (m model) managerKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.visibleIndexes())-1 {
			m.cursor++
		}
	case " ":
		// With a pending update under the cursor, space toggles the update
		// download; without one — up to date, unchecked, failed, or an
		// unrecognized ISO — space deletes the file instead, matching d.
		row := m.cursorRow()
		if row >= 0 && selectable(m.rows[row]) {
			m.toggleCursorSelect()
		} else {
			return m.toggleCursorDelete()
		}
	case "a":
		m.toggleSelectAll()
	case "o":
		m.toggleCursorKeep()
	case "O":
		m.toggleKeepAll()
	case "d":
		return m.toggleCursorDelete()
	case "s":
		m.toggleShowUnrecognized()
	case "c":
		return m.startCheck()
	case "n":
		return m.openCatalog()
	case "enter":
		return m.startApply()
	}
	return m, nil
}

// visibleIndexes lists which row indexes the table currently shows: every
// recognized ISO, plus unrecognized files once the user reveals them.
func (m model) visibleIndexes() []int {
	var rows []int
	for i, u := range m.rows {
		if m.showUnrecognized || u.Current.Family != "" {
			rows = append(rows, i)
		}
	}
	return rows
}

// cursorRow maps the display cursor to its underlying row index, or -1 when
// nothing visible sits there.
func (m model) cursorRow() int {
	vis := m.visibleIndexes()
	if m.cursor < 0 || m.cursor >= len(vis) {
		return -1
	}
	return vis[m.cursor]
}

// positionOf reverse-maps a row index to its display position, or -1 when
// that row is hidden.
func (m model) positionOf(row int) int {
	for pos, r := range m.visibleIndexes() {
		if r == row {
			return pos
		}
	}
	return -1
}

// clampCursor keeps the display cursor parked on a visible row.
func (m *model) clampCursor() {
	vis := m.visibleIndexes()
	m.cursor = max(0, min(m.cursor, len(vis)-1))
}

// toggleShowUnrecognized reveals or hides unrecognized ISO files; the cursor
// stays with the row it currently points at when visibility allows.
func (m *model) toggleShowUnrecognized() {
	row := m.cursorRow()
	m.showUnrecognized = !m.showUnrecognized
	if pos := m.positionOf(row); pos >= 0 {
		m.cursor = pos
	} else if row < 0 {
		m.clampCursor()
	}
}

// selectable reports whether a row's update can be toggled at all.
func selectable(u Update) bool {
	if u.Err != nil || u.Latest.Image.Filename == "" {
		return false
	}
	if u.New {
		return true
	}
	if u.Current.Family == "" {
		return false // unrecognized ISOs cannot be matched upstream
	}
	return u.Latest.Image.Filename != u.Current.Filename // already up to date?
}

// toggleCursorSelect flips the highlighted row's update selection; selecting
// an update clears its delete mark so the two stay mutually exclusive.
func (m *model) toggleCursorSelect() {
	row := m.cursorRow()
	if row < 0 {
		return
	}
	u := m.rows[row]
	if !selectable(u) {
		return
	}
	m.rows[row].Selected = !u.Selected
	m.rows[row].DeleteISO = false
}

// toggleSelectAll is the one-keystroke update-all path: every simultaneously
// selectable row — pending updates and queued catalog downloads alike — is
// selected (all-selected runs deselect everything again). Applying still
// waits for enter on purpose, so the batch stays reviewable.
func (m *model) toggleSelectAll() {
	all := true
	for _, u := range m.rows {
		if selectable(u) && !u.Selected {
			all = false
			break
		}
	}
	for i, u := range m.rows {
		if selectable(u) {
			selecting := !all
			m.rows[i].Selected = selecting
			if selecting {
				m.rows[i].DeleteISO = false
			}
		}
	}
}

// toggleKeepAll flips the keep flag on every row with a pending update at
// once: if those are all kept it unkeeps all, otherwise it keeps all.
func (m *model) toggleKeepAll() {
	anyPending, allKept := false, true
	for _, u := range m.rows {
		if u.pendingUpdate() {
			anyPending = true
			if !u.KeepOld {
				allKept = false
				break
			}
		}
	}
	if !anyPending {
		return
	}
	keep := !allKept
	for i := range m.rows {
		if m.rows[i].pendingUpdate() {
			m.rows[i].KeepOld = keep
		}
	}
}

// toggleCursorKeep flips the highlighted row's keep-old flag; rows without a
// pending update have nothing to supplant, so they cannot be kept or lost.
func (m *model) toggleCursorKeep() {
	row := m.cursorRow()
	if row < 0 {
		return
	}
	if m.rows[row].pendingUpdate() {
		m.rows[row].KeepOld = !m.rows[row].KeepOld
	}
}

// toggleCursorDelete marks the highlighted ISO for deletion without
// replacement — recognized or unrecognized alike — or drops a catalog row
// that has nothing to delete yet.
func (m model) toggleCursorDelete() (tea.Model, tea.Cmd) {
	row := m.cursorRow()
	if row < 0 {
		return m, nil
	}
	u := m.rows[row]
	if u.New {
		m.rows = append(m.rows[:row], m.rows[row+1:]...)
		m.clampCursor()
		return m, nil
	}
	m.rows[row].DeleteISO = !u.DeleteISO
	m.rows[row].Selected = false
	return m, nil
}

// startCheck resolves the latest candidate for every installed row through the
// source resolver, showing a spinner until the whole batch returns.
func (m model) startCheck() (tea.Model, tea.Cmd) {
	if m.checking || m.applying {
		return m, nil
	}
	m.notice = ""
	var jobs []resolveJob
	for i, u := range m.rows {
		if u.Current.Family != "" && !u.Checked {
			jobs = append(jobs, resolveJob{i, u.Current})
		}
	}
	if len(jobs) == 0 {
		return m, nil
	}
	m.checking = true
	m.resCh = make(chan resolveResult)
	cmds := []tea.Cmd{m.spinner.Tick, listenResolve(m.resCh), m.resolveJobsCmd(jobs)}
	return m, tea.Batch(cmds...)
}

// openCatalog shows the built-in distribution catalog for new downloads,
// skipping every distribution the manager already lists — installed on the
// drive or queued for download. The catalog is the same one the source
// resolver and ISO parser use, so new distributions need only one entry.
func (m model) openCatalog() (tea.Model, tea.Cmd) {
	if m.checking || m.applying {
		return m, nil
	}
	listed := map[string]bool{}
	for _, u := range m.rows {
		if u.Current.Family != "" && !u.DeleteISO {
			listed[flavorKey(u.Current)] = true
		}
	}
	var visible []iso.Image
	for _, entry := range iso.Catalog() {
		image := entry.Image()
		if !listed[flavorKey(image)] {
			visible = append(visible, image)
		}
	}
	m.catalog = visible
	m.mode = modeCatalog
	m.catalogCursor = 0
	m.catalogSel = map[int]bool{}
	return m, nil
}

// flavorKey matches catalog entries against listed rows on identity alone.
func flavorKey(i iso.Image) string {
	return i.Family + "|" + i.Edition + "|" + i.Flavor
}

// startApply runs the selected downloads and deletions in the background.
func (m model) startApply() (tea.Model, tea.Cmd) {
	anything := false
	for _, u := range m.rows {
		if u.DeleteISO || selectable(u) && u.Selected {
			anything = true
			break
		}
	}
	if !anything {
		return m, nil
	}
	m.notice = "" // a previous cancellation no longer applies
	m.cancel = nil
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.applying = true
	m.startProgress()
	return m, tea.Batch(m.spinner.Tick, listenProgress(m.progressCh), m.applyCmd(ctx))
}

// openCatalogRows copies the catalog's marked entries into the row table and
// resolves each one before the download really starts.
func (m model) confirmCatalog() (tea.Model, tea.Cmd) {
	var jobs []resolveJob
	for n, entry := range m.catalog {
		if !m.catalogSel[n] {
			continue
		}
		m.rows = append(m.rows, Update{Current: entry, New: true})
		jobs = append(jobs, resolveJob{len(m.rows) - 1, entry})
	}
	m.mode = modeManager
	if len(jobs) == 0 {
		return m, nil
	}
	m.checking = true
	m.resCh = make(chan resolveResult)
	return m, tea.Batch(m.spinner.Tick, listenResolve(m.resCh), m.resolveJobsCmd(jobs))
}

// catalogUpdate handles keys in the download-catalog screen.
func (m model) catalogUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "esc", "ctrl+c":
		m.mode = modeManager
	case "up":
		if m.catalogCursor > 0 {
			m.catalogCursor--
		}
	case "down":
		if m.catalogCursor < len(m.catalog)-1 {
			m.catalogCursor++
		}
	case " ", "x":
		n := m.catalogCursor
		if n < len(m.catalog) {
			m.catalogSel[n] = !m.catalogSel[n]
		}
	case "a":
		all := true
		for n := range m.catalog {
			if !m.catalogSel[n] {
				all = false
				break
			}
		}
		for n := range m.catalog {
			m.catalogSel[n] = !all
		}
	case "enter":
		return m.confirmCatalog()
	}
	return m, nil
}

// checkedOrder implements the table ordering after an update check: ISOs
// with pending updates lead the table — freshest pending release first —
// then the remainder by release age (dated before undated), unrecognized
// files always trailing.
func checkedOrder(a, b Update) bool {
	if a.pendingUpdate() != b.pendingUpdate() {
		return a.pendingUpdate()
	}
	aUnrec, bUnrec := a.Current.Family == "", b.Current.Family == ""
	if aUnrec != bUnrec {
		return bUnrec // recognized before unrecognized inside a group
	}
	aPub, bPub := a.Latest.Published, b.Latest.Published
	if !aPub.Equal(bPub) {
		if aPub.IsZero() {
			return false // undated rows sink
		}
		if bPub.IsZero() {
			return true
		}
		return aPub.After(bPub) // newer releases first
	}
	return recognizedSorting(a, b)
}

// rebuildRows re-sorts after the check batch and removes catalog rows whose
// candidate is indistinct from what the drive already holds.
func (m *model) rebuildRows() {
	kept := make([]Update, 0, len(m.rows))
	installed := map[string]bool{}
	for _, u := range m.rows {
		if !u.New {
			installed[u.Current.Filename] = true
		}
	}
	for _, u := range m.rows {
		if u.New {
			name := u.Latest.Image.Filename
			if u.Err == nil && (name == "" || installed[name]) {
				continue
			}
			installed[name] = true
		}
		kept = append(kept, u)
	}
	m.rows = kept
	// After a check the table re-groups (pending updates lead, unrecognized
	// files trail), so a cursor resting on a recognized ISO snaps back to
	// the top row for a fresh read-through; unrecognized rows keep the
	// attach-to-row behaviour.
	row := m.cursorRow()
	recognized := row >= 0 && m.rows[row].Current.Family != ""
	sort.SliceStable(m.rows, func(i, j int) bool { return checkedOrder(m.rows[i], m.rows[j]) })
	if recognized {
		m.cursor = 0
		return
	}
	if row >= 0 {
		if pos := m.positionOf(row); pos >= 0 {
			m.cursor = pos
			return
		}
	}
	m.clampCursor()
}

// resolveJobsCmd runs each resolution sequentially, streaming results through
// resCh, and closes it once everything is checked.
func (m model) resolveJobsCmd(jobs []resolveJob) tea.Cmd {
	ch := m.resCh
	resolver := m.resolver
	return func() tea.Msg {
		for _, job := range jobs {
			candidate, err := resolver.Latest(context.Background(), job.img)
			ch <- resolveResult{job.index, candidate, err}
		}
		close(ch)
		return resolveClosed{}
	}
}

// listenResolve yields the next queued resolution event, or resolveClosed once
// the channel is shut down.
func listenResolve(ch chan resolveResult) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		event, open := <-ch
		if !open {
			return resolveClosed{}
		}
		return event
	}
}

// startProgress computes the expected byte total and opens the progress channel.
func (m *model) startProgress() {
	for _, u := range m.rows {
		if u.Selected && u.Err == nil && u.Latest.Image.Filename != "" {
			m.overallSize += u.LatestSize
		}
	}
	m.progressCh = make(chan downloadProgress, 64)
}

// applyCmd runs the downloads in the background, emitting throttled progress;
// the context lets the user abort the batch from the progress screen.
func (m model) applyCmd(ctx context.Context) tea.Cmd {
	ch := m.progressCh
	cancel := m.cancel
	return func() tea.Msg {
		err := apply(ctx, m.dir, m.root, m.rows, func(name string, done, total int64) {
			select {
			case ch <- downloadProgress{name, done, total}:
			default: // the listener may already be gone once apply finishes
			}
		})
		cancel()
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
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("Ventoy ISO manager")
	switch {
	case m.done != "":
		return title + "\n\n" + m.done
	case m.err != "":
		return title + "\n\nApply failed: " + m.err
	case m.applying:
		return title + "\n\n" + m.progressView()
	case m.mode == modeCatalog:
		return title + "\n\n" + m.catalogView()
	}
	lines := []string{title, ""}
	if m.checking {
		lines = append(lines, m.spinner.View()+" Checking upstream releases…", "")
	}
	// Drive state first: the drive, its usage bar, and the warning lead the
	// table so the space context is visible before scanning the ISOs.
	if space := m.spaceView(); space != "" {
		lines = append(lines, space, "")
	}
	lines = append(lines, m.tableView())
	lines = append(lines, "", m.selectedView())
	if m.notice != "" {
		lines = append(lines, lipgloss.NewStyle().Faint(true).Render(m.notice))
	}
	if m.err != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(m.err))
	}
	return strings.Join(lines, "\n") + "\n\n" + m.hints()
}

// hints builds the three-line footer: contextual, selection-scoped actions
// on the first line, global view commands below, and the confirm/exit row
// last (apply in green when anything is pending, quit in red). Entries
// appear only when their key can actually do something in the current
// state; an unrun update check is highlighted in yellow.
func (m model) hints() string {
	faint := lipgloss.NewStyle().Faint(true)
	line := func(entries []string) string {
		styled := make([]string, 0, len(entries))
		for _, entry := range entries {
			if strings.Contains(entry, "\x1b") { // already colored
				styled = append(styled, entry)
				continue
			}
			styled = append(styled, faint.Render(entry))
		}
		return strings.Join(styled, " • ")
	}
	footer := func(lines ...[]string) string {
		var rendered []string
		for _, entries := range lines {
			if s := line(entries); s != "" {
				rendered = append(rendered, s)
			}
		}
		// The confirm/exit row hugs the right edge of the terminal.
		if n := len(rendered); n > 0 && m.width > 0 {
			rendered[n-1] = lipgloss.NewStyle().Width(m.width).
				Align(lipgloss.Right).Render(rendered[n-1])
		}
		return strings.Join(rendered, "\n")
	}

	if m.mode == modeCatalog {
		anySelectable, anyMarked := false, false
		for _, u := range m.rows {
			if selectable(u) {
				anySelectable = true
			}
			if u.Selected {
				anyMarked = true
			}
		}
		var first, second []string
		if len(m.rows) > 0 {
			first = append(first, "↑/↓ select")
			if anySelectable {
				first = append(first, "space mark", "a all marks")
			}
		}
		second = append(second, "esc back")
		var third []string
		if anyMarked {
			third = append(third, lipgloss.NewStyle().
				Foreground(lipgloss.Color("42")).Render("enter download selection"))
		}
		return footer(first, second, third)
	}

	var selected, pending, deletable, catalogRows, checked, unrecognized, applied int
	for _, u := range m.rows {
		if u.Checked {
			checked++
		}
		if u.Current.Family == "" {
			unrecognized++
		}
		if selectable(u) {
			selected++
		}
		if u.pendingUpdate() {
			pending++
		}
		if u.Selected || u.DeleteISO {
			applied++
		}
		if u.New {
			catalogRows++
		} else {
			deletable++ // installed or unrecognized files can always go
		}
	}

	var first, second, third []string
	if len(m.rows) > 0 {
		first = append(first, "↑/↓ selection")
	}
	// The center hint follows the row under the cursor: rows without an
	// available update — and unrecognized ISOs — cannot download anything,
	// so space deletes them instead (with d) and the update hints stay
	// silent; the delete hint then comes from this branch alone.
	cursorHintsDelete := false
	if cursor := m.cursorRow(); cursor >= 0 {
		u := m.rows[cursor]
		if selectable(u) {
			first = append(first, "space update")
		} else {
			first = append(first, "space/d delete ISO")
			cursorHintsDelete = true
		}
		if u.pendingUpdate() {
			first = append(first, "o keep old version")
		}
	}
	if selected > 0 { // global marks stay announced from anywhere
		first = append(first, "a update all")
	}
	if pending > 0 {
		first = append(first, "O keep all old")
	}
	if !cursorHintsDelete && (deletable > 0 || catalogRows > 0) {
		first = append(first, "d delete ISO")
	}

	if !m.checking { // the check is already running; its spinner says so
		if checked == 0 {
			second = append(second, lipgloss.NewStyle().
				Foreground(lipgloss.Color("220")).Render("c check updates"))
		} else {
			second = append(second, "c check updates")
		}
	}
	second = append(second, "n new ISO")
	if unrecognized > 0 {
		state := "hidden"
		if m.showUnrecognized {
			state = "shown"
		}
		second = append(second, "s unrecognized: "+state)
	}
	if applied > 0 {
		third = append(third, lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")).Render("enter apply"))
	}
	third = append(third, lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).Render("q quit"))
	return footer(first, second, third)
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
			done*100/m.overallSize, space.HumanBytes(done), space.HumanBytes(m.overallSize)))
	}
	if m.fileName != "" {
		sizes := space.HumanBytes(m.fileDone)
		if m.fileTotal > 0 {
			sizes += " / " + space.HumanBytes(m.fileTotal)
		}
		lines = append(lines, fmt.Sprintf("  %s %s  %s", render(m.fileDone, m.fileTotal), truncate(m.fileName, 36), sizes))
	}
	dim := lipgloss.NewStyle().Faint(true)
	lines = append(lines, "", dim.Render("  x or esc — cancel the batch"))
	return strings.Join(lines, "\n")
}

var (
	headers = []string{"Update", "ISO", "Version", "Available", "Age", "Keep", "Delete"}
	widths  = []int{8, 34, 12, 14, 11, 7, 8}
)

// tableView renders the manager table; unrecognized ISOs stay hidden behind
// a count line until the user reveals them.
func (m model) tableView() string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	var lines []string
	for i, header := range headers {
		lines = append(lines, headerStyle.Render(cell(header, widths[i])))
	}
	dim := lipgloss.NewStyle().Faint(true)
	output := "  " + strings.Join(lines, "") + "\n"
	vis := m.visibleIndexes()
	prevFamily := ""
	for _, row := range vis {
		u := m.rows[row]
		if u.Current.Family == "" && prevFamily != "" {
			output += dim.Render("   ── Unrecognized ISOs ──") + "\n"
		}
		prevFamily = u.Current.Family
		output += m.rowView(m.positionOf(row), u) + "\n"
	}
	if hidden := len(m.rows) - len(vis); hidden > 0 && !m.showUnrecognized {
		output += dim.Render(fmt.Sprintf("  (+%d unrecognized ISO%s)", hidden, plural(hidden)))
	}
	return strings.TrimSuffix(output, "\n")
}

// rowView renders one table row for the given update.
func (m model) rowView(index int, u Update) string {
	unrecognized := u.Current.Family == ""
	update, deleteMark := "[ ]", "[ ]"
	if u.Selected {
		update = "[x]"
	}
	keep := " —" // only rows with a pending update can keep or lose an old ISO
	if u.pendingUpdate() {
		keep = "[x]"
		if !u.KeepOld {
			keep = "[ ]"
		}
	}
	if u.DeleteISO {
		deleteMark = "[x]"
	}
	if !selectable(u) {
		// No update can be toggled here — unchecked, up to date, failed, or
		// unrecognized — so the checkbox gives way to a grayed dash, offset
		// with a leading space, aligned with the boxes above.
		update = " -"
	}
	if unrecognized {
		// There is no checkbox to toggle: a grayed dash, offset with a
		// leading space, keeps its column aligned with the boxes above.
		update, keep = " -", " -"
	}
	version := u.Current.Version
	if version == "" {
		version = "—"
	}
	// Recognized but unchecked rows carry a "?" until the user requests the
	// upstream check; unrecognized files stay out of reach with a dash.
	available, color := "—", lipgloss.Color("245")
	if !u.Checked && u.Current.Family != "" {
		available = "?"
	}
	age := ""
	if u.Checked {
		if u.Err == nil {
			if u.Latest.Image.Filename != "" && u.Latest.Image.Filename != u.Current.Filename {
				available, color = u.Latest.Image.Version, lipgloss.Color("42")
				age = source.AgeSuffix(u.Latest.Published, time.Now())
			} else {
				// Still show how old the installed release is, in gray.
				available, color = "up to date", lipgloss.Color("245")
				age = source.AgeSuffix(u.Latest.Published, time.Now())
			}
		} else {
			available, color = "Unavailable", lipgloss.Color("196")
		}
	}
	// A two-rune gutter shared by every row: the cursor indicator plus the
	// new-row tag — or a filler space — so values align with the header.
	indicator := " "
	if index == m.cursor {
		indicator = "›"
	}
	tag := " "
	if u.New {
		tag = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("✚")
	}
	fields := []string{update, rowLabel(u), version, available, age, keep, deleteMark}
	line := indicator + tag
	dim := lipgloss.NewStyle().Faint(true)
	for n, field := range fields {
		// cell() also pads one space per side, so cap the visible text there.
		const padding = 2
		if runes := len([]rune(field)); runes > widths[n]-padding {
			field = truncate(field, widths[n]-padding)
		}
		styled := cell(field, widths[n])
		switch n {
		case 1:
			if unrecognized && index != m.cursor {
				styled = dim.Render(styled)
			}
		case 3:
			styled = lipgloss.NewStyle().Foreground(color).Render(styled)
		case 4:
			if field != "" {
				styled = lipgloss.NewStyle().Foreground(u.ageShade()).Render(styled)
			}
		case 0, 5:
			if field == " -" { // grayed, non-togglable placeholders
				styled = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(styled)
			}
		}
		line += styled
	}
	return line
}

// flavorLabel renders a versionless distribution name with the flavor in
// parentheses, e.g. "Ubuntu Desktop LTS" or "Linux Mint (Cinnamon)".
func flavorLabel(i iso.Image) string {
	name := i.Family
	if i.Edition != "" {
		name += " " + i.Edition
	}
	if i.Flavor != "" {
		name += " (" + i.Flavor + ")"
	}
	return name
}

// rowLabel is the visible ISO identity: the versionless canonical name with
// the flavor in parentheses, or the raw filename for unrecognized files. The
// version has its own column.
func rowLabel(u Update) string {
	if u.Current.Family == "" {
		return u.Current.Filename
	}
	return flavorLabel(u.Current)
}

// selectedView describes the highlighted row: its size on disk plus, once
// checked, what upstream offers instead.
func (m model) selectedView() string {
	if len(m.rows) == 0 {
		return "This drive has no ISOs. Press n to download one."
	}
	row := m.cursorRow()
	if row < 0 {
		return ""
	}
	u := m.rows[row]
	parts := []string{rowLabel(u)}
	switch {
	case u.New:
		if u.Checked && u.Err == nil {
			parts = append(parts, "new download", fmt.Sprintf("%s to add", space.HumanBytes(u.LatestSize)))
		} else {
			parts = append(parts, "resolving…")
		}
	case u.Current.Family == "":
		parts = append(parts, fmt.Sprintf("%s on disk", space.HumanBytes(u.CurrentSize)), "unrecognized, never updated")
	case !u.Checked:
		parts = append(parts, fmt.Sprintf("%s on disk", space.HumanBytes(u.CurrentSize)), "not checked — press c")
	default:
		parts = append(parts, fmt.Sprintf("%s on disk", space.HumanBytes(u.CurrentSize)))
		if u.Err != nil {
			parts = append(parts, "upstream unavailable: "+u.Err.Error())
		} else if u.Latest.Image.Filename == u.Current.Filename {
			parts = append(parts, "up to date")
		} else {
			detail := fmt.Sprintf("%s %s available", u.Latest.Image.Version, space.HumanBytes(u.LatestSize))
			if age := source.AgeSuffix(u.Latest.Published, time.Now()); age != "" {
				detail += " (" + age + ")"
			}
			parts = append(parts, detail)
		}
	}
	return strings.Join(parts, " · ")
}

// catalogView renders the download-catalog multiselect list; distributions
// the list already holds are filtered out at openCatalog time, so everything
// shown is something the drive does not have.
func (m model) catalogView() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).
		Render("Newest releases available for download")
	if len(m.catalog) == 0 {
		return header + "\n\n  Everything the catalog offers is already listed."
	}
	var lines []string
	for i, entry := range m.catalog {
		mark := "[ ]"
		if m.catalogSel[i] {
			mark = "[x]"
		}
		prefix := "  "
		if i == m.catalogCursor {
			prefix = "› "
		}
		lines = append(lines, fmt.Sprintf("%s%s  %s", prefix, mark, flavorLabel(entry)))
	}
	return header + "\n\n" + strings.Join(lines, "\n")
}

// spaceView renders projected drive usage after the selected downloads,
// deletions, and kept old ISOs in one bar: cyan is the space that stays, and
// the net change is red when the projection grows or green when it shrinks. It
// updates live as rows are toggled, and the change always takes at least one
// cell so MiB-scale deltas on large drives remain visible.
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
	add, freed := spaceEffect(m.rows)
	kept := s.Total - s.Free
	delta := add - freed
	after := kept + delta
	if after < 0 {
		after = 0
	}
	dim := lipgloss.NewStyle().Faint(true)
	name := "Drive"
	if label := drive.Label(m.root); label != "" {
		name += " (" + label + ")"
	}
	header := dim.Render(fmt.Sprintf("%s  %s total · %s used · %s free", name, space.HumanBytes(s.Total), space.HumanBytes(kept), space.HumanBytes(s.Free)))
	bar := space.UsageBar(s.Total,
		space.Segment{Size: kept, Color: used}, space.Segment{Size: delta, Color: red})
	color := red
	if delta < 0 {
		// The freed bytes stay visible as a green segment covering the
		// area the usage segment gives up.
		color = green
		bar = space.UsageBar(s.Total,
			space.Segment{Size: after, Color: used}, space.Segment{Size: -delta, Color: green})
	}
	line := bar + " " + space.HumanBytes(after)
	if delta != 0 {
		line += lipgloss.NewStyle().Foreground(color).Render(fmt.Sprintf("  (%s%s)", sign(delta), space.HumanBytes(abs64(delta))))
	}
	lines := []string{header, line}
	if add > s.Free {
		lines = append(lines, lipgloss.NewStyle().Foreground(red).
			Render("⚠ Not enough free space for the downloads; deselect rows or delete ISOs to clear space."))
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

// pendingUpdate reports whether this checked row offers a newer release that
// would supersede the installed ISO. Catalog rows are never "pending" — they
// have no old copy to keep.
func (u Update) pendingUpdate() bool {
	return !u.New && u.Checked && u.Err == nil && u.Latest.Image.Filename != "" &&
		u.Latest.Image.Filename != u.Current.Filename
}

// ageShade picks the Age color: pending releases grade white → yellow → red
// with age, while anything else on the table is a steady gray.
func (u Update) ageShade() lipgloss.Color {
	if !u.pendingUpdate() {
		return lipgloss.Color("245") // up-to-date ages are always gray
	}
	return ageColor(u.Latest.Published)
}

// ageColor shades ages of pending releases so older ones stand out more:
// red after half a year, yellow after a month, white for anything younger.
func ageColor(published time.Time) lipgloss.Color {
	switch age := time.Since(published); {
	case age >= 180*24*time.Hour:
		return lipgloss.Color("196")
	case age >= 30*24*time.Hour:
		return lipgloss.Color("220")
	default:
		return lipgloss.Color("255")
	}
}

// truncate shortens a filename so a progress line never wraps.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// cell constrains a visible table value to its allotted terminal width.
func cell(value string, width int) string {
	return lipgloss.NewStyle().Align(lipgloss.Left).Width(width).MaxWidth(width).Padding(0, 1).Render(value)
}
