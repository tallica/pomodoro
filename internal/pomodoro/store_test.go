package pomodoro

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func focusAt(t time.Time, mins int, done bool) Session {
	return Session{
		Kind: PhaseFocus, Start: t, End: t.Add(time.Duration(mins) * time.Minute),
		Planned: 1500, Actual: mins * 60, Completed: done,
	}
}

func TestStoreRoundTripsThroughDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}

	base := time.Date(2026, 9, 19, 9, 0, 0, 0, time.Local)
	for i := 0; i < 3; i++ {
		if err := s.Add(focusAt(base.Add(time.Duration(i)*time.Hour), 25, true)); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	reopened, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	st := reopened.Range(base.Add(-time.Hour), base.Add(24*time.Hour))
	if st.Completed != 3 || st.Focus != 75*time.Minute {
		t.Fatalf("reloaded stats = %+v, want 3 sessions / 75m", st)
	}
}

func TestStoreSkipsCorruptLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.jsonl")
	good := `{"kind":"focus","start":"2026-09-19T09:00:00Z","end":"2026-09-19T09:25:00Z","planned_seconds":1500,"actual_seconds":1500,"completed":true}`
	if err := os.WriteFile(path, []byte(good+"\nnot json\n\n"+good+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	from := time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC)
	if got := s.Range(from, from.AddDate(0, 0, 1)).Completed; got != 2 {
		t.Fatalf("completed = %d, want 2 (corrupt line skipped)", got)
	}
}

func TestRangeSeparatesFocusBreaksAndInterruptions(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "s.jsonl"))
	day := time.Date(2026, 9, 19, 8, 0, 0, 0, time.Local)

	_ = s.Add(focusAt(day, 25, true))
	_ = s.Add(focusAt(day.Add(time.Hour), 11, false))
	_ = s.Add(Session{Kind: PhaseShortBreak, Start: day.Add(30 * time.Minute), End: day.Add(35 * time.Minute), Actual: 300, Completed: true})

	st := s.Range(StartOfDay(day), StartOfDay(day).AddDate(0, 0, 1))
	if st.Completed != 1 || st.Interrupted != 1 {
		t.Fatalf("completed=%d interrupted=%d, want 1/1", st.Completed, st.Interrupted)
	}
	if st.Focus != 36*time.Minute {
		t.Fatalf("focus = %v, want 36m (completed plus partial)", st.Focus)
	}
	if st.Break != 5*time.Minute {
		t.Fatalf("break = %v, want 5m", st.Break)
	}
}

func TestStreakCountsBackFromTodayAndToleratesAnUnstartedToday(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "s.jsonl"))
	now := time.Date(2026, 9, 19, 14, 0, 0, 0, time.Local)

	for _, back := range []int{1, 2, 3, 6} { // gap at 4 and 5 days ago
		_ = s.Add(focusAt(StartOfDay(now).AddDate(0, 0, -back).Add(10*time.Hour), 25, true))
	}
	if got := s.Streak(now); got != 3 {
		t.Fatalf("streak with nothing today = %d, want 3", got)
	}

	_ = s.Add(focusAt(StartOfDay(now).Add(9*time.Hour), 25, true))
	if got := s.Streak(now); got != 4 {
		t.Fatalf("streak including today = %d, want 4", got)
	}
}

func TestStreakIgnoresInterruptedSessions(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "s.jsonl"))
	now := time.Date(2026, 9, 19, 14, 0, 0, 0, time.Local)
	_ = s.Add(focusAt(StartOfDay(now).Add(9*time.Hour), 8, false))

	if got := s.Streak(now); got != 0 {
		t.Fatalf("streak = %d, want 0", got)
	}
}

func TestLastDaysIncludesEmptyDays(t *testing.T) {
	s, _ := OpenStore(filepath.Join(t.TempDir(), "s.jsonl"))
	now := time.Date(2026, 9, 19, 14, 0, 0, 0, time.Local)
	_ = s.Add(focusAt(StartOfDay(now).AddDate(0, 0, -2).Add(9*time.Hour), 25, true))

	days := s.LastDays(7, now)
	if len(days) != 7 {
		t.Fatalf("got %d days, want 7", len(days))
	}
	if !days[6].Day.Equal(StartOfDay(now)) {
		t.Fatalf("last day = %v, want today", days[6].Day)
	}
	if days[4].Stats.Completed != 1 {
		t.Fatalf("day -2 completed = %d, want 1", days[4].Stats.Completed)
	}
}

// Truncate works in UTC, so a naive implementation puts "today" on the wrong
// day for anyone east or west of Greenwich.
func TestStartOfDayUsesLocalMidnight(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*3600)
	got := StartOfDay(time.Date(2026, 9, 19, 2, 30, 0, 0, zone))
	want := time.Date(2026, 9, 19, 0, 0, 0, 0, zone)
	if !got.Equal(want) {
		t.Fatalf("StartOfDay = %v, want %v", got, want)
	}
}

func TestStartOfWeekIsMonday(t *testing.T) {
	sunday := time.Date(2026, 9, 20, 23, 0, 0, 0, time.Local) // a Sunday
	got := StartOfWeek(sunday)
	if got.Weekday() != time.Monday || got.Day() != 14 {
		t.Fatalf("StartOfWeek(%v) = %v, want Mon 14 Sep", sunday, got)
	}
	monday := time.Date(2026, 9, 14, 0, 30, 0, 0, time.Local)
	if got := StartOfWeek(monday); !got.Equal(StartOfDay(monday)) {
		t.Fatalf("StartOfWeek on a Monday = %v, want that same day", got)
	}
}
