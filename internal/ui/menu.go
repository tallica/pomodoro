package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/caseymrm/menuet/v2"

	"github.com/tallica/pomodoro/internal/pomodoro"
)

// UI renders the engine and the session log into the macOS menu bar.
type UI struct {
	app     *menuet.Application
	engine  *pomodoro.Engine
	store   *pomodoro.Store
	dataDir string
	cfgPath string
	version string
	icons   bool // status-bar images are bundled; see haveIcons

	notifyDenied atomic.Bool
}

// New wires a UI onto the menuet application singleton.
func New(app *menuet.Application, engine *pomodoro.Engine, store *pomodoro.Store, dataDir, cfgPath, version string) *UI {
	return &UI{app: app, engine: engine, store: store, dataDir: dataDir, cfgPath: cfgPath, version: version, icons: haveIcons()}
}

// Install registers the menu and paints the initial state.
func (u *UI) Install() {
	u.app.Children = u.children
	u.Refresh()

	// Notification() silently does nothing when the user has denied the
	// permission, which reads as "no sessions finished" exactly when it isn't.
	// Check once in the background and surface it in the menu instead.
	go func() {
		if u.app.NotificationAuthorization() == "denied" {
			u.notifyDenied.Store(true)
			u.Refresh()
		}
	}()
}

// Refresh repaints the menu bar title and any open menu.
func (u *UI) Refresh() {
	v := u.engine.Snapshot()
	u.app.SetMenuState(u.menuState(v))
	u.app.MenuChanged()
}

// OnSession persists a finished or abandoned session.
func (u *UI) OnSession(s pomodoro.Session) {
	_ = u.store.Add(s)
}

// OnFinished alerts the user that a phase ended. Skipping a phase is the user's
// own doing, so only a natural completion is announced.
func (u *UI) OnFinished(fin pomodoro.Finished) {
	if !fin.Natural {
		return
	}
	cfg := u.engine.Config()

	if cfg.Sound {
		sound := "Glass"
		if fin.Phase.IsBreak() {
			sound = "Ping"
		}
		go exec.Command("afplay", "/System/Library/Sounds/"+sound+".aiff").Run()
	}
	if !cfg.Notifications {
		return
	}

	title := "Pomodoro complete"
	if fin.Phase.IsBreak() {
		title = fin.Phase.Label() + " over"
	}
	next := fmt.Sprintf("%s · %d min", fin.Next.Label(), int(cfg.Duration(fin.Next)/time.Minute))
	message := "Up next: " + next
	if fin.AutoStarted {
		message = "Started: " + next
	}
	u.app.Notification(menuet.Notification{
		Title:      title,
		Subtitle:   u.todayLine(),
		Message:    message,
		Identifier: "phase-" + strconv.FormatInt(time.Now().UnixNano(), 36),
	})
}

// menuState builds the status item. It is deliberately monochrome: the icon
// is a template image and no run carries a color, so AppKit renders both in
// the menu bar's own label color — white over a dark bar, black over a light
// one, like every system icon. State is carried by shape alone (a solid
// tomato for focus, an outline on a break, a pause mark when paused), never by
// color or dimming. Digits are monospaced so the item keeps a constant width
// instead of jittering every second.
func (u *UI) menuState(v View) *menuet.MenuState {
	cfg := u.engine.Config()
	paused := v.State == pomodoro.StatePaused

	var runs []menuet.TextRun
	switch {
	case v.State == pomodoro.StateIdle:
		if n := u.store.Range(pomodoro.StartOfDay(time.Now()), time.Now().Add(time.Second)).Completed; n > 0 {
			runs = append(runs, menuet.TextRun{Text: " " + strconv.Itoa(n), FontSize: 12, Monospaced: true})
		}
	case cfg.ShowCountdown:
		runs = append(runs, menuet.TextRun{Text: " " + clock(v.Remaining), FontSize: 13, Monospaced: true})
	}

	if !u.icons {
		// A bare binary has no Resources directory to load the icon from;
		// fall back to a glyph so the item is never blank.
		glyph := v.Phase.Symbol()
		if paused {
			glyph = "‖" // ‖ DOUBLE VERTICAL LINE
		}
		runs = append([]menuet.TextRun{{Text: glyph, FontSize: 12}}, runs...)
		return &menuet.MenuState{Runs: runs}
	}

	icon := "tomato-focus"
	if v.Phase.IsBreak() {
		icon = "tomato-break"
	}
	if paused {
		icon += "-paused"
	}
	return &menuet.MenuState{Image: icon, Runs: runs}
}

