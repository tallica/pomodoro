package ui

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/caseymrm/menuet/v2"

	"github.com/tallica/pomodoro/internal/pomodoro"
)

func (u *UI) settings() []menuet.MenuItem {
	cfg := u.engine.Config()

	return []menuet.MenuItem{
		u.minutesMenu("Focus", cfg.FocusMinutes, []int{15, 20, 25, 30, 45, 50, 60},
			func(c *pomodoro.Config, v int) { c.FocusMinutes = v }),
		u.minutesMenu("Short break", cfg.ShortBreakMinutes, []int{3, 5, 8, 10, 15},
			func(c *pomodoro.Config, v int) { c.ShortBreakMinutes = v }),
		u.minutesMenu("Long break", cfg.LongBreakMinutes, []int{10, 15, 20, 25, 30},
			func(c *pomodoro.Config, v int) { c.LongBreakMinutes = v }),
		u.countMenu("Long break after", cfg.LongBreakEvery, []int{2, 3, 4, 5, 6}, "rounds",
			func(c *pomodoro.Config, v int) { c.LongBreakEvery = v }),
		menuet.Separator{},
		u.toggle("Auto-start breaks", cfg.AutoStartBreaks,
			func(c *pomodoro.Config, v bool) { c.AutoStartBreaks = v }),
		u.toggle("Auto-start next focus", cfg.AutoStartFocus,
			func(c *pomodoro.Config, v bool) { c.AutoStartFocus = v }),
		menuet.Separator{},
		u.toggle("Play a sound", cfg.Sound,
			func(c *pomodoro.Config, v bool) { c.Sound = v }),
		u.toggle("Show notifications", cfg.Notifications,
			func(c *pomodoro.Config, v bool) { c.Notifications = v }),
		u.toggle("Show countdown in menu bar", cfg.ShowCountdown,
			func(c *pomodoro.Config, v bool) { c.ShowCountdown = v }),
		menuet.Separator{},
		u.countMenu("Daily goal", cfg.DailyGoal, []int{0, 4, 6, 8, 10, 12, 16}, "pomodoros",
			func(c *pomodoro.Config, v int) { c.DailyGoal = v }),
		menuet.Separator{},
		menuet.Regular{
			Text:    "Reset round counter",
			Clicked: u.engine.ResetCycle,
		},
		menuet.Regular{
			Text:    "Open data folder",
			Clicked: func() { _ = exec.Command("open", u.dataDir).Run() },
		},
		menuet.Separator{},
		menuet.Regular{
			Runs:   []menuet.TextRun{{Text: "Pomodoro " + u.version, Color: menuet.LabelTertiary}},
			Static: true,
		},
	}
}

// apply mutates the config, hands it to the engine (which clamps it) and writes
// the clamped result back to disk, so what is saved is what is running.
func (u *UI) apply(mut func(*pomodoro.Config)) {
	cfg := u.engine.Config()
	mut(&cfg)
	u.engine.SetConfig(cfg)
	_ = pomodoro.SaveConfig(u.cfgPath, u.engine.Config())
	u.Refresh()
}

func (u *UI) toggle(label string, on bool, set func(*pomodoro.Config, bool)) menuet.Regular {
	return menuet.Regular{
		Text:  label,
		State: on,
		Clicked: func() {
			u.apply(func(c *pomodoro.Config) { set(c, !on) })
		},
	}
}

// minutesMenu is a preset list plus a "Custom…" prompt, so the common lengths
// are one click away without capping anyone to the presets.
func (u *UI) minutesMenu(label string, current int, options []int, set func(*pomodoro.Config, int)) menuet.Regular {
	return menuet.Regular{
		Runs: []menuet.TextRun{
			{Text: pad(label, 18), Monospaced: true},
			{Text: fmt.Sprintf("%d min", current), Monospaced: true, Color: menuet.LabelSecondary},
		},
		Children: func() []menuet.MenuItem {
			items := make([]menuet.MenuItem, 0, len(options)+2)
			for _, opt := range options {
				items = append(items, menuet.Regular{
					Text:    fmt.Sprintf("%d minutes", opt),
					State:   opt == current,
					Clicked: func() { u.apply(func(c *pomodoro.Config) { set(c, opt) }) },
				})
			}
			items = append(items, menuet.Separator{}, menuet.Regular{
				Text:  "Custom…",
				State: !contains(options, current),
				Clicked: func() {
					if v, ok := u.askNumber(label, "Length in minutes (1–180)", current); ok {
						u.apply(func(c *pomodoro.Config) { set(c, v) })
					}
				},
			})
			return items
		},
	}
}

func (u *UI) countMenu(label string, current int, options []int, unit string, set func(*pomodoro.Config, int)) menuet.Regular {
	value := fmt.Sprintf("%d %s", current, unit)
	if current == 0 {
		value = "off"
	}
	return menuet.Regular{
		Runs: []menuet.TextRun{
			{Text: pad(label, 18), Monospaced: true},
			{Text: value, Monospaced: true, Color: menuet.LabelSecondary},
		},
		Children: func() []menuet.MenuItem {
			items := make([]menuet.MenuItem, 0, len(options))
			for _, opt := range options {
				text := fmt.Sprintf("%d %s", opt, unit)
				if opt == 0 {
					text = "No goal"
				}
				items = append(items, menuet.Regular{
					Text:    text,
					State:   opt == current,
					Clicked: func() { u.apply(func(c *pomodoro.Config) { set(c, opt) }) },
				})
			}
			return items
		},
	}
}

// askNumber prompts with a modal. Clicked callbacks already run on their own
// goroutine, so blocking here does not stall the menu.
func (u *UI) askNumber(title, prompt string, current int) (int, bool) {
	res := u.app.Alert(menuet.Alert{
		MessageText:     title,
		InformativeText: prompt,
		Buttons:         []string{"Set", "Cancel"},
		Inputs:          []menuet.AlertInput{{Placeholder: prompt, Value: strconv.Itoa(current)}},
	})
	if res.Button != 0 || len(res.Inputs) == 0 {
		return 0, false
	}
	v, err := strconv.Atoi(strings.TrimSpace(res.Inputs[0]))
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

func contains(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
