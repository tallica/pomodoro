package pomodoro

import (
	"context"
	"sync"
	"time"
)

// Phase is what the timer is currently counting down.
type Phase string

const (
	PhaseFocus      Phase = "focus"
	PhaseShortBreak Phase = "short_break"
	PhaseLongBreak  Phase = "long_break"
)

// Label is the human name of a phase.
func (p Phase) Label() string {
	switch p {
	case PhaseShortBreak:
		return "Short break"
	case PhaseLongBreak:
		return "Long break"
	default:
		return "Focus"
	}
}

// Symbol is the glyph used in the menu bar for a phase. These are plain text
// glyphs rather than emoji so the status item stays monochrome and picks up the
// menu bar's own label color in both light and dark mode: a solid dot for work,
// a hollow one for a break.
func (p Phase) Symbol() string {
	if p == PhaseFocus {
		return "\u25cf" // ● BLACK CIRCLE
	}
	return "\u25cb" // ○ WHITE CIRCLE
}

// IsBreak reports whether the phase is a break rather than focused work.
func (p Phase) IsBreak() bool { return p != PhaseFocus }

// State is the run state of the timer.
type State int

const (
	StateIdle State = iota
	StateRunning
	StatePaused
)

// minRecorded is the shortest abandoned focus worth writing to the log —
// below it, a stray start/stop is noise rather than history.
const minRecorded = time.Minute

// Finished describes a phase that just ended.
type Finished struct {
	Phase       Phase
	Next        Phase
	Natural     bool // ran to zero rather than being skipped
	AutoStarted bool // the next phase started on its own
}

// Events are the callbacks the UI hangs off the engine. All are optional and
// are always invoked outside the engine lock, so a handler may call back in.
type Events struct {
	OnUpdate   func()         // state changed, or the displayed second ticked
	OnFinished func(Finished) // a phase ended
	OnSession  func(Session)  // a session worth recording
}

// Engine is the timer state machine. It is deadline-based rather than
// tick-counting, so it stays accurate across a laptop sleeping mid-session.
type Engine struct {
	mu        sync.Mutex
	cfg       Config
	events    Events
	now       func() time.Time
	phase     Phase
	state     State
	total     time.Duration
	remaining time.Duration // authoritative while idle or paused
	deadline  time.Time     // authoritative while running
	startedAt time.Time     // wall clock start of the current phase
	focusDone int           // completed focus sessions in the current cycle
}

// NewEngine returns an idle engine parked at the start of a focus session.
func NewEngine(cfg Config, events Events) *Engine {
	cfg.clamp()
	e := &Engine{cfg: cfg, events: events, now: time.Now, phase: PhaseFocus}
	e.total = cfg.Duration(PhaseFocus)
	e.remaining = e.total
	return e
}

// View is an immutable read of the engine, safe to render from.
type View struct {
	Phase      Phase
	Next       Phase
	State      State
	Remaining  time.Duration
	Total      time.Duration
	CycleDone  int // completed focus sessions since the last long break
	CycleEvery int
}

// Snapshot captures the current state for rendering.
func (e *Engine) Snapshot() View {
	e.mu.Lock()
	defer e.mu.Unlock()
	return View{
		Phase:      e.phase,
		Next:       e.nextPhaseLocked(),
		State:      e.state,
		Remaining:  e.remainingLocked(),
		Total:      e.total,
		CycleDone:  e.focusDone,
		CycleEvery: e.cfg.LongBreakEvery,
	}
}

// Config returns the engine's current settings.
func (e *Engine) Config() Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cfg
}

// SetConfig applies new settings. A phase that has not started yet picks up its
// new length immediately; a running one keeps its original length so the
// countdown on screen never jumps.
func (e *Engine) SetConfig(cfg Config) {
	cfg.clamp()
	e.mu.Lock()
	e.cfg = cfg
	if e.state == StateIdle {
		e.total = cfg.Duration(e.phase)
		e.remaining = e.total
	}
	e.mu.Unlock()
	e.notify()
}

// Run drives the countdown until ctx is cancelled. It blocks; call it in a
// goroutine. While a phase is running it wakes several times a second so the
// displayed second flips on time; while idle or paused there is nothing to
// count, so it idles at a much lower rate rather than spinning on battery.
func (e *Engine) Run(ctx context.Context) {
	const (
		activeInterval = 200 * time.Millisecond
		idleInterval   = 2 * time.Second
	)

	timer := time.NewTimer(activeInterval)
	defer timer.Stop()

	lastShown := -1
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if fin, sess, ok := e.expire(); ok {
				e.emit(fin, sess)
				lastShown = -1
			}

			v := e.Snapshot()
			interval := idleInterval
			if v.State == StateRunning {
				interval = activeInterval
				if shown := int(v.Remaining.Round(time.Second) / time.Second); shown != lastShown {
					lastShown = shown
					e.notify()
				}
			}
			timer.Reset(interval)
		}
	}
}

// Toggle is the primary control: start an idle timer, pause a running one,
// resume a paused one.
func (e *Engine) Toggle() {
	e.mu.Lock()
	switch e.state {
	case StateRunning:
		e.remaining = e.remainingLocked()
		e.state = StatePaused
	case StatePaused:
		e.deadline = e.now().Add(e.remaining)
		e.state = StateRunning
	default:
		e.startLocked()
	}
	e.mu.Unlock()
	e.notify()
}

// Start begins the current phase if it is not already running.
func (e *Engine) Start() {
	e.mu.Lock()
	if e.state != StateRunning {
		if e.state == StatePaused {
			e.deadline = e.now().Add(e.remaining)
			e.state = StateRunning
		} else {
			e.startLocked()
		}
	}
	e.mu.Unlock()
	e.notify()
}

