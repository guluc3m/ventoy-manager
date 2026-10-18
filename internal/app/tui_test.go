package app

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
