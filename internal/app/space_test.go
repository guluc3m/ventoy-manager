package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/guluc3m/ventoy-updater/internal/iso"
	"github.com/guluc3m/ventoy-updater/internal/source"
	"github.com/guluc3m/ventoy-updater/internal/space"
)

// TestSpaceEffect verifies the projected-downloads and free-up-space halves
// of the drive bar: selected rows add bytes, delete-marked and superseded
// rows free them, and kept old ISOs free nothing.
func TestSpaceEffect(t *testing.T) {
	candidate := source.Candidate{Image: iso.Image{Family: "X", Filename: "x.iso"}, Size: 6 << 30}
	updates := []Update{
		{CurrentSize: 5 << 30, Latest: candidate, LatestSize: 6 << 30, Selected: true},
		{CurrentSize: 3 << 30, Latest: candidate, LatestSize: 4 << 30, Selected: true, KeepOld: true},
		{CurrentSize: 9 << 30, Latest: candidate, LatestSize: 9 << 30},
		{CurrentSize: 2 << 30, DeleteISO: true},
		{Err: errTest, Selected: true, Latest: candidate, LatestSize: 7 << 30},
	}
	add, freed := spaceEffect(updates)
	if add != 10<<30 {
		t.Errorf("add = %d, want %d", add, int64(10<<30))
	}
	if freed != 7<<30 {
		t.Errorf("freed = %d, want %d", freed, int64(7<<30))
	}
}

// TestSelectableGuardsRows verifies that unrecognized, up-to-date, and queued
// catalog rows never offer an update toggle.
func TestSelectable(t *testing.T) {
	updateRow := Update{Current: iso.Image{Family: "Ubuntu", Filename: "ubuntu-24.04.iso"}, Latest: source.Candidate{Image: iso.Image{Filename: "ubuntu-26.04.iso"}}}
	upToDate := updateRow
	upToDate.Latest.Image.Filename = "ubuntu-24.04.iso"
	unrecognized := Update{Current: iso.Image{Filename: "backup.iso"}}
	if !selectable(updateRow) {
		t.Error("genuine update should be selectable")
	}
	if selectable(upToDate) {
		t.Error("up-to-date row should not be selectable")
	}
	if selectable(unrecognized) {
		t.Error("unrecognized ISO should not be selectable")
	}
	if selectable(Update{New: true, Latest: source.Candidate{Image: iso.Image{Filename: "tails.iso"}}}) {
		t.Error("queued catalog addition should not offer an update toggle")
	}
}

