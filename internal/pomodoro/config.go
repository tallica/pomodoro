package pomodoro

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config holds every user-tunable setting. It is persisted as JSON next to the
// session log so that a crash or a force-quit never loses preferences.
type Config struct {
	FocusMinutes      int  `json:"focus_minutes"`
	ShortBreakMinutes int  `json:"short_break_minutes"`
	LongBreakMinutes  int  `json:"long_break_minutes"`
	LongBreakEvery    int  `json:"long_break_every"`
	AutoStartBreaks   bool `json:"auto_start_breaks"`
	AutoStartFocus    bool `json:"auto_start_focus"`
	Sound             bool `json:"sound"`
	Notifications     bool `json:"notifications"`
	ShowCountdown     bool `json:"show_countdown"`
	FocusMode         bool `json:"focus_mode"` // run the Focus shortcuts during focus rounds
	DailyGoal         int  `json:"daily_goal"`
}

// DefaultConfig is the classic Pomodoro technique: 25/5, long break every 4th.
func DefaultConfig() Config {
	return Config{
		FocusMinutes:      25,
		ShortBreakMinutes: 5,
		LongBreakMinutes:  15,
		LongBreakEvery:    4,
		AutoStartBreaks:   true,
		AutoStartFocus:    false,
		Sound:             true,
		Notifications:     true,
		ShowCountdown:     true,
		DailyGoal:         8,
	}
}

// Duration returns the configured length of a phase.
func (c Config) Duration(p Phase) time.Duration {
	switch p {
	case PhaseShortBreak:
		return time.Duration(c.ShortBreakMinutes) * time.Minute
	case PhaseLongBreak:
		return time.Duration(c.LongBreakMinutes) * time.Minute
	default:
		return time.Duration(c.FocusMinutes) * time.Minute
	}
}

// clamp keeps a config loaded from disk (or hand-edited) inside sane bounds, so
// a zeroed field can never produce a zero-length timer that fires in a loop.
func (c *Config) clamp() {
	clampInt(&c.FocusMinutes, 1, 180, 25)
	clampInt(&c.ShortBreakMinutes, 1, 60, 5)
	clampInt(&c.LongBreakMinutes, 1, 120, 15)
	clampInt(&c.LongBreakEvery, 1, 12, 4)
	clampInt(&c.DailyGoal, 0, 50, 8)
}

func clampInt(v *int, min, max, fallback int) {
	if *v == 0 {
		*v = fallback
		return
	}
	if *v < min {
		*v = min
	}
	if *v > max {
		*v = max
	}
}

// DataDir is where the config and the session log live.
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Application Support", "Pomodoro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadConfig reads config.json, falling back to defaults for a missing or
// unreadable file — a broken config should never stop the app from launching.
func LoadConfig(path string) Config {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}
	cfg.clamp()
	return cfg
}

// SaveConfig writes config.json atomically.
func SaveConfig(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
