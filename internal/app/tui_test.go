package app

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/guluc3m/ventoy-updater/internal/iso"
	"github.com/guluc3m/ventoy-updater/internal/source"
	"github.com/guluc3m/ventoy-updater/internal/space"
)

// TestApplyCompletionQuits verifies the TUI leaves the alternate screen once the
// apply phase finishes, so a completed update does not sit there forever.
func TestApplyCompletionQuits(t *testing.T) {
	m := model{applying: true, progressCh: make(chan downloadProgress)}

	updated, cmd := m.Update(appliedMsg{})
	finished, ok := updated.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", updated)
	}
	if finished.applying {
		t.Error("applying = true after completion, want false")
	}
	if finished.done == "" {
		t.Error("done message is empty after a successful apply")
	}
	if cmd == nil {
		t.Fatal("no command returned, want tea.Quit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Errorf("command produced %T, want tea.QuitMsg", cmd())
	}
}

// TestResolveJobsCompletionFollowsResults verifies the check producer closes
// the result channel without sending resolveClosed itself. Completion must be
// observed by listenResolve after the last resolveResult; an earlier signal
// would stop the check before the final row's answer is applied, leaving that
// row displaying "?" as though it were never checked.
func TestResolveJobsCompletionFollowsResults(t *testing.T) {
	results := make(chan resolveResult)
	m := model{resCh: results, resolver: source.New()}
	// An unsupported family makes Latest fail immediately, without network.
	jobs := []resolveJob{{index: 0, img: iso.Image{Family: "not-a-distribution"}}}
	done := make(chan tea.Msg, 1)
	go func() { done <- m.resolveJobsCmd(jobs)() }()

	select {
	case result := <-results:
		if result.index != 0 || result.err == nil {
			t.Errorf("result = %+v, want the failed resolution for row 0", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no resolveResult produced")
	}
	select {
	case msg := <-done:
		if _, premature := msg.(resolveClosed); premature {
			t.Fatal("resolveJobsCmd signalled completion before the listener drained results")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resolveJobsCmd did not finish")
	}
	if _, open := <-results; open {
		t.Fatal("result channel is still open after resolution finished")
	}
}

// TestApplyFailureQuitsWithError verifies a failed apply also exits, recording
// the error so Run can report it and set a non-zero exit status.
func TestApplyFailureQuitsWithError(t *testing.T) {
	m := model{applying: true, progressCh: make(chan downloadProgress)}
	downloadErr := errors.New("download stalled: no data received for 60s")

	updated, cmd := m.Update(appliedMsg{err: downloadErr})
	finished := updated.(model)
	if finished.err != downloadErr.Error() {
		t.Errorf("err = %q, want %q", finished.err, downloadErr)
	}
	if cmd == nil {
		t.Fatal("no command returned, want tea.Quit")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Errorf("command produced %T, want tea.QuitMsg", cmd())
	}
}

// catalogTestModel opens the download tree with fresh marks and no queued
// rows, so tests exercise real keys through Update.
func catalogTestModel(t *testing.T) model {
	t.Helper()
	m := newModel(t.TempDir(), "", nil, space.Space{})
	m.tree = buildCatalogTree(map[string]bool{})
	m.mode = modeCatalog
	m.treeCursor = 0
	return m
}

// press types one or more keys into the model and returns the new model.
func (m model) press(t *testing.T, keys ...tea.KeyMsg) model {
	t.Helper()
	for _, key := range keys {
		updated, _ := m.Update(key)
		typed, ok := updated.(model)
		if !ok {
			t.Fatalf("Update returned %T, want model", updated)
		}
		m = typed
	}
	return m
}

// keyRunes types a printable key.
func keyRunes(r ...rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: r} }

// keySpace types the space key, the tree's action key.
func keySpace() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeySpace} }

// countLeafTotal counts every selectable leaf under a tree index, marked or
// not — the other half of countLeaves the tests assert against.
func (m model) countLeafTotal(idx int) int {
	total, _ := m.countLeaves(idx)
	return total
}

