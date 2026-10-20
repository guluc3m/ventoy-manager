package app

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/guluc3m/ventoy-updater/internal/iso"
	"github.com/guluc3m/ventoy-updater/internal/source"
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
