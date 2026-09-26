package ui

import (
	"log"
	"os/exec"
	"strings"

	"github.com/caseymrm/menuet/v2"

	"github.com/tallica/pomodoro/internal/focusmode"
	"github.com/tallica/pomodoro/internal/pomodoro"
)

// syncFocus asks for the macOS Focus to be on exactly while a focus round is
// running with the setting enabled: pausing, a break, or a reset turn it off.
// It is called on every refresh; the controller ignores repeats.
func (u *UI) syncFocus(v View) {
	want := u.engine.Config().FocusMode && v.Phase == pomodoro.PhaseFocus && v.State == pomodoro.StateRunning
	if want && !u.focusWanted.Swap(true) {
		// Each round that starts re-checks, so a shortcut created since the
		// last check clears the warning.
		u.checkShortcuts()
	} else if !want {
		u.focusWanted.Store(false)
	}
	u.focus.Set(want)
}

// checkShortcuts looks up the two shortcuts in the background and refreshes
// the menu with the result.
func (u *UI) checkShortcuts() {
	go func() {
		missing, err := focusmode.Missing()
		if err != nil {
			log.Printf("pomodoro: listing shortcuts: %v", err)
			return
		}
		u.focusMissing.Store(&missing)
		u.Refresh()
	}()
}

// focusWarning is the menu row shown while the setting is on but a shortcut
// it runs does not exist, since the run would otherwise fail silently.
func (u *UI) focusWarning() (menuet.Regular, bool) {
	missing := u.focusMissing.Load()
	if !u.engine.Config().FocusMode || missing == nil || len(*missing) == 0 {
		return menuet.Regular{}, false
	}
	text := "⚠︎  Create the “" + strings.Join(*missing, "” and “") + "” shortcut"
	if len(*missing) > 1 {
		text += "s"
	}
	return menuet.Regular{
		Runs: []menuet.TextRun{{Text: text + " — open Shortcuts", Color: menuet.LabelSecondary}},
		Clicked: func() {
			_ = exec.Command("open", "-a", "Shortcuts").Run()
		},
	}, true
}
