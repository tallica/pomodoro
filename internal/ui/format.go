package ui

import (
	"fmt"
	"strings"
	"time"
)

// clock renders a countdown as MM:SS, rounding up so a fresh 25 minute timer
// reads 25:00 rather than 24:59 the instant it starts.
func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int((d + time.Second - 1) / time.Second)
	if secs >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", secs/3600, (secs%3600)/60, secs%60)
	}
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

// human renders a total as "2h 05m" / "45m" — the shape you want when reading
// a day's worth of work at a glance.
func human(d time.Duration) string {
	mins := int(d.Round(time.Minute) / time.Minute)
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dh %02dm", mins/60, mins%60)
}

// plural returns "1 session" / "3 sessions".
func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// bar draws a text meter. Menus have no progress-view row, and a run of block
// glyphs in a monospaced run lines up cleanly across items.
func bar(value, max, width int) string {
	if max <= 0 || width <= 0 {
		return strings.Repeat("░", maxInt(width, 0))
	}
	filled := value * width / max
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// pad right-pads to n runes so monospaced columns align.
func pad(s string, n int) string {
	if r := []rune(s); len(r) < n {
		return s + strings.Repeat(" ", n-len(r))
	}
	return s
}

// lpad left-pads to n runes, for right-aligned numbers.
func lpad(s string, n int) string {
	if r := []rune(s); len(r) < n {
		return strings.Repeat(" ", n-len(r)) + s
	}
	return s
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
