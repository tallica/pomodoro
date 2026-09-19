package pomodoro

import (
	"bufio"
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// Session is one finished (or abandoned) stretch of the timer. Sessions are
// appended to a JSON-lines file: append-only means a crash mid-write can cost
// at most the last line, never the history.
type Session struct {
	Kind      Phase     `json:"kind"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	Planned   int       `json:"planned_seconds"`
	Actual    int       `json:"actual_seconds"`
	Completed bool      `json:"completed"`
}

// Elapsed is how long the session actually ran.
func (s Session) Elapsed() time.Duration { return time.Duration(s.Actual) * time.Second }

// Store keeps the session log in memory and mirrors every addition to disk.
type Store struct {
	mu       sync.RWMutex
	path     string
	sessions []Session
}

// OpenStore loads the log at path, skipping any line that fails to parse.
func OpenStore(path string) (*Store, error) {
	s := &Store{path: path}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var sess Session
		if err := json.Unmarshal(line, &sess); err != nil {
			continue
		}
		s.sessions = append(s.sessions, sess)
	}
	sort.Slice(s.sessions, func(i, j int) bool { return s.sessions[i].Start.Before(s.sessions[j].Start) })
	return s, scanner.Err()
}

// Path is the location of the session log.
func (s *Store) Path() string { return s.path }

// Add records a session and appends it to the log.
func (s *Store) Add(sess Session) error {
	s.mu.Lock()
	s.sessions = append(s.sessions, sess)
	s.mu.Unlock()

	line, err := json.Marshal(sess)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

// Stats is an aggregate over some window of time.
type Stats struct {
	Completed   int           // focus sessions finished
	Interrupted int           // focus sessions abandoned part-way
	Focus       time.Duration // time actually spent focusing (completed + partial)
	Break       time.Duration // time actually spent on breaks
	ActiveDays  int           // distinct days with at least one completed focus
}

// Range aggregates sessions whose start falls in [from, to).
func (s *Store) Range(from, to time.Time) Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var st Stats
	days := make(map[string]struct{})
	for _, sess := range s.sessions {
		if sess.Start.Before(from) || !sess.Start.Before(to) {
			continue
		}
		if sess.Kind == PhaseFocus {
			st.Focus += sess.Elapsed()
			if sess.Completed {
				st.Completed++
				days[sess.Start.Format("2006-01-02")] = struct{}{}
			} else {
				st.Interrupted++
			}
			continue
		}
		st.Break += sess.Elapsed()
	}
	st.ActiveDays = len(days)
	return st
}

// DayStat is one calendar day's worth of focus.
type DayStat struct {
	Day   time.Time
	Stats Stats
}

// LastDays returns the most recent n days, oldest first, including empty ones
// so a chart has a bar per day rather than gaps.
func (s *Store) LastDays(n int, now time.Time) []DayStat {
	out := make([]DayStat, 0, n)
	start := StartOfDay(now).AddDate(0, 0, -(n - 1))
	for i := 0; i < n; i++ {
		day := start.AddDate(0, 0, i)
		out = append(out, DayStat{Day: day, Stats: s.Range(day, day.AddDate(0, 0, 1))})
	}
	return out
}

// Streak counts consecutive days up to now with at least one completed focus
// session. Today not being started yet does not break the streak.
func (s *Store) Streak(now time.Time) int {
	day := StartOfDay(now)
	if s.Range(day, day.AddDate(0, 0, 1)).Completed == 0 {
		day = day.AddDate(0, 0, -1)
	}
	streak := 0
	for {
		if s.Range(day, day.AddDate(0, 0, 1)).Completed == 0 {
			return streak
		}
		streak++
		day = day.AddDate(0, 0, -1)
	}
}

// First returns the start of the earliest recorded session.
func (s *Store) First() (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.sessions) == 0 {
		return time.Time{}, false
	}
	return s.sessions[0].Start, true
}

// Today returns completed focus sessions started today, most recent first.
func (s *Store) Today(now time.Time) []Session {
	from := StartOfDay(now)
	to := from.AddDate(0, 0, 1)

	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Session
	for i := len(s.sessions) - 1; i >= 0; i-- {
		sess := s.sessions[i]
		if sess.Start.Before(from) {
			break
		}
		if !sess.Start.Before(to) || sess.Kind != PhaseFocus {
			continue
		}
		out = append(out, sess)
	}
	return out
}

// StartOfDay truncates to local midnight. time.Truncate works in UTC and so
// lands on the wrong instant for any non-UTC zone; build the date instead.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// StartOfWeek returns the most recent Monday at midnight.
func StartOfWeek(t time.Time) time.Time {
	day := StartOfDay(t)
	offset := (int(day.Weekday()) + 6) % 7 // Monday = 0
	return day.AddDate(0, 0, -offset)
}

// StartOfMonth returns the first of the month at midnight.
func StartOfMonth(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}
