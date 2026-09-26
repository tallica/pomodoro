package pomodoro

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock lets the tests advance time without sleeping.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type recorder struct {
	sessions []Session
	finished []Finished
}

func newTestEngine(t *testing.T, cfg Config) (*Engine, *fakeClock, *recorder) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 9, 19, 9, 0, 0, 0, time.UTC)}
	rec := &recorder{}
	e := NewEngine(cfg, Events{
		OnSession:  func(s Session) { rec.sessions = append(rec.sessions, s) },
		OnFinished: func(f Finished) { rec.finished = append(rec.finished, f) },
	})
	e.now = clk.now
	return e, clk, rec
}

// tick drains the engine the way Run does, without the real ticker.
func tick(e *Engine) {
	for {
		fin, sess, ok := e.expire()
		if !ok {
			return
		}
		e.emit(fin, sess)
	}
}

func TestFocusCompletesAndRecordsFullLength(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoStartBreaks = false
	e, clk, rec := newTestEngine(t, cfg)

	e.Start()
	if got := e.Snapshot().Remaining; got != 25*time.Minute {
		t.Fatalf("remaining after start = %v, want 25m", got)
	}

	clk.advance(10 * time.Minute)
	if got := e.Snapshot().Remaining; got != 15*time.Minute {
		t.Fatalf("remaining after 10m = %v, want 15m", got)
	}

	clk.advance(15 * time.Minute)
	tick(e)

	if len(rec.sessions) != 1 {
		t.Fatalf("recorded %d sessions, want 1", len(rec.sessions))
	}
	s := rec.sessions[0]
	if !s.Completed || s.Kind != PhaseFocus || s.Actual != 1500 {
		t.Fatalf("session = %+v, want a completed 1500s focus", s)
	}

	v := e.Snapshot()
	if v.Phase != PhaseShortBreak || v.State != StateIdle {
		t.Fatalf("after focus: phase=%v state=%v, want short break, idle", v.Phase, v.State)
	}
}

// A Mac that sleeps through the deadline must not bank the sleep as work.
func TestSleepPastDeadlineRecordsPlannedLength(t *testing.T) {
	e, clk, rec := newTestEngine(t, DefaultConfig())
	e.Start()
	clk.advance(3 * time.Hour)
	tick(e)

	if got := rec.sessions[0].Actual; got != 1500 {
		t.Fatalf("actual = %ds, want 1500", got)
	}
}

func TestLongBreakEveryFourthRound(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoStartBreaks = true
	cfg.AutoStartFocus = true
	e, clk, _ := newTestEngine(t, cfg)

	e.Start()
	var breaks []Phase
	for i := 0; i < 4; i++ {
		clk.advance(cfg.Duration(PhaseFocus))
		tick(e)
		breaks = append(breaks, e.Snapshot().Phase)
		clk.advance(cfg.Duration(e.Snapshot().Phase))
		tick(e)
	}

	want := []Phase{PhaseShortBreak, PhaseShortBreak, PhaseShortBreak, PhaseLongBreak}
	for i := range want {
		if breaks[i] != want[i] {
			t.Fatalf("break %d = %v, want %v (all: %v)", i+1, breaks[i], want[i], breaks)
		}
	}
}

func TestPauseHoldsRemaining(t *testing.T) {
	e, clk, _ := newTestEngine(t, DefaultConfig())
	e.Start()
	clk.advance(5 * time.Minute)
	e.Pause()

	clk.advance(time.Hour)
	if got := e.Snapshot().Remaining; got != 20*time.Minute {
		t.Fatalf("remaining while paused = %v, want 20m", got)
	}

	e.Toggle() // resume
	clk.advance(20 * time.Minute)
	tick(e)
	if v := e.Snapshot(); v.Phase != PhaseShortBreak {
		t.Fatalf("phase after resumed focus = %v, want short break", v.Phase)
	}
}

func TestResetRecordsInterruptedFocusOnlyWhenSubstantial(t *testing.T) {
	e, clk, rec := newTestEngine(t, DefaultConfig())

	e.Start()
	clk.advance(20 * time.Second)
	e.Reset()
	if len(rec.sessions) != 0 {
		t.Fatalf("a 20s abandoned focus was logged: %+v", rec.sessions)
	}

	e.Start()
	clk.advance(9 * time.Minute)
	e.Reset()
	if len(rec.sessions) != 1 {
		t.Fatalf("recorded %d sessions, want 1", len(rec.sessions))
	}
	if s := rec.sessions[0]; s.Completed || s.Actual != 540 {
		t.Fatalf("session = %+v, want an interrupted 540s focus", s)
	}
	if v := e.Snapshot(); v.State != StateIdle || v.Remaining != 25*time.Minute {
		t.Fatalf("after reset: state=%v remaining=%v, want idle at full length", v.State, v.Remaining)
	}
}

