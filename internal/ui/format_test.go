package ui

import (
	"testing"
	"time"
)

func TestClockRoundsUpSoAFreshTimerShowsItsFullLength(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{25 * time.Minute, "25:00"},
		{25*time.Minute - time.Millisecond, "25:00"},
		{90 * time.Second, "01:30"},
		{time.Millisecond, "00:01"},
		{0, "00:00"},
		{-time.Second, "00:00"},
		{90 * time.Minute, "1:30:00"},
	}
	for _, c := range cases {
		if got := clock(c.in); got != c.want {
			t.Errorf("clock(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHuman(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0m"},
		{45 * time.Minute, "45m"},
		{time.Hour, "1h 00m"},
		{100 * time.Minute, "1h 40m"},
		{25 * time.Hour, "25h 00m"},
	}
	for _, c := range cases {
		if got := human(c.in); got != c.want {
			t.Errorf("human(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBarClampsToWidth(t *testing.T) {
	if got := bar(3, 4, 8); got != "██████░░" {
		t.Errorf("bar(3,4,8) = %q", got)
	}
	if got := bar(9, 4, 8); got != "████████" {
		t.Errorf("bar over max = %q, want full", got)
	}
	if got := bar(1, 0, 4); got != "░░░░" {
		t.Errorf("bar with no max = %q, want empty", got)
	}
	if got := []rune(bar(1, 3, 10)); len(got) != 10 {
		t.Errorf("bar width = %d, want 10", len(got))
	}
}

func TestPadCountsRunesNotBytes(t *testing.T) {
	if got := []rune(pad("café", 6)); len(got) != 6 {
		t.Errorf("pad width = %d, want 6", len(got))
	}
	if got := pad("overlong", 3); got != "overlong" {
		t.Errorf("pad truncated: %q", got)
	}
	if got := lpad("7", 3); got != "  7" {
		t.Errorf("lpad = %q", got)
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1, "pomodoro"); got != "1 pomodoro" {
		t.Errorf("got %q", got)
	}
	if got := plural(3, "pomodoro"); got != "3 pomodoros" {
		t.Errorf("got %q", got)
	}
	if got := plural(0, "day"); got != "0 days" {
		t.Errorf("got %q", got)
	}
}