// findRow returns the display position of the row labelled label, or -1.
func findRow(m model, label string) int {
	for pos, idx := range m.visibleCatalog() {
		if m.tree[idx].label == label {
			return pos
		}
	}
	return -1
}

// leafCursor finds the display position of the first leaf carrying arch
// below the given ancestry labels, nearest level first.
func leafCursor(t *testing.T, m model, arch string, ancestry ...string) int {
	t.Helper()
	for pos, idx := range m.visibleCatalog() {
		node := m.tree[idx]
		if node.arch != arch {
			continue
		}
		if len(ancestry) == 0 {
			return pos
		}
		depth, walk := 0, node.parent
		for walk >= 0 && depth < len(ancestry) {
			if m.tree[walk].label != ancestry[len(ancestry)-1-depth] {
				break
			}
			depth++
			if depth == len(ancestry) {
				return pos
			}
			walk = m.tree[walk].parent
		}
	}
	t.Fatalf("no %q leaf under %v", arch, ancestry)
	return -1
}

// TestTreeGroupsFamilyEditionFlavorArch verifies the tree keeps its three
// grouping levels and that levels a distribution does not carry collapse
// away.
func TestTreeGroupsFamilyEditionFlavorArch(t *testing.T) {
	tree := buildCatalogTree(map[string]bool{})
	want := []struct{ label, arch string }{
		{"Ubuntu", ""}, {"Desktop LTS", ""}, {"amd64", "amd64"}, {"arm64", "arm64"},
		{"Desktop", ""}, {"amd64", "amd64"}, {"arm64", "arm64"},
		{"Fedora", ""}, {"Workstation", ""}, {"GNOME", ""}, {"amd64", "amd64"}, {"arm64", "arm64"}, {"KDE", ""},
	}
	if len(tree) < len(want) {
		t.Fatalf("tree has %d rows, want at least %d", len(tree), len(want))
	}
	for i, w := range want {
		if tree[i].label != w.label || tree[i].arch != w.arch {
			t.Errorf("tree[%d] = (%q, arch %q), want (%q, arch %q)",
				i, tree[i].label, tree[i].arch, w.label, w.arch)
		}
	}
	// Arch leaves hang under their flavor or edition, and a distribution
	// without an edition or flavor (Debian) hangs its arches straight off
	// the family.
	if tree[2].parent != 1 {
		t.Errorf("Ubuntu amd64 parent = %d, want the Desktop LTS row", tree[2].parent)
	}
	debian := -1
	for i, node := range tree {
		if node.label == "Debian" && node.parent == -1 {
			debian = i
			break
		}
	}
	if debian < 0 || len(tree[debian].children) != 2 ||
		tree[tree[debian].children[0]].arch == "" ||
		tree[tree[debian].children[0]].label != "amd64" ||
		tree[tree[debian].children[1]].label != "arm64" {
		t.Errorf("Debian children = %v, want amd64 and arm64 below the family", debian)
	}
}

// TestTreeSkipsListedRowsAndEmptyLevels verifies an identity the drive holds
// loses its leaf, and a grouping level left with no leaves disappears.
func TestTreeSkipsListedRowsAndEmptyLevels(t *testing.T) {
	listed := map[string]bool{}
	for _, entry := range iso.Catalog() {
		if entry.Family != "Ubuntu" {
			continue
		}
		for _, image := range entry.Images() {
			listed[flavorKey(image)] = true
		}
	}
	tree := buildCatalogTree(listed)
	if tree[0].label != "Fedora" {
		t.Errorf("tree[0] = %q, want the fully-listed Ubuntu level dropped", tree[0].label)
	}
}