// haveIcons reports whether the status-bar images were bundled next to the
// executable, where NSImage imageNamed: will find them.
func haveIcons() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(filepath.Dir(exe), "..", "Resources", "tomato-focus.png"))
	return err == nil
}

// View is a local alias so the render helpers read cleanly.
type View = pomodoro.View

func (u *UI) children() []menuet.MenuItem {
	v := u.engine.Snapshot()
	items := u.header(v)
	items = append(items, menuet.Separator{})
	items = append(items, u.controls(v)...)
	items = append(items, menuet.Separator{})
	items = append(items, u.statistics()...)
	items = append(items, menuet.Separator{})

	if u.notifyDenied.Load() {
		items = append(items, menuet.Regular{
			Runs: []menuet.TextRun{
				{Text: "⚠︎  Notifications are off — open System Settings", Color: menuet.LabelSecondary},
			},
			Clicked: func() {
				_ = exec.Command("open", "x-apple.systempreferences:com.apple.preference.notifications").Run()
			},
		})
	}
	items = append(items, menuet.Regular{Text: "Settings", Children: u.settings})
	return items
}

// header is the two rows at the top: what is running with how much is left,
// then a progress line. It is two real rows rather than one row with a
// Subtitle — NSMenuItem.subtitle drops run styling, and menuet renders it as
// an empty band here, so the monospaced meter needs a row of its own.
func (u *UI) header(v View) []menuet.MenuItem {
	state := "ready"
	switch v.State {
	case pomodoro.StateRunning:
		state = clock(v.Remaining) + " left"
	case pomodoro.StatePaused:
		state = clock(v.Remaining) + " left · paused"
	}

	elapsed := v.Total - v.Remaining
	pct := 0
	if v.Total > 0 {
		pct = int(elapsed * 100 / v.Total)
	}

	// Which round of the cycle this is. During a break the round that just
	// finished is the one worth naming, so count back rather than forward.
	cycle := v.CycleDone%maxInt(v.CycleEvery, 1) + 1
	if v.Phase.IsBreak() {
		if cycle = v.CycleDone % maxInt(v.CycleEvery, 1); cycle == 0 {
			cycle = v.CycleEvery
		}
	}

	return []menuet.MenuItem{
		menuet.Regular{
			Runs: []menuet.TextRun{
				{Text: v.Phase.Label(), FontWeight: menuet.WeightSemibold},
				{Text: "  " + state, Monospaced: true, Color: menuet.LabelSecondary},
			},
		},
		menuet.Regular{
			Runs: []menuet.TextRun{
				{Text: bar(pct, 100, 12), Monospaced: true, Color: menuet.LabelSecondary},
				{
					Text: fmt.Sprintf("  %d%%  ·  round %d of %d  ·  next: %s",
						pct, cycle, v.CycleEvery, v.Next.Label()),
					Monospaced: true, FontSize: 12, Color: menuet.LabelTertiary,
				},
			},
		},
	}
}

func (u *UI) controls(v View) []menuet.MenuItem {
	primary := "Start " + v.Phase.Label()
	switch v.State {
	case pomodoro.StateRunning:
		primary = "Pause"
	case pomodoro.StatePaused:
		primary = "Resume"
	}

	return []menuet.MenuItem{
		menuet.Regular{
			Text:     primary,
			Clicked:  u.engine.Toggle,
			Shortcut: &menuet.Shortcut{KeyCode: menuet.KeySpace, Modifiers: menuet.ModCtrl | menuet.ModAlt},
		},
		menuet.Regular{
			Text:     "Skip to " + v.Next.Label(),
			Clicked:  u.engine.Skip,
			Shortcut: &menuet.Shortcut{KeyCode: menuet.KeyS, Modifiers: menuet.ModCtrl | menuet.ModAlt},
		},
		menuet.Regular{
			Text:    "Restart " + v.Phase.Label(),
			Clicked: u.engine.Reset,
		},
	}
}

// statistics renders the three headline rows, each drilling into a breakdown.
func (u *UI) statistics() []menuet.MenuItem {
	now := time.Now()
	end := now.Add(time.Second)

	today := u.store.Range(pomodoro.StartOfDay(now), end)
	week := u.store.Range(pomodoro.StartOfWeek(now), end)
	month := u.store.Range(pomodoro.StartOfMonth(now), end)

	todayRow := statRow("Today", today)
	todayRow.Children = u.todayDetail
	weekRow := statRow("This week", week)
	weekRow.Children = u.weekDetail
	monthRow := statRow("This month", month)
	monthRow.Children = u.monthDetail

	return []menuet.MenuItem{todayRow, weekRow, monthRow}
}

