# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Skip starts the next phase straight away.** Skipping a break used to leave
  the focus round stopped when **Auto-start next focus** was off. The auto-start
  settings now apply only to phases that run out on their own.

### Removed

- **Back to focus.** With Skip now starting focus straight away, it was the same
  as **Skip to Focus** followed by Pause.

## [0.2.0] - 2026-09-26

### Changed

- **Status bar icon is now a tomato graphic.** A template image that AppKit
  recolors with the menu bar, white on a dark bar and black on a light one, like
  the system icons. Solid while focusing, outlined on a break, with a pause mark
  when paused. The item is no longer dimmed to gray when idle or paused.

## [0.1.0] - 2026-09-19

Initial release.

### Added

- **Status bar timer.** A monochrome status item showing the current phase and
  time left — a solid dot while focusing, a hollow one on a break, dimmed while
  paused or idle. It takes the menu bar's own label color, so it inverts
  correctly in dark mode and over a tinted wallpaper. No window, no Dock icon.
- **Pomodoro cycle.** Focus rounds separated by short breaks, with a long break
  every Nth round. Start, pause, resume, skip and restart from the menu.
- **Global shortcuts.** `⌃⌥Space` to start or pause and `⌃⌥S` to skip, working
  system-wide so a round can begin without leaving what you are doing.
- **Session history** written to `~/Library/Application Support/Pomodoro/sessions.jsonl`
  as append-only JSON Lines. A round that reaches zero is recorded at its
  planned length; one you abandon is recorded as interrupted with the time it
  actually ran, unless it ran under a minute.
- **Statistics for today, this week and this month**, each opening a breakdown:
  today lists every session with its start and end time against your daily
  goal, the week draws a bar per day, the month a bar per week. An all-time
  summary carries totals, active days and your current streak.
- **Settings**, saved immediately to `config.json`: phase lengths with presets
  and a custom prompt, long-break interval, auto-start for breaks and for the
  next focus round, sound, notifications, daily goal, and a compact mode that
  drops the countdown digits for the dot alone.
- **Notifications and a sound** when a phase ends, naming what comes next. If
  macOS has notifications denied the menu says so and links to the right
  settings pane, rather than silently dropping them.
- **Start at Login** toggle.
- `make` targets for building, ad-hoc signing, installing to `/Applications`,
  and `make preview`, which dumps the whole menu — submenus expanded — to JSON
  without opening a GUI.

### Notes

- The timer is deadline-based rather than tick-counting, so it does not drift
  across a lid close. A round completed while the Mac slept records its planned
  length: sleep is not banked as work.
- Skipping a round does not count toward the long break.
- Changing a phase length never disturbs a phase already running; it applies
  from the next one.

<!-- Add compare links once this has a remote, e.g.
[Unreleased]: https://github.com/OWNER/pomodoro/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/OWNER/pomodoro/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/OWNER/pomodoro/releases/tag/v0.1.0
-->