// TestTreeSpaceTogglesSubtree verifies space on a family marks every leaf
// beneath it, and space again — from any node of that subtree, even one
// collapsed out of sight — clears the lot.
func TestTreeSpaceTogglesSubtree(t *testing.T) {
	m := catalogTestModel(t)
	m = m.press(t, keySpace())
	total := m.countLeafTotal(m.visibleCatalog()[0])
	if total == 0 || len(m.treeSel) != total {
		t.Errorf("treeSel = %v, want all %d Ubuntu leaves marked", m.treeSel, total)
	}
	m = m.press(t, keyRunes('j'), keyRunes('j'), keyRunes('k'), keyRunes('k'), keySpace())
	if len(m.treeSel) != 0 {
		t.Errorf("treeSel = %v, want a toggle on the full family to clear everything", m.treeSel)
	}
	m = m.press(t, keyRunes('l'), keyRunes('h')) // open, then fold it again
	if !m.tree[m.visibleCatalog()[0]].collapsed {
		t.Fatal("h did not fold the Ubuntu level")
	}
	m = m.press(t, keySpace()) // marks apply under the collapsed level too
	m.treeCursor = 0
	m = m.press(t, keyRunes('l'))
	if len(m.treeSel) != total {
		t.Errorf("treeSel = %v, want all %d marks placed from inside the fold",
			m.treeSel, total)
	}
}

// TestTreeASelectsBelowWithoutClearing verifies a only ever adds marks under
// the node, and unrelated families keep theirs untouched.
func TestTreeASelectsBelowWithoutClearing(t *testing.T) {
	m := catalogTestModel(t)
	m = m.press(t, keySpace()) // Ubuntu-wide: everything under it marked
	total := m.countLeafTotal(m.visibleCatalog()[0])
	m = m.press(t, keyRunes('l'), keyRunes('j'), keyRunes('l')) // open Ubuntu, then its edition
	arm := leafCursor(t, m, "arm64", "Ubuntu", "Desktop LTS")
	m.treeCursor = arm
	m = m.press(t, keySpace()) // clears arm64, leaves the LTS amd64 alone
	if len(m.treeSel) != total-1 {
		t.Errorf("treeSel = %v, want every Ubuntu amd64 leaf marked only", m.treeSel)
	}
	m.treeCursor = 0
	m = m.press(t, keyRunes('a'))
	if len(m.treeSel) != total {
		t.Errorf("treeSel = %v, want all %d leaves marked after a", m.treeSel, total)
	}
	// Linux Mint is an unrelated family: never touched from Ubuntu's node.
	if m.treeSel[flavorKey(iso.Image{Family: "Linux Mint", Flavor: "Cinnamon", Arch: "amd64"})] {
		t.Error("a on Ubuntu reached Linux Mint")
	}
	mint := findRow(m, "Linux Mint")
	if mint < 0 {
		t.Fatal("Linux Mint missing from the tree")
	}
	mintLeaves := m.countLeafTotal(m.visibleCatalog()[mint])
	m.treeCursor = mint
	m = m.press(t, keyRunes('a'))
	if got := len(m.treeSel); got != total+mintLeaves {
		t.Errorf("treeSel holds %d marks after marking Mint, want %d", got, total+mintLeaves)
	}
}

// TestTreeJKLHFold verifies j/k move along visible rows and h/l fold and
// unfold while the cursor parks on the folding node itself. Everything
// starts folded, so Ubuntu is unfolded by hand first.
func TestTreeJKLHFold(t *testing.T) {
	m := catalogTestModel(t)
	families := len(m.visibleCatalog())
	if families == 0 {
		t.Fatal("the tree opened with no visible families")
	}
	if !m.tree[m.visibleCatalog()[0]].collapsed {
		t.Error("the tree should open with every family folded")
	}
	m = m.press(t, keyRunes('j'))
	if m.treeCursor != 1 {
		t.Fatalf("cursor = %d after j, want 1", m.treeCursor)
	}
	m = m.press(t, keyRunes('k'))
	if m.treeCursor != 0 {
		t.Errorf("cursor = %d after k, want 0", m.treeCursor)
	}
	m = m.press(t, keyRunes('l'))
	if len(m.visibleCatalog()) == families {
		t.Errorf("l left the family folded at %d visible rows", families)
	}
	openRows := len(m.visibleCatalog())
	m = m.press(t, keyRunes('h'))
	folded := len(m.visibleCatalog())
	if folded >= openRows {
		t.Errorf("visible rows = %d after h, want fewer than %d", folded, openRows)
	}
	if idx := m.treeRow(); idx != 0 || !m.tree[0].collapsed {
		t.Errorf("cursor parked at %d (collapsed = %v), want the folded Ubuntu node",
			idx, m.tree[idx].collapsed)
	}
	m = m.press(t, keyRunes('l'))
	if m.tree[0].collapsed {
		t.Error("l left the family collapsed")
	}
	if got := len(m.visibleCatalog()); got != openRows {
		t.Errorf("visible rows = %d after l, want %d", got, openRows)
	}
}