// Pause stops the countdown, keeping the remaining time.
func (e *Engine) Pause() {
	e.mu.Lock()
	if e.state == StateRunning {
		e.remaining = e.remainingLocked()
		e.state = StatePaused
	}
	e.mu.Unlock()
	e.notify()
}

// Reset returns the current phase to its full length and stops it, logging the
// abandoned session if it had run long enough to matter.
func (e *Engine) Reset() {
	e.mu.Lock()
	sess, logged := e.abandonLocked()
	e.state = StateIdle
	e.total = e.cfg.Duration(e.phase)
	e.remaining = e.total
	e.mu.Unlock()

	if logged {
		e.session(sess)
	}
	e.notify()
}

// Skip ends the current phase early and starts the next one straight away.
// Clicking Skip means the user is at the Mac and ready, so the auto-start
// settings, which exist for phases that end unattended, do not apply.
func (e *Engine) Skip() {
	e.mu.Lock()
	sess, logged := e.abandonLocked()
	fin := e.advanceLocked(false)
	e.mu.Unlock()

	if logged {
		e.session(sess)
	}
	e.emit(fin, Session{})
}

// SwitchTo jumps straight to a given phase, stopped at its full length.
func (e *Engine) SwitchTo(p Phase) {
	e.mu.Lock()
	sess, logged := e.abandonLocked()
	e.phase = p
	e.state = StateIdle
	e.total = e.cfg.Duration(p)
	e.remaining = e.total
	e.mu.Unlock()

	if logged {
		e.session(sess)
	}
	e.notify()
}

// ResetCycle forgets how many focus sessions have passed since the last long
// break, so the next long break is a full cycle away.
func (e *Engine) ResetCycle() {
	e.mu.Lock()
	e.focusDone = 0
	e.mu.Unlock()
	e.notify()
}

// expire completes the phase if its deadline has passed.
func (e *Engine) expire() (Finished, Session, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != StateRunning || e.remainingLocked() > 0 {
		return Finished{}, Session{}, false
	}

	// A natural completion always records the full planned length: if the Mac
	// slept through the deadline, wall-clock elapsed would overstate the work.
	sess := Session{
		Kind:      e.phase,
		Start:     e.startedAt,
		End:       e.startedAt.Add(e.total),
		Planned:   int(e.total / time.Second),
		Actual:    int(e.total / time.Second),
		Completed: true,
	}
	return e.advanceLocked(true), sess, true
}

// advanceLocked moves to the next phase and starts it: always when skipped,
// per the auto-start settings when the phase ran out on its own. Only a focus
// session that ran to completion counts toward the long-break cycle —
// skipping out of one should not earn the long break.
func (e *Engine) advanceLocked(natural bool) Finished {
	done := e.phase
	if natural && done == PhaseFocus {
		e.focusDone++
	}
	next := PhaseFocus
	if done == PhaseFocus {
		next = e.breakAfter(e.focusDone)
	}

	e.phase = next
	e.total = e.cfg.Duration(next)
	e.remaining = e.total
	e.state = StateIdle

	auto := !natural
	switch {
	case natural && next.IsBreak():
		auto = e.cfg.AutoStartBreaks
	case natural:
		auto = e.cfg.AutoStartFocus
	}
	if auto {
		e.startLocked()
	}
	return Finished{Phase: done, Next: next, Natural: natural, AutoStarted: auto}
}

// abandonLocked builds the record for a phase stopped part-way through, and
// reports whether it is worth keeping.
func (e *Engine) abandonLocked() (Session, bool) {
	if e.state == StateIdle {
		return Session{}, false
	}
	elapsed := e.total - e.remainingLocked()
	if elapsed < minRecorded || e.phase != PhaseFocus {
		return Session{}, false
	}
	return Session{
		Kind:      e.phase,
		Start:     e.startedAt,
		End:       e.now(),
		Planned:   int(e.total / time.Second),
		Actual:    int(elapsed / time.Second),
		Completed: false,
	}, true
}

func (e *Engine) startLocked() {
	if e.remaining <= 0 {
		e.remaining = e.cfg.Duration(e.phase)
		e.total = e.remaining
	}
	e.startedAt = e.now()
	e.deadline = e.startedAt.Add(e.remaining)
	e.state = StateRunning
}

func (e *Engine) remainingLocked() time.Duration {
	if e.state != StateRunning {
		return e.remaining
	}
	if d := e.deadline.Sub(e.now()); d > 0 {
		return d
	}
	return 0
}

// nextPhaseLocked previews what follows the current phase: focus → break →
// focus, with a long break every Nth round. An in-flight focus session counts,
// since the preview answers "what happens when this one finishes?".
func (e *Engine) nextPhaseLocked() Phase {
	if e.phase.IsBreak() {
		return PhaseFocus
	}
	done := e.focusDone
	if e.state != StateIdle {
		done++
	}
	return e.breakAfter(done)
}

// breakAfter picks the break that follows the nth completed focus session.
func (e *Engine) breakAfter(done int) Phase {
	if done > 0 && e.cfg.LongBreakEvery > 0 && done%e.cfg.LongBreakEvery == 0 {
		return PhaseLongBreak
	}
	return PhaseShortBreak
}

func (e *Engine) notify() {
	if e.events.OnUpdate != nil {
		e.events.OnUpdate()
	}
}

func (e *Engine) session(s Session) {
	if e.events.OnSession != nil {
		e.events.OnSession(s)
	}
}

func (e *Engine) emit(fin Finished, sess Session) {
	if sess.Kind != "" {
		e.session(sess)
	}
	if e.events.OnFinished != nil {
		e.events.OnFinished(fin)
	}
	e.notify()
}