func TestSkipDoesNotCountTowardLongBreakCycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoStartBreaks = false
	e, _, _ := newTestEngine(t, cfg)

	for i := 0; i < 5; i++ {
		e.SwitchTo(PhaseFocus)
		e.Start()
		e.Skip()
	}
	if got := e.Snapshot().Phase; got != PhaseShortBreak {
		t.Fatalf("phase after five skipped rounds = %v, want short break", got)
	}
}

func TestFocusWaitsAfterBreakRunsOut(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoStartBreaks = true
	cfg.AutoStartFocus = false
	e, clk, _ := newTestEngine(t, cfg)

	e.Start()
	clk.advance(cfg.Duration(PhaseFocus))
	tick(e)
	clk.advance(cfg.Duration(PhaseShortBreak))
	tick(e)
	if v := e.Snapshot(); v.Phase != PhaseFocus || v.State != StateIdle {
		t.Fatalf("after break ran out: phase=%v state=%v, want idle focus", v.Phase, v.State)
	}
}

func TestSkipStartsNextPhaseRegardlessOfAutoStart(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoStartBreaks = false
	cfg.AutoStartFocus = false
	e, _, _ := newTestEngine(t, cfg)

	e.Start()
	e.Skip()
	if v := e.Snapshot(); v.Phase != PhaseShortBreak || v.State != StateRunning {
		t.Fatalf("after skipping focus: phase=%v state=%v, want a running short break", v.Phase, v.State)
	}

	e.Skip()
	if v := e.Snapshot(); v.Phase != PhaseFocus || v.State != StateRunning {
		t.Fatalf("after skipping break: phase=%v state=%v, want running focus", v.Phase, v.State)
	}
}

func TestSetConfigLeavesRunningTimerAlone(t *testing.T) {
	e, clk, _ := newTestEngine(t, DefaultConfig())
	e.Start()
	clk.advance(time.Minute)

	cfg := e.Config()
	cfg.FocusMinutes = 50
	e.SetConfig(cfg)

	if got := e.Snapshot().Remaining; got != 24*time.Minute {
		t.Fatalf("remaining after config change = %v, want 24m", got)
	}

	clk.advance(24 * time.Minute)
	tick(e)
	e.SwitchTo(PhaseFocus)
	if got := e.Snapshot().Remaining; got != 50*time.Minute {
		t.Fatalf("next focus = %v, want 50m", got)
	}
}

func TestClampRejectsZeroAndOutOfRange(t *testing.T) {
	c := Config{FocusMinutes: 0, ShortBreakMinutes: -4, LongBreakMinutes: 9999, LongBreakEvery: 0, DailyGoal: 0}
	c.clamp()
	if c.FocusMinutes != 25 || c.ShortBreakMinutes != 1 || c.LongBreakMinutes != 120 || c.LongBreakEvery != 4 {
		t.Fatalf("clamped config = %+v", c)
	}
}

// TestRunLoopDrivesCompletion exercises the real Run loop on the real clock,
// which the fake-clock tests above deliberately bypass.
func TestRunLoopDrivesCompletion(t *testing.T) {
	finished := make(chan Finished, 1)
	sessions := make(chan Session, 1)
	updates := make(chan struct{}, 64)

	cfg := DefaultConfig()
	cfg.AutoStartBreaks = false
	e := NewEngine(cfg, Events{
		OnFinished: func(f Finished) { finished <- f },
		OnSession:  func(s Session) { sessions <- s },
		OnUpdate: func() {
			select {
			case updates <- struct{}{}:
			default:
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.Run(ctx)

	e.Start()
	// Shorten the in-flight phase rather than waiting 25 real minutes.
	e.mu.Lock()
	e.total = 300 * time.Millisecond
	e.deadline = e.now().Add(300 * time.Millisecond)
	e.mu.Unlock()

	select {
	case f := <-finished:
		if f.Phase != PhaseFocus || f.Next != PhaseShortBreak || !f.Natural {
			t.Fatalf("finished = %+v", f)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run never completed the phase")
	}

	select {
	case s := <-sessions:
		if !s.Completed {
			t.Fatalf("session = %+v, want completed", s)
		}
	case <-time.After(time.Second):
		t.Fatal("no session recorded")
	}

	if v := e.Snapshot(); v.Phase != PhaseShortBreak || v.State != StateIdle {
		t.Fatalf("after completion: %+v", v)
	}
}