// TestTreeHOnLeafFoldsItsHost verifies h (or left) sitting on an arch leaf
// closes the submenu that owns the leaf, parking the cursor on that node:
// Rocky Linux amd64 folds the Rocky Linux family itself.
func TestTreeHOnLeafFoldsItsHost(t *testing.T) {
	m := catalogTestModel(t)
	families := len(m.visibleCatalog())
	m = m.press(t, keyRunes('h')) // Ubuntu folds itself: roots have no host above
	if !m.tree[m.visibleCatalog()[0]].collapsed {
		t.Fatal("h on a root family did not fold it")
	}
	rocky := findRow(m, "Rocky Linux")
	if rocky < 0 {
		t.Fatal("Rocky Linux missing from the tree")
	}
	m.treeCursor = rocky
	m = m.press(t, keyRunes('l')) // unfold Rocky so its amd64 leaf shows up
	m.treeCursor = leafCursor(t, m, "amd64", "Rocky Linux")
	m = m.press(t, keyRunes('h'))
	if idx := m.treeRow(); m.tree[idx].label != "Rocky Linux" || !m.tree[idx].collapsed {
		t.Errorf("cursor parked at %q (collapsed = %v), want the folded Rocky Linux node",
			m.tree[idx].label, m.tree[idx].collapsed)
	}
	if got := len(m.visibleCatalog()); got != families {
		t.Errorf("visible rows = %d, want the %d families back", got, families)
	}
}

// TestTreeHOnOpenGroupFoldsItself verifies an open grouping level folds only
// itself: h on Fedora Workstation's GNOME flavor closes GNOME, leaving the
// Workstation edition's other content untouched.
func TestTreeHOnOpenGroupFoldsItself(t *testing.T) {
	m := catalogTestModel(t)
	fedora := findRow(m, "Fedora")
	if fedora < 0 {
		t.Fatal("Fedora missing from the tree")
	}
	m.treeCursor = fedora
	m = m.press(t, keyRunes('l'))                // open Fedora; Workstation and Silverblue below
	m = m.press(t, keyRunes('j'), keyRunes('l')) // open Workstation; GNOME and KDE below
	flavor := fedora + 2                         // GNOME, the first Workstation flavor
	if gnome := m.tree[m.visibleCatalog()[flavor]]; gnome.label != "GNOME" ||
		gnome.parent != m.visibleCatalog()[fedora+1] {
		t.Fatalf("row below Workstation = %q, want the GNOME flavor node", gnome.label)
	}
	m = m.press(t, keyRunes('j'), keyRunes('l')) // open the GNOME flavor
	if leaf := m.tree[m.visibleCatalog()[flavor+1]]; leaf.arch == "" {
		t.Fatalf("row below GNOME = %q, want an arch leaf", leaf.label)
	}
	m.treeCursor = flavor // back on Workstation's GNOME node
	m = m.press(t, keyRunes('h'))
	if idx := m.treeRow(); m.tree[idx].label != "GNOME" || !m.tree[idx].collapsed {
		t.Errorf("cursor parked at %q (collapsed = %v), want the folded GNOME node",
			m.tree[idx].label, m.tree[idx].collapsed)
	}
	if m.tree[m.visibleCatalog()[fedora+1]].collapsed {
		t.Error("h on GNOME folded Workstation as well")
	}
}