// TestToggleKeepAll verifies the global O toggle flips every row with a
// pending update so they share a keep state, and never touches other rows.
func TestToggleKeepAll(t *testing.T) {
	m := model{rows: []Update{
		{Current: iso.Image{Family: "Ubuntu", Filename: "a.iso"}, KeepOld: false,
			Checked: true, Latest: source.Candidate{Image: iso.Image{Filename: "a-new.iso"}}},
		{Current: iso.Image{Family: "Debian", Filename: "b.iso"}, KeepOld: true,
			Checked: true, Latest: source.Candidate{Image: iso.Image{Filename: "b-new.iso"}}},
		{Current: iso.Image{Family: "Arch Linux", Filename: "c.iso"}, // unchecked: no pending update
			KeepOld: true},
		{New: true},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	mm := updated.(model)
	for i, want := range []bool{true, true, true, false} {
		if got := mm.rows[i].KeepOld; got != want {
			t.Errorf("row %d KeepOld = %v, want %v", i, got, want)
		}
	}
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	mm = updated.(model)
	for i, u := range mm.rows {
		if i < 2 && u.KeepOld {
			t.Errorf("row %d KeepOld still set after unkeeping all", i)
		}
		if i == 2 && !u.KeepOld {
			t.Errorf("row %d KeepOld was overwritten by a pending-only toggle", i)
		}
		if i == 3 && u.KeepOld {
			t.Errorf("row %d KeepOld was set despite being a catalog row", i)
		}
	}
}

// TestKeepFlagRendersForPendingRowsOnly verifies the Keep column shows a
// checkbox solely for rows whose update would replace the installed ISO; all
// other rows show a dash.
func TestKeepFlagRendersForPendingRowsOnly(t *testing.T) {
	pending := Update{
		Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"},
		Latest:  source.Candidate{Image: iso.Image{Family: "Ubuntu", Filename: "a-new.iso"}},
		Checked: true, KeepOld: true,
	}
	plain := Update{Current: iso.Image{Family: "Debian", Version: "12", Filename: "b.iso"}}
	m := model{rows: []Update{pending, plain}}
	if got := keepCell(stripANSI(m.rowView(0, m.rows[0]))); got != "[x]" {
		t.Errorf("pending row Keep cell = %q, want [x]", got)
	}
	if got := keepCell(stripANSI(m.rowView(1, m.rows[1]))); got != "—" {
		t.Errorf("non-pending row Keep cell = %q, want a dash", got)
	}
}

// keepCell extracts the Keep column's visible text from a rendered row: the
// three-rune cursor gutter comes first, then the table columns, and cell()
// pads one space per side.
func keepCell(row string) string {
	start := 3 + widths[0] + widths[1] + widths[2] + widths[3] + widths[4]
	runes := []rune(row)
	return strings.TrimSpace(string(runes[start : start+widths[5]]))
}

// TestDeleteMarksExclusiveSelection verifies that marking an ISO for deletion
// clears its update selection and vice versa.
func TestDeleteMarksExclusiveSelection(t *testing.T) {
	row := Update{
		Current:     iso.Image{Family: "Ubuntu", Filename: "a.iso"},
		CurrentSize: 5 << 30,
		Latest:      source.Candidate{Image: iso.Image{Family: "Ubuntu", Filename: "b.iso"}},
	}
	m := model{rows: []Update{row}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = updated.(model)
	if !m.rows[0].DeleteISO || m.rows[0].Selected {
		t.Errorf("delete row = %+v, want DeleteISO and cleared selection", m.rows[0])
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = updated.(model)
	if m.rows[0].DeleteISO || !m.rows[0].Selected {
		t.Errorf("update row = %+v, want Selected and cleared delete mark", m.rows[0])
	}
}

// TestRebuildRowsDropsInstalledCandidates verifies catalog rows whose newest
// release matches an installed ISO disappear rather than duplicate it.
func TestRebuildRowsDropsInstalledCandidates(t *testing.T) {
	m := model{
		rows: []Update{
			{Current: iso.Image{Family: "Ubuntu", Edition: "Desktop LTS", Filename: "ubuntu-26.04.1-desktop-amd64.iso"}},
			{New: true, Checked: true, Current: iso.Image{Family: "Ubuntu", Edition: "Desktop LTS"},
				Latest: source.Candidate{Image: iso.Image{Filename: "ubuntu-26.04.1-desktop-amd64.iso"}}},
			{New: true, Checked: true, Current: iso.Image{Family: "Arch Linux"},
				Latest: source.Candidate{Image: iso.Image{Filename: "archlinux-2026.01.01-x86_64.iso"}}},
		},
	}
	m.rebuildRows()
	if len(m.rows) != 2 {
		t.Fatalf("len(rows) = %d, want the Ubuntu duplicate dropped", len(m.rows))
	}
	if m.rows[0].Current.Family != "Arch Linux" {
		t.Errorf("rows[0] family = %q, recognized rows sort first", m.rows[0].Current.Family)
	}
}

// TestUnrecognizedListLast verifies diskRows keeps canonical ISOs at the top
// and pushes unrecognized files to the bottom of the table.
func TestUnrecognizedListLast(t *testing.T) {
	images := []iso.Image{
		{Path: "/iso/backup-2024.iso", Filename: "backup-2024.iso"},
		{Path: "/iso/ubuntu-24.04.3-desktop-amd64.iso", Family: "Ubuntu", Edition: "Desktop LTS", Version: "24.04.3", Filename: "ubuntu-24.04.3-desktop-amd64.iso"},
		{Path: "/iso/archlinux-2026.01.01-x86_64.iso", Family: "Arch Linux", Version: "2026.01.01", Filename: "archlinux-2026.01.01-x86_64.iso"},
	}
	rows := diskRows(images, false)
	want := []string{"Arch Linux", "Ubuntu Desktop LTS"}
	for i, label := range want {
		if got := rowLabel(rows[i]); got != label {
			t.Errorf("row %d = %q, want %q", i, got, label)
		}
	}
	if rows[len(rows)-1].Current.Filename != "backup-2024.iso" {
		t.Errorf("last row = %q, want the unrecognized file last", rows[len(rows)-1].Current.Filename)
	}
}

// TestLongFilenameTruncatesToFit verifies unrecognized filenames wider than
// the ISO column are ellipsized so the table stays on single lines.
func TestLongFilenameTruncatesToFit(t *testing.T) {
	name := "kali-linux-2026.3-installer-amd64-netinst-long-name.iso"
	m := model{rows: []Update{{Current: iso.Image{Path: "/iso/" + name, Filename: name}}}}
	row := m.rowView(0, m.rows[0])
	if strings.Contains(row, "\n") {
		t.Fatal("row view spans multiple lines")
	}
	plain := stripANSI(row)
	total := 3 // the row gutter (cursor indicator, space, tag-or-filler)
	for _, w := range widths {
		total += w
	}
	if runes := len([]rune(plain)); runes > total {
		t.Errorf("row width = %d runes, exceeds the %d-column table", runes, total)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("row = %q, want an ellipsis in the truncated filename", plain)
	}
}

// TestAgeOnlyForPendingReleases verifies up-to-date rows keep their age but
// in the steady gray, while pending releases use the threshold palette.
func TestAgeOnlyForPendingReleases(t *testing.T) {
	published := time.Now().Add(-48 * time.Hour)
	current := iso.Image{Family: "Ubuntu", Edition: "Desktop LTS", Version: "24.04.3",
		Filename: "ubuntu-24.04.3-desktop-amd64.iso"}
	latest := source.Candidate{
		Image:     iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: current.Filename},
		Published: published,
	}
	m := model{rows: []Update{{Current: current, Latest: latest, Checked: true}}}
	row := stripANSI(m.rowView(0, m.rows[0]))
	if !strings.Contains(row, "2d") {
		t.Errorf("up-to-date row lost its age: %q", row)
	}
	if got := m.rows[0].ageShade(); got != lipgloss.Color("245") {
		t.Errorf("up-to-date shade = %v, want gray 245", got)
	}

	m.rows[0].Latest.Image.Version = "26.04"
	m.rows[0].Latest.Image.Filename = "ubuntu-26.04-desktop-amd64.iso"
	row = stripANSI(m.rowView(0, m.rows[0]))
	if !strings.Contains(row, "2d") {
		t.Errorf("update row lost its age: %q", row)
	}
	if got := m.rows[0].ageShade(); got != lipgloss.Color("255") {
		t.Errorf("pending release shade = %v, want white 255 for a fresh release", got)
	}
}

// TestAgeColorShades verifies the pending-release palette: white for fresh
// releases, yellow after a month, red after half a year.
func TestAgeColorShades(t *testing.T) {
	cases := []struct {
		days int
		want lipgloss.Color
	}{
		{0, lipgloss.Color("255")},
		{1, lipgloss.Color("255")},
		{29, lipgloss.Color("255")},
		{31, lipgloss.Color("220")},
		{180, lipgloss.Color("196")},
		{365, lipgloss.Color("196")},
	}
	for _, c := range cases {
		if got := ageColor(time.Now().Add(-time.Duration(c.days) * 24 * time.Hour)); got != c.want {
			t.Errorf("ageColor(%d days) = %v, want %v", c.days, got, c.want)
		}
	}
}

// TestFlavorInParentheses verifies canonical labels wrap the flavor in
// parentheses while editions stay bare, e.g. "Manjaro (KDE)".
func TestFlavorInParentheses(t *testing.T) {
	cases := map[string]iso.Image{
		"Ubuntu Desktop LTS":           {Family: "Ubuntu", Edition: "Desktop LTS"},
		"Linux Mint (Cinnamon)":        {Family: "Linux Mint", Flavor: "Cinnamon"},
		"openSUSE Leap":                {Family: "openSUSE", Edition: "Leap"},
		"Manjaro (KDE)":                {Family: "Manjaro", Flavor: "KDE"},
		"Ubuntu Desktop LTS (arm64)":   {Family: "Ubuntu", Edition: "Desktop LTS", Arch: "arm64"},
		"Linux Mint (Cinnamon, amd64)": {Family: "Linux Mint", Flavor: "Cinnamon", Arch: "amd64"},
	}
	for want, img := range cases {
		if got := flavorLabel(img); got != want {
			t.Errorf("flavorLabel(%v) = %q, want %q", img, got, want)
		}
	}
}

// TestOpenCatalogSkipsListedDistributions verifies the download screen only
// offers what the drive neither holds nor already queued for download.
// TestEmptyDriveStillOpensUI verifies a drive without ISOs is not rejected:
// the manager shows the empty hint and the catalog download screen works.
func TestEmptyDriveStillOpensUI(t *testing.T) {
	m := model{rows: nil}
	if view := m.View(); !strings.Contains(view, "no ISOs") || !strings.Contains(view, "n to download") {
		t.Errorf("empty drive view = %q, want the download hint", view)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mm := updated.(model)
	if mm.mode != modeCatalog || len(mm.tree) == 0 {
		t.Errorf("pressing n on an empty drive: mode %v with %d catalog rows",
			mm.mode, len(mm.tree))
	}
}

func TestOpenCatalogSkipsListedDistributions(t *testing.T) {
	listed := flavorKey(iso.Image{Family: "Ubuntu", Edition: "Desktop LTS", Version: "24.04.3", Arch: "amd64"})
	m := model{rows: []Update{
		{Current: iso.Image{Family: "Ubuntu", Edition: "Desktop LTS", Version: "24.04.3", Arch: "amd64", Filename: "a.iso"}},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	mm := updated.(model)
	if mm.mode != modeCatalog {
		t.Fatalf("pressing n did not open the catalog (mode %v)", mm.mode)
	}
	var leaves int
	for _, node := range mm.tree {
		if node.arch == "" {
			continue
		}
		leaves++
		if flavorKey(node.image) == listed {
			t.Errorf("Ubuntu Desktop LTS amd64 offered although the drive holds it (%+v)", node.image)
		}
	}
	// Every catalog image except the installed Ubuntu amd64 one is offered.
	offers := 0
	for _, entry := range iso.Catalog() {
		offers += len(entry.Images())
	}
	offers-- // Ubuntu amd64 is already on the drive
	if leaves != offers {
		t.Errorf("leaf count = %d, want %d", leaves, offers)
	}
}

// TestOpenCatalogEmptyView verifies a fully-listed catalog renders the
// nothing-left message instead of an empty selection screen.
func TestOpenCatalogEmptyView(t *testing.T) {
	m := model{
		mode: modeCatalog,
		tree: []catalogNode{},
	}
	if got := m.catalogView(); !strings.Contains(got, "already listed") {
		t.Errorf("catalogView = %q, want the exhausted-catalog message", got)
	}
}

// TestCatalogHintsPutAddSelectedOnTheSecondLine verifies the download screen
// announces the enter action as "add selected" on the second footer line.
func TestCatalogHintsPutAddSelectedOnTheSecondLine(t *testing.T) {
	m := model{
		mode:    modeCatalog,
		tree:    []catalogNode{{image: iso.Image{Family: "Ubuntu", Arch: "amd64"}}},
		treeSel: map[string]bool{"ubuntu/amd64": true},
	}
	plain := stripANSI(m.hints())
	if strings.Contains(plain, "download") {
		t.Errorf("catalog hints still mention downloading: %q", plain)
	}
	lines := strings.Split(plain, "\n")
	if len(lines) != 2 {
		t.Fatalf("catalog hints render %d lines, want two: %q", len(lines), plain)
	}
	if !strings.Contains(lines[1], "esc back") || !strings.Contains(lines[1], "enter add selected") {
		t.Errorf("second catalog line = %q, want esc back and enter add selected", lines[1])
	}
}

// TestUncheckedRowsCarryAQuestionMark verifies recognized ISOs render "?" in
// the Available column until the update check runs, while unrecognized files
// keep their dash.
func TestUncheckedRowsCarryAQuestionMark(t *testing.T) {
	m := model{rows: []Update{
		{Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"}},
		{Current: iso.Image{Filename: "backup.iso"}},
	}}
	if got := availableCell(stripANSI(m.rowView(0, m.rows[0]))); got != "?" {
		t.Errorf("unchecked recognized row Available cell = %q, want ?", got)
	}
	if got := availableCell(stripANSI(m.rowView(1, m.rows[1]))); got != "—" {
		t.Errorf("unrecognized row Available cell = %q, want a dash", got)
	}

	m.rows[0].Checked = true
	m.rows[0].Err = errors.New("mirror down")
	if got := availableCell(stripANSI(m.rowView(0, m.rows[0]))); got != "Unavailable" {
		t.Errorf("failed check Available cell = %q, want Unavailable", got)
	}
}

// availableCell extracts the Available column's visible text from a rendered
// row: columns run after the three-rune cursor gutter.
func availableCell(row string) string {
	start := 3 + widths[0] + widths[1] + widths[2]
	runes := []rune(row)
	return strings.TrimSpace(string(runes[start : start+widths[3]]))
}

// TestUnrecognizedHiddenByDefault verifies unrecognized ISOs stay invisible
// behind a count line until the s toggle reveals them.
func TestUnrecognizedHiddenByDefault(t *testing.T) {
	m := newModel("", "", []Update{
		{Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"}},
		{Current: iso.Image{Filename: "backup.iso"}},
		{Current: iso.Image{Filename: "another.iso"}},
	}, space.Space{})
	table := m.tableView()
	if strings.Contains(table, "backup.iso") {
		t.Errorf("unrecognized ISOs visible before the toggle: %q", table)
	}
	if !strings.Contains(table, "(+2 unrecognized ISOs)") {
		t.Errorf("hidden count line missing: %q", table)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	table = stripANSI(updated.(model).tableView())
	if !strings.Contains(table, "backup.iso") || !strings.Contains(table, "Unrecognized ISOs") {
		t.Errorf("toggle did not reveal unrecognized rows: %q", table)
	}
	if strings.Contains(table, "(+2") {
		t.Errorf("count line still rendered after revealing: %q", table)
	}

	updated, _ = updated.(model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	table = stripANSI(updated.(model).tableView())
	if strings.Contains(table, "backup.iso") {
		t.Errorf("double toggle failed to hide the rows again: %q", table)
	}
	if !strings.Contains(table, "(+2 unrecognized ISOs)") {
		t.Errorf("count line missing after hiding again: %q", table)
	}
}

// TestCursorFollowsVisibility verifies navigation skips hidden rows and the
// toggle keeps the cursor on the same ISO.
func TestCursorFollowsVisibility(t *testing.T) {
	m := newModel("", "", []Update{
		{Current: iso.Image{Family: "Ubuntu", Filename: "a.iso"}},
		{Current: iso.Image{Filename: "backup.iso"}},
	}, space.Space{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm := updated.(model)
	if got := mm.cursorRow(); got != 0 {
		t.Errorf("cursorRow after down = %d, want to stay on row 0 (hidden row skipped)", got)
	}

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	mm = updated.(model)
	if got := mm.cursorRow(); got != 0 {
		t.Errorf("cursorRow after toggle = %d, want the same row 0", got)
	}

	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm = updated.(model)
	if got := mm.cursorRow(); got != 1 {
		t.Errorf("cursorRow after down = %d, want the revealed row 1", got)
	}
}

// TestUnrecognizedRowsRenderAlignedDashes verifies unrecognized ISOs skip
// the toggle checkboxes: both the Update and Keep columns instead carry a
// grayed-out dash, offset to sit inside the bracket zone of the boxes.
func TestUnrecognizedRowsRenderAlignedDashes(t *testing.T) {
	unrec := Update{Current: iso.Image{Filename: "backup.iso"}}
	pending := Update{
		Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"},
		Latest:  source.Candidate{Image: iso.Image{Family: "Ubuntu", Filename: "a-new.iso"}},
		Checked: true, KeepOld: true,
	}
	m := model{rows: []Update{unrec, pending}}

	// The Update cell's dash must land exactly where a box's interior sits.
	row := stripANSI(m.rowView(0, m.rows[0]))
	updateStart := 3
	if got := dashAt(row, updateStart); got != '-' {
		t.Errorf("unrecognized Update cell dash = %q, want one inside the box zone", got)
	}
	if got := strings.TrimSpace(runesOf(row, updateStart, updateStart+widths[0])); got != "-" {
		t.Errorf("unrecognized Update cell text = %q, want a bare dash", got)
	}

	// Same for the Keep column.
	keepStart := updateStart + widths[0] + widths[1] + widths[2] + widths[3] + widths[4]
	if got := strings.TrimSpace(runesOf(row, keepStart, keepStart+widths[5])); got != "-" {
		t.Errorf("unrecognized Keep cell text = %q, want a bare dash", got)
	}
	if got := strings.TrimSpace(runesOf(stripANSI(m.rowView(1, m.rows[1])), keepStart, keepStart+widths[5])); got != "[x]" {
		t.Errorf("pending row Keep cell = %q, want [x]", got)
	}
}

// dashAt returns the rune three columns into a cell's interior, where the
// checkbox interior sits once cell() has padded one leading space.
func dashAt(row string, cellStart int) byte {
	runes := []rune(row)
	// " -" plus cell padding = "  -", so the dash lands on the cell's third
	// rune — the box interior position of "[x]"/"[ ]".
	return byte(runes[cellStart+2])
}

// runesOf returns the runes of row between start and end.
func runesOf(row string, start, end int) string {
	runes := []rune(row)
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}

// TestVimMovementAndSelectionNaming verifies j and k move the cursor again
// now that the keep toggles moved off those keys.
func TestVimMovementAndSelectionNaming(t *testing.T) {
	m := newModel("", "", []Update{
		{Current: iso.Image{Family: "Ubuntu", Filename: "a.iso"}},
		{Current: iso.Image{Family: "Debian", Filename: "b.iso"}},
	}, space.Space{})
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	mm := updated.(model)
	if got := mm.cursorRow(); got != 1 {
		t.Errorf("cursorRow after j = %d, want 1", got)
	}
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	mm = updated.(model)
	if got := mm.cursorRow(); got != 0 {
		t.Errorf("cursorRow after k = %d, want 0", got)
	}
}

// TestUpdateAllSelectsEveryPendingUpdate verifies a selects every selectable
// update without starting the batch, without touching plain or catalog rows,
// and without dropping unrelated delete marks.
func TestUpdateAllSelectsEveryPendingUpdate(t *testing.T) {
	m := model{space: space.Space{Total: 100 << 30, Free: 50 << 30}, rows: []Update{
		{Current: iso.Image{Family: "Ubuntu", Filename: "a.iso"}, Checked: true,
			Latest: source.Candidate{Image: iso.Image{Filename: "a-new.iso"}, Size: 1 << 30}},
		{Current: iso.Image{Filename: "backup.iso"}},
		{New: true, Latest: source.Candidate{Image: iso.Image{Filename: "tails.iso"}}},
		{Current: iso.Image{Family: "Debian", Filename: "b.iso"}, DeleteISO: true},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	mm := updated.(model)
	if !mm.rows[0].Selected {
		t.Errorf("update-all did not mark the pending row: %+v", mm.rows)
	}
	if mm.rows[1].Selected {
		t.Error("unrecognized row got selected by update-all")
	}
	if mm.rows[2].Selected {
		t.Error("update-all touched a catalog addition, which is queued on its own")
	}
	if mm.applying {
		t.Error("update-all started applying instead of waiting for enter")
	}
	if !mm.rows[3].DeleteISO || mm.rows[3].Selected {
		t.Error("update-all disturbed an explicit delete mark")
	}

	// Exclusivity: a delete-marked row that becomes selectable loses the
	// delete mark when update-all selects it.
	mm.rows[3].Latest = source.Candidate{Image: iso.Image{Filename: "b-new.iso"}}
	mm.rows[3].Err, mm.rows[3].Checked = nil, true
	mm.toggleSelectAll()
	if !mm.rows[3].Selected || mm.rows[3].DeleteISO {
		t.Errorf("update-all must select the row and drop its delete mark: %+v", mm.rows[3])
	}

	// Pressing a again resets the whole batch: every selection clears.
	mm.toggleSelectAll()
	for i, u := range mm.rows {
		if u.Selected && selectable(u) {
			t.Errorf("row %d stayed selected after the all-selected reset", i)
		}
	}
}

// TestApplyCancellationStops the running batch handles the user's x/esc:
// pressing it calls the stored cancel function while the batch is still
// applying, and the cancelled appliedMsg unwinds to a notice in the manager
// view rather than an error or a program exit.
func TestApplyCancellationStopsTheRunningBatch(t *testing.T) {
	m := model{applying: true}
	invoked := false
	m.cancel = func() { invoked = true }

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mm := updated.(model)
	if !invoked {
		t.Error("pressing x did not cancel the running batch")
		return
	}
	if !mm.applying {
		t.Error("x must not end the apply phase; the batch goroutine reports back")
	}
	if cmd != nil {
		t.Error("cancellation should not schedule the next message")
	}

	// The goroutine then reports the cancelled batch; the model returns to
	// the manager view with a notice instead of quitting with status.
	mm.applying = true
	updated, _ = mm.Update(appliedMsg{err: errCancelled})
	mm = updated.(model)
	if mm.applying {
		t.Error("applying must stop after the cancelled batch reports back")
	}
	if mm.cancel != nil {
		t.Error("the cancel handle stayed set after cancellation")
	}
	if mm.err != "" {
		t.Errorf("cancellation surfaced as an error: %q", mm.err)
	}
	if mm.done != "" {
		t.Errorf("cancellation recorded the exit message: %q", mm.done)
	}
	if mm.notice == "" {
		t.Error("cancellation left no notice for the manager view")
	}

	// The progress screen tells the user cancellation is possible.
	if got := m.progressView(); !strings.Contains(stripANSI(got), "x or esc") {
		t.Errorf("progress screen hides the cancel keys: %q", got)
	}
}

// TestApplyStopsWhenContextCancelled drives a real batch against a slow HTTP
// server and verifies a cancelled context stops the downloads, reports
// errCancelled, and touches neither the old ISO nor the final target.
func TestApplyStopsWhenContextCancelled(t *testing.T) {
	started := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		for r.Context().Err() == nil { // stalled transfer; writes notice the client leaving
			if _, err := w.Write([]byte("chunk")); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer slow.Close()

	dir := t.TempDir()
	if werr := os.WriteFile(filepath.Join(dir, "ventoy.json"), []byte("{}"), 0o644); werr != nil {
		t.Fatal(werr)
	}
	old := dir + "/old.iso"
	if werr := os.WriteFile(old, []byte("old"), 0o644); werr != nil {
		t.Fatal(werr)
	}
	rows := []Update{{
		Current:  iso.Image{Path: old, Family: "X", Filename: "old.iso"},
		Selected: true, Checked: true,
		Latest: source.Candidate{
			Image: iso.Image{Filename: "old-new.iso"}, URL: slow.URL, Size: 1 << 30,
		},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan error, 1)
	go func() {
		results <- apply(ctx, dir, dir, rows, nil)
	}()
	<-started
	time.Sleep(100 * time.Millisecond) // let the download take hold
	cancel()
	if err := <-results; !errors.Is(err, errCancelled) {
		t.Errorf("apply returned %v, want errCancelled", err)
	}
	if _, statErr := os.Stat(old); statErr != nil {
		t.Error("the old ISO was disturbed although the download never completed")
	}
	entries, readErr := os.ReadDir(dir)
	// Only the fixtures survive: the partial download is cleaned up.
	if readErr != nil || len(entries) != 2 {
		t.Errorf("files left behind: %v (stat err %v)", entries, readErr)
	}
}

// TestApplyWithoutVentoyConfig verifies a drive without ventoy.json still
// downloads and replaces ISOs, and never creates a configuration file.
func TestApplyWithoutVentoyConfig(t *testing.T) {
	payload := []byte("new-iso")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dir := t.TempDir()
	old := filepath.Join(dir, "old.iso")
	if err := os.WriteFile(old, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := []Update{{
		Current:  iso.Image{Path: old, Family: "X", Filename: "old.iso"},
		Selected: true, Checked: true,
		Latest: source.Candidate{
			Image: iso.Image{Filename: "new.iso"}, URL: server.URL, Size: int64(len(payload)),
		},
	}}
	if err := apply(context.Background(), dir, dir, rows, nil); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "new.iso")); err != nil || string(got) != string(payload) {
		t.Errorf("downloaded file = %q, %v; want %q", got, err, payload)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old ISO was not removed (stat err %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ventoy.json")); !os.IsNotExist(err) {
		t.Errorf("apply created a ventoy.json for a config-less drive (stat err %v)", err)
	}
}

// TestUpdateCheckboxOnlyWhenUpdatable verifies the Update cell carries a
// checkbox only for rows that can actually toggle an update: unchecked,
// up-to-date, and failed rows show the aligned gray dash instead.
func TestUpdateCheckboxOnlyWhenUpdatable(t *testing.T) {
	base := Update{Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"}}
	upToDate := base
	upToDate.Checked = true
	upToDate.Latest = source.Candidate{Image: iso.Image{Version: "24.04.3", Filename: "a.iso"}}
	failed := base
	failed.Checked = true
	failed.Err = errors.New("mirror down")
	pending := base
	pending.Checked = true
	pending.Latest = source.Candidate{Image: iso.Image{Version: "26.04", Filename: "a-new.iso"}}

	m := model{rows: []Update{base, upToDate, failed, pending}}
	for i, want := range []string{"-", "-", "-", "[ ]"} {
		row := stripANSI(m.rowView(i, m.rows[i]))
		if got := updateCell(row); got != want {
			t.Errorf("row %d Update cell = %q, want %q", i, got, want)
		}
		if want == "-" {
			if got := dashAt(row, 3); got != '-' {
				t.Errorf("row %d dash not aligned with the box zone: %q", i, row)
			}
		}
	}

	m.rows[3].Selected = true
	if got := updateCell(stripANSI(m.rowView(3, m.rows[3]))); got != "[x]" {
		t.Errorf("selected pending row Update cell = %q, want [x]", got)
	}
}

// updateCell extracts the Update column's visible text from a rendered row:
// columns begin after the three-rune cursor gutter.
func updateCell(row string) string {
	runes := []rune(row)
	return strings.TrimSpace(string(runes[3 : 3+widths[0]]))
}

// TestCheckedOrderKeepsAdditionsOnTop verifies the post-check ordering:
// queued catalog additions stay on top, pending updates follow with the
// freshest release first, then the remainder by age (undated rows sink), and
// unrecognized files stay last.
func TestCheckedOrderKeepsAdditionsOnTop(t *testing.T) {
	unrecognized := Update{Current: iso.Image{Filename: "backup.iso"}}
	fedora := Update{Current: iso.Image{Family: "Fedora", Version: "44", Filename: "f.iso"}} // never checked
	ubuntu := Update{
		Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"},
		Checked: true,
		Latest: source.Candidate{
			Image:     iso.Image{Family: "Ubuntu", Version: "26.04", Filename: "a-new.iso"},
			Published: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
		},
	}
	arch := Update{
		Current: iso.Image{Family: "Arch Linux", Version: "2026.05.01", Filename: "c.iso"},
		Checked: true,
		Latest: source.Candidate{
			Image:     iso.Image{Family: "Arch Linux", Version: "2026.10.01", Filename: "c-new.iso"},
			Published: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
		},
	}
	debian := Update{
		Current: iso.Image{Family: "Debian", Version: "12.9", Filename: "b.iso"},
		Checked: true,
		Latest: source.Candidate{
			Image:     iso.Image{Family: "Debian", Version: "12.9", Filename: "b.iso"}, // up to date
			Published: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	tails := Update{
		New: true, Checked: true,
		Current: iso.Image{Family: "Tails"},
		Latest: source.Candidate{
			Image:     iso.Image{Family: "Tails", Version: "7.6", Filename: "tails.iso"},
			Published: time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC),
		},
	}
	m := model{rows: []Update{debian, unrecognized, ubuntu, fedora, tails, arch}}
	m.rebuildRows()

	want := []string{"Tails", "Arch Linux", "Ubuntu", "Debian", "Fedora", "backup.iso"}
	for i, label := range want {
		row := m.rows[i]
		got := row.Current.Family
		if got == "" {
			got = row.Current.Filename
		}
		if got != label {
			t.Errorf("row %d = %q, want %q", i, got, label)
		}
	}
}

// TestHintsAdaptToState adapts: only keys that can act are listed,
// contextual actions lead the first line and global commands the second,
// and the unchecked-update `c` hint is highlighted in yellow.
func TestHintsAdaptToState(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256) // under go test colors are off
	defer lipgloss.SetColorProfile(original)

	unrecognized := Update{Current: iso.Image{Filename: "backup.iso"}}
	ubuntu := Update{Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"}}
	m := model{rows: []Update{ubuntu, unrecognized}}
	raw := m.hints()
	plain := strings.Join(strings.SplitN(stripANSI(raw), "\n", 999), "\n")

	first := strings.SplitN(plain, "\n", 2)[0]
	if !strings.Contains(first, "↑/↓ selection") || !strings.Contains(first, "d remove ISO from drive") {
		t.Errorf("first line should carry movement and deletion, got %q", first)
	}
	for _, absent := range []string{"space update", "a update all", "o keep old version", "O keep all old", "enter apply"} {
		if strings.Contains(plain, absent) {
			t.Errorf("hint %q offered although no row can use it", absent)
		}
	}
	lines := strings.Split(plain, "\n")
	if len(lines) > 3 {
		t.Fatalf("hints render %d lines, want at most three: %q", len(lines), plain)
	}
	second, third := lines[1], lines[2]
	if second != "c check updates • n new ISO • s unrecognized: hidden" {
		t.Errorf("second global line = %q (quit has its own third line)", second)
	}
	if !strings.HasPrefix(second, "c check updates") {
		t.Error("c must lead the global line")
	}
	if !strings.Contains(raw, "\x1b[38;5;220mc check updates") {
		t.Error("c must be yellow before any update check has run")
	}
	if len(third) > 0 && !strings.HasPrefix(third, "q quit") {
		t.Errorf("third confirm/exit line = %q, want q quit alone", third)
	}
	if !strings.Contains(raw, "\x1b[38;5;196mq quit") {
		t.Error("q quit must be red")
	}

	// After a check with pending updates, the yellow highlight disappears and
	// the rows' applicable actions appear.
	installed := Update{
		Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"},
		Checked: true,
		Latest: source.Candidate{
			Image:     iso.Image{Family: "Ubuntu", Version: "26.04", Filename: "a-new.iso"},
			Published: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
		},
	}
	upToDate := installed
	upToDate.Current.Family, upToDate.Current.Filename = "Debian", "b.iso"
	upToDate.Latest.Image.Version, upToDate.Latest.Image.Filename = "12.9", "b.iso"
	m = model{rows: []Update{installed, upToDate}}
	plain = stripANSI(m.hints())
	for _, present := range []string{"space update", "a update all", "o keep old version", "O keep all old"} {
		if !strings.Contains(plain, present) {
			t.Errorf("hint %q missing although rows can use it", present)
		}
	}
	if strings.Contains(m.hints(), "\x1b[38;5;220mc check") {
		t.Error("c must not be yellow once updates were checked")
	}

	m.rows[0].Selected = true
	hints := m.hints()
	if !strings.Contains(stripANSI(hints), "enter apply") {
		t.Error("enter apply should appear as soon as a selection exists")
	}
	if !strings.Contains(hints, "\x1b[38;5;42menter apply") {
		t.Error("enter apply must be green when applicable")
	}
	if !strings.Contains(hints, "\x1b[38;5;196mq quit") {
		t.Error("q quit must be red")
	}
	// The confirm row reads apply • quit, in that order and left-aligned.
	third = strings.SplitN(stripANSI(hints), "\n", 3)[2]
	if !strings.HasPrefix(third, "enter apply • q quit") {
		t.Errorf("third confirm/exit line = %q, want enter apply first, then q quit", third)
	}

	m.width = 80
	confirm := strings.Split(strings.TrimRight(stripANSI(m.hints()), " "), "\n")
	last := confirm[len(confirm)-1]
	if last != "enter apply • q quit" {
		t.Errorf("left-aligned confirm row = %q, want it flush left", last)
	}
}

// TestCheckCursorSnapsToTopForRecognizedRows verifies the post-check cursor:
// resting on a recognized ISO it snaps to the first row (the re-grouped
// table makes mid-table positions stale); on an unrecognized ISO the
// attach-to-row behaviour survives instead.
func TestCheckCursorSnapsToTopForRecognizedRows(t *testing.T) {
	pending := Update{
		Checked: true, Current: iso.Image{Family: "Ubuntu", Version: "24.04.3", Filename: "a.iso"},
		Latest: source.Candidate{Image: iso.Image{Family: "Ubuntu", Version: "26.04", Filename: "a-new.iso"}},
	}
	upToDate := Update{
		Checked: true, Current: iso.Image{Family: "Debian", Version: "12.9", Filename: "b.iso"},
		Latest: source.Candidate{Image: iso.Image{Version: "12.9", Filename: "b.iso"}},
	}
	unrecognized := Update{Current: iso.Image{Filename: "backup.iso"}}

	// Recognized ISO: the cursor rests on Debian, which slides under the
	// pending Ubuntu during the re-sort; the cursor must land on the top
	// row, not stay attached mid-table.
	m := model{rows: []Update{upToDate, pending, unrecognized}}
	if m.cursorRow() != 0 { // Debian
		t.Fatalf("cursor rests on row %d, want Debian", m.cursorRow())
	}
	m.rebuildRows()
	if m.cursor != 0 {
		t.Errorf("cursor after the check = %d, want 0 (top row)", m.cursor)
	}

	// Unrecognized ISO: the cursor keeps pointing at its row even though
	// the re-sort moves it last.
	m = model{rows: []Update{unrecognized, pending, upToDate}, showUnrecognized: true}
	m.cursor = m.positionOf(0)
	m.rebuildRows()
	if pos := m.positionOf(0); m.cursor != pos {
		t.Errorf("cursor on an unrecognized row = %d, want %d (attached)", m.cursor, pos)
	}
}

// TestCatalogAdditionsCountTowardProjection verifies a queued catalog row
// carries its download size into the projected usage bar.
func TestCatalogAdditionsCountTowardProjection(t *testing.T) {
	m := model{rows: []Update{{
		New: true, Selected: true, Checked: true,
		Current:    iso.Image{Family: "Tails"},
		LatestSize: 2 << 30,
		Latest: source.Candidate{
			Image: iso.Image{Family: "Tails", Version: "7.6", Filename: "tails.iso"},
			Size:  2 << 30,
		},
	}}}
	add, freed := spaceEffect(m.rows)
	if add != 2<<30 || freed != 0 {
		t.Errorf("spaceEffect = (%d, %d), want (2GiB, 0) for a queued addition", add, freed)
	}
}

// TestCatalogAdditionsOfferNoUpdateOrDelete verifies to-be-added rows show
// placeholders in the Update and Delete columns, carry a green plus tag, and
// only advertise their removal from the list.
func TestCatalogAdditionsOfferNoUpdateOrDelete(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256) // under go test colors are off
	defer lipgloss.SetColorProfile(original)

	m := model{rows: []Update{{
		New: true, Selected: true, Checked: true,
		Current:    iso.Image{Family: "Tails"},
		LatestSize: 2 << 30,
		Latest: source.Candidate{
			Image: iso.Image{Family: "Tails", Version: "7.6", Filename: "tails.iso"},
			Size:  2 << 30,
		},
	}}}
	raw := m.rowView(0, m.rows[0])
	if !strings.Contains(raw, "\x1b[38;5;42m+") {
		t.Errorf("catalog addition tag = %q, want a green plus", raw)
	}
	if strings.Contains(raw, "✚") || strings.Contains(raw, "\x1b[38;5;220m") {
		t.Errorf("catalog addition tag = %q, want no unicode plus or yellow", raw)
	}
	row := stripANSI(raw)
	if got := updateCell(row); got != "-" {
		t.Errorf("catalog addition Update cell = %q, want a placeholder", got)
	}
	if got := deleteCell(row); got != "-" {
		t.Errorf("catalog addition Delete cell = %q, want a placeholder", got)
	}

	plain := stripANSI(m.hints())
	if !strings.Contains(plain, "space/d remove from list") {
		t.Errorf("catalog addition hints = %q, want the removal hint", plain)
	}
	if strings.Contains(plain, "space update") || strings.Contains(plain, "delete") {
		t.Errorf("catalog addition hints = %q, want no update or delete option", plain)
	}
}

// TestSpaceDropsQueuedCatalogRow verifies space removes a to-be-added row
// instead of toggling an update mark.
func TestSpaceDropsQueuedCatalogRow(t *testing.T) {
	m := model{rows: []Update{
		{New: true, Selected: true, Current: iso.Image{Family: "Tails"}},
		{Current: iso.Image{Family: "Ubuntu", Filename: "a.iso"}},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	mm := updated.(model)
	if len(mm.rows) != 1 || mm.rows[0].New {
		t.Errorf("space left rows %+v, want the queued addition dropped", mm.rows)
	}
}

// deleteCell extracts the Delete column's visible text from a rendered row.
func deleteCell(row string) string {
	start := 3
	for _, w := range widths[:6] {
		start += w
	}
	runes := []rune(row)
	return strings.TrimSpace(string(runes[start : start+widths[6]]))
}

// TestSpaceDeletesWithoutPendingUpdate verifies space doubles as delete when
// the cursor row offers no update — an up-to-date ISO here — and the hint
// line stops advertising updates while it does so.
func TestSpaceDeletesWithoutPendingUpdate(t *testing.T) {
	upToDate := Update{
		Checked: true,
		Current: iso.Image{Family: "Debian", Version: "12.9", Filename: "b.iso"},
		Latest:  source.Candidate{Image: iso.Image{Version: "12.9", Filename: "b.iso"}},
	}
	m := model{rows: []Update{upToDate}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	mm := updated.(model)
	if !mm.rows[0].DeleteISO || mm.rows[0].Selected {
		t.Errorf("space on an up-to-date row = %+v, want a delete mark and no selection", mm.rows[0])
	}

	plain := stripANSI(mm.hints())
	if !strings.Contains(plain, "space/d remove ISO from drive") {
		t.Errorf("hint line should advertise space as removal: %q", plain)
	}
	if strings.Contains(plain, "space update") || strings.Contains(plain, "o keep old version") {
		t.Error("update hints leaked into a row without an available update")
	}
}

// TestDeleteMarksUnrecognizedISOs verifies d works on unrecognized files:
// the mark toggles, drives the projected space bar, and never fights the
// (absent) update selection.
func TestDeleteMarksUnrecognizedISOs(t *testing.T) {
	m := model{
		rows: []Update{
			{Current: iso.Image{Filename: "backup.iso", Path: "/mnt/x/backup.iso"}, CurrentSize: 2 << 30},
			{Current: iso.Image{Family: "Ubuntu", Filename: "a.iso"}},
		},
		showUnrecognized: true,
	}
	if m.positionOf(0) != 0 {
		t.Fatal("the unrecognized row should be visible and first")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm := updated.(model)
	if !mm.rows[0].DeleteISO {
		t.Error("d did not mark the unrecognized ISO for deletion")
	}

	// Toggling twice clears it again.
	mm.rows[0].Selected = false
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	mm = updated.(model)
	if mm.rows[0].DeleteISO {
		t.Error("a second d press did not clear the delete mark")
	}

	// The projection counts the freed bytes.
	mm.rows[0].DeleteISO = true
	add, freed := spaceEffect(mm.rows)
	if add != 0 || freed != 2<<30 {
		t.Errorf("spaceEffect = (%d, %d), want (0, 2GiB) for a delete-marked unrecognized ISO", add, freed)
	}
}

// TestDeleteBarNeverGrows verifies marking an ISO for deletion cannot grow
// the space bar: cells come from cumulative boundaries, so the filled length
// keeps or shrinks and the green segment simply covers the freed area.
func TestDeleteBarNeverGrows(t *testing.T) {
	rows := []Update{{
		Current:     iso.Image{Family: "Arch Linux", Version: "2026.05.01", Filename: "archlinux-x86_64.iso"},
		CurrentSize: 3 << 30,
	}}
	m := model{rows: rows, space: space.Space{Total: 64 << 30, Free: 55 << 30}}

	filled := func(view string) int { return strings.Count(stripANSI(strings.Split(view, "\n")[1]), "█") }
	before := filled(m.spaceView()) // 9 GiB of 64: proportional 5 cells
	if before != 5 {
		t.Fatalf("unchanged bar = %d filled cells, want 5", before)
	}

	m.rows[0].DeleteISO = true
	afterCells := filled(m.spaceView())
	if afterCells > before {
		t.Errorf("deletion grew the bar from %d to %d filled cells", before, afterCells)
	}
	if afterCells != 5 { // cyan 3 (6 GiB live) + green 2 (freed area)
		t.Fatalf("deletion bar = %d filled cells, want 5 split as cyan+green", afterCells)
	}

	// The pathological case that used to inflate the bar: 1 GiB used on a
	// 64 GiB drive rounds to zero cells with any one-cell minimum gone.
	m = model{rows: rows, space: space.Space{Total: 64 << 30, Free: 63 << 30}}
	m.rows[0].CurrentSize = 1 << 30
	if cells := filled(m.spaceView()); cells != 0 {
		t.Errorf("tiny usage bar = %d cells, want proportional 0", cells)
	}
	m.rows[0].DeleteISO = true
	if cells := filled(m.spaceView()); cells > 1 {
		t.Errorf("tiny deletion bar = %d cells, want no growth", cells)
	}
}

var errTest = &testError{}

type testError struct{}

func (*testError) Error() string { return "test" }

// stripANSI removes escape sequences so tests can count visible characters.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