func statRow(label string, st pomodoro.Stats) menuet.Regular {
	return menuet.Regular{
		Runs: []menuet.TextRun{
			{Text: pad(label, 11), Monospaced: true},
			{Text: lpad(strconv.Itoa(st.Completed), 3), Monospaced: true, FontWeight: menuet.WeightSemibold},
			{Text: " · " + lpad(human(st.Focus), 7), Monospaced: true, Color: menuet.LabelSecondary},
		},
	}
}

func (u *UI) todayLine() string {
	st := u.store.Range(pomodoro.StartOfDay(time.Now()), time.Now().Add(time.Second))
	return fmt.Sprintf("Today: %s · %s focused", plural(st.Completed, "pomodoro"), human(st.Focus))
}

func (u *UI) todayDetail() []menuet.MenuItem {
	now := time.Now()
	st := u.store.Range(pomodoro.StartOfDay(now), now.Add(time.Second))
	cfg := u.engine.Config()

	items := []menuet.MenuItem{
		menuet.Regular{Text: now.Format("Monday, 2 January"), FontWeight: menuet.WeightSemibold},
	}
	if cfg.DailyGoal > 0 {
		items = append(items, menuet.Regular{
			Runs: []menuet.TextRun{
				{Text: bar(st.Completed, cfg.DailyGoal, 16) + " ", Monospaced: true, Color: menuet.LabelSecondary},
				{Text: fmt.Sprintf("%d / %d", st.Completed, cfg.DailyGoal), Monospaced: true},
			},
		})
	}
	items = append(items,
		infoRow("Focused", human(st.Focus)),
		infoRow("On break", human(st.Break)),
		infoRow("Interrupted", strconv.Itoa(st.Interrupted)),
	)
	if streak := u.store.Streak(now); streak > 1 {
		items = append(items, infoRow("Streak", plural(streak, "day")))
	}
	items = append(items, menuet.Separator{})

	sessions := u.store.Today(now)
	if len(sessions) == 0 {
		items = append(items, menuet.Regular{Text: "No sessions yet today", Color: menuet.LabelTertiary})
		return items
	}
	if len(sessions) > 12 {
		sessions = sessions[:12]
	}
	for _, s := range sessions {
		mark, color := "✓", menuet.LabelSecondary
		if !s.Completed {
			mark, color = "✗", menuet.LabelTertiary
		}
		items = append(items, menuet.Regular{
			Runs: []menuet.TextRun{
				{Text: mark + " ", Color: color},
				{Text: s.Start.Format("15:04") + " – " + s.End.Format("15:04"), Monospaced: true},
				{Text: "  " + human(s.Elapsed()), Monospaced: true, Color: menuet.LabelTertiary},
			},
		})
	}
	return items
}

func (u *UI) weekDetail() []menuet.MenuItem {
	now := time.Now()
	start := pomodoro.StartOfWeek(now)
	st := u.store.Range(start, now.Add(time.Second))

	items := []menuet.MenuItem{
		menuet.Regular{
			Text:       fmt.Sprintf("Week of %s", start.Format("2 Jan")),
			FontWeight: menuet.WeightSemibold,
		},
	}

	var days []pomodoro.DayStat
	peak := 1
	for i := 0; i < 7; i++ {
		day := start.AddDate(0, 0, i)
		d := pomodoro.DayStat{Day: day, Stats: u.store.Range(day, day.AddDate(0, 0, 1))}
		days = append(days, d)
		if d.Stats.Completed > peak {
			peak = d.Stats.Completed
		}
	}
	items = append(items, dayRows(days, peak, now)...)

	avg := time.Duration(0)
	if st.ActiveDays > 0 {
		avg = st.Focus / time.Duration(st.ActiveDays)
	}
	items = append(items,
		menuet.Separator{},
		infoRow("Total", fmt.Sprintf("%s · %s", plural(st.Completed, "pomodoro"), human(st.Focus))),
		infoRow("Active days", strconv.Itoa(st.ActiveDays)),
		infoRow("Per active day", human(avg)),
	)
	return items
}