// TestTreeHClimbsWhenItsLevelIsFolded verifies h climbs to the host node
// once nothing at the cursor's level is open anymore — but not while a
// sibling still shows content.
func TestTreeHClimbsWhenItsLevelIsFolded(t *testing.T) {
	m := catalogTestModel(t)
	fedora := findRow(m, "Fedora")
	if fedora < 0 {
		t.Fatal("Fedora missing from the tree")
	}
	m.treeCursor = fedora
	m = m.press(t, keyRunes('l')) // Fedora open; Workstation sits below
	edition := fedora + 1
	if label := m.tree[m.visibleCatalog()[edition]].label; label != "Workstation" {
		t.Fatalf("row below Fedora = %q, want the Workstation edition", label)
	}
	m = m.press(t, keyRunes('j'), keyRunes('l'), keyRunes('j'), keyRunes('l'))
	// Unfolded Workstation → GNOME → amd64: closing GNOME leaves Workstation's
	// level all folded, so the next two h presses climb — Workstation first,
	// then Fedora once nothing in between is open.
	m.treeCursor = edition + 1
	m = m.press(t, keyRunes('h')) // GNOME open → folds itself
	if idx := m.treeRow(); m.tree[idx].label != "GNOME" || !m.tree[idx].collapsed {
		t.Fatalf("cursor parked at %q (collapsed = %v), want GNOME folded first",
			m.tree[idx].label, m.tree[idx].collapsed)
	}
	m = m.press(t, keyRunes('h')) // Workstation folded with everything below
	if idx := m.treeRow(); m.tree[idx].label != "Workstation" || !m.tree[idx].collapsed {
		t.Fatalf("cursor parked at %q (collapsed = %v), want Workstation folded next",
			m.tree[idx].label, m.tree[idx].collapsed)
	}
	m = m.press(t, keyRunes('h')) // Fedora level all folded → climbs to Fedora
	if idx := m.treeRow(); m.tree[idx].label != "Fedora" || !m.tree[idx].collapsed {
		t.Fatalf("cursor parked at %q (collapsed = %v), want Fedora folded last",
			m.tree[idx].label, m.tree[idx].collapsed)
	}
}

// TestTreeHStaysWhenASiblingIsOpen verifies a folded node does not climb to
// its host while any sibling at its level still shows content: h then folds
// the node itself, which changes nothing.
func TestTreeHStaysWhenASiblingIsOpen(t *testing.T) {
	m := catalogTestModel(t)
	fedora := findRow(m, "Fedora")
	if fedora < 0 {
		t.Fatal("Fedora missing from the tree")
	}
	m.treeCursor = fedora
	m = m.press(t, keyRunes('l'))                               // Fedora open: Workstation, Silverblue
	m = m.press(t, keyRunes('j'), keyRunes('j'), keyRunes('l')) // Silverblue unfolds
	m.treeCursor = fedora + 1                                   // folded Workstation
	m = m.press(t, keyRunes('h'))
	if idx := m.treeRow(); m.tree[idx].label != "Workstation" {
		t.Errorf("cursor moved to %q, want it parked on Workstation", m.tree[idx].label)
	}
	if m.tree[m.visibleCatalog()[fedora]].collapsed {
		t.Error("h climbed to Fedora although Silverblue is still open")
	}
}

// TestTreeConfirmQueuesMarkedLeaves verifies enter copies each marked leaf
// into the manager table as a new resolve-and-download row.
func TestTreeConfirmQueuesMarkedLeaves(t *testing.T) {
	m := catalogTestModel(t)
	ubuntu := m.countLeafTotal(m.visibleCatalog()[0])
	m = m.press(t, keySpace()) // mark every Ubuntu leaf
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(model)
	if mm.mode != modeManager {
		t.Fatalf("mode = %v after enter, want the manager screen", mm.mode)
	}
	if len(mm.rows) != ubuntu {
		t.Fatalf("rows = %d, want the %d Ubuntu downloads", len(mm.rows), ubuntu)
	}
	for _, row := range mm.rows {
		if !row.New || row.Current.Family != "Ubuntu" {
			t.Errorf("queued row = %+v, want a new Ubuntu download", row)
		}
		if !row.Selected {
			t.Errorf("queued row %+v is not selected, so the space graph ignores it", row)
		}
	}
	if !mm.checking || cmd == nil {
		t.Errorf("checking = %v, cmd = %v, want the resolve batch to start",
			mm.checking, cmd != nil)
	}
}