func (u *UI) monthDetail() []menuet.MenuItem {
	now := time.Now()
	start := pomodoro.StartOfMonth(now)
	st := u.store.Range(start, now.Add(time.Second))

	items := []menuet.MenuItem{
		menuet.Regular{Text: now.Format("January 2006"), FontWeight: menuet.WeightSemibold},
	}

	// Weekly buckets read better than 30 day rows in a menu.
	type week struct {
		label string
		stats pomodoro.Stats
	}
	var weeks []week
	peak := 1
	for cur := pomodoro.StartOfWeek(start); cur.Before(now); cur = cur.AddDate(0, 0, 7) {
		from, to := cur, cur.AddDate(0, 0, 7)
		if from.Before(start) {
			from = start
		}
		s := u.store.Range(from, to)
		weeks = append(weeks, week{label: from.Format("2 Jan"), stats: s})
		if s.Completed > peak {
			peak = s.Completed
		}
	}
	for _, w := range weeks {
		items = append(items, menuet.Regular{
			Runs: []menuet.TextRun{
				{Text: pad("w/c "+w.label, 12), Monospaced: true},
				{Text: bar(w.stats.Completed, peak, 10) + " ", Monospaced: true, Color: menuet.LabelSecondary},
				{Text: lpad(strconv.Itoa(w.stats.Completed), 3), Monospaced: true},
				{Text: " · " + human(w.stats.Focus), Monospaced: true, Color: menuet.LabelTertiary},
			},
		})
	}

	avgDay := time.Duration(0)
	if st.ActiveDays > 0 {
		avgDay = st.Focus / time.Duration(st.ActiveDays)
	}
	items = append(items,
		menuet.Separator{},
		infoRow("Total", fmt.Sprintf("%s · %s", plural(st.Completed, "pomodoro"), human(st.Focus))),
		infoRow("Active days", strconv.Itoa(st.ActiveDays)),
		infoRow("Per active day", human(avgDay)),
		menuet.Separator{},
		menuet.Regular{Text: "All time", Children: u.allTimeDetail},
	)
	return items
}

func (u *UI) allTimeDetail() []menuet.MenuItem {
	now := time.Now()
	from, ok := u.store.First()
	if !ok {
		return []menuet.MenuItem{menuet.Regular{Text: "No sessions recorded yet"}}
	}
	st := u.store.Range(from.Add(-time.Second), now.Add(time.Second))
	span := int(pomodoro.StartOfDay(now).Sub(pomodoro.StartOfDay(from))/(24*time.Hour)) + 1

	avg := time.Duration(0)
	if st.ActiveDays > 0 {
		avg = st.Focus / time.Duration(st.ActiveDays)
	}
	return []menuet.MenuItem{
		infoRow("Since", from.Format("2 Jan 2006")),
		infoRow("Pomodoros", strconv.Itoa(st.Completed)),
		infoRow("Focused", human(st.Focus)),
		infoRow("Interrupted", strconv.Itoa(st.Interrupted)),
		infoRow("Active days", fmt.Sprintf("%d of %d", st.ActiveDays, span)),
		infoRow("Per active day", human(avg)),
		infoRow("Current streak", plural(u.store.Streak(now), "day")),
		menuet.Separator{},
		menuet.Regular{
			Text:    "Reveal session log in Finder",
			Clicked: func() { _ = exec.Command("open", "-R", u.store.Path()).Run() },
		},
	}
}

func dayRows(days []pomodoro.DayStat, peak int, now time.Time) []menuet.MenuItem {
	today := pomodoro.StartOfDay(now)
	items := make([]menuet.MenuItem, 0, len(days))
	for _, d := range days {
		label := menuet.TextRun{Text: pad(d.Day.Format("Mon 2"), 7), Monospaced: true}
		if d.Day.Equal(today) {
			label.FontWeight = menuet.WeightSemibold
		} else if d.Day.After(today) {
			label.Color = menuet.LabelTertiary
		}
		items = append(items, menuet.Regular{
			Runs: []menuet.TextRun{
				label,
				{Text: bar(d.Stats.Completed, peak, 10) + " ", Monospaced: true, Color: menuet.LabelSecondary},
				{Text: lpad(strconv.Itoa(d.Stats.Completed), 3), Monospaced: true},
				{Text: " · " + human(d.Stats.Focus), Monospaced: true, Color: menuet.LabelTertiary},
			},
		})
	}
	return items
}

func infoRow(label, value string) menuet.Regular {
	return menuet.Regular{
		Runs: []menuet.TextRun{
			{Text: pad(label, 15), Monospaced: true, Color: menuet.LabelSecondary},
			{Text: value, Monospaced: true},
		},
	}
}
