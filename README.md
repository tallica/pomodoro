# Pomodoro

A native macOS Pomodoro timer that lives in the status bar. No window, no dock
icon — a countdown in the menu bar, a menu to drive it, and a session log it
summarises by day, week and month.

Written in Go, with the menu bar UI built on [menuet](https://github.com/caseymrm/menuet)
(AppKit `NSStatusItem` / `NSMenu` through cgo).

> [!WARNING]
> **AI-assisted project**
>
> This codebase was built with Claude Code. It works for the author's specific
> setup but has not been independently audited. Review the code before running
> it in any security-sensitive or production environment.

## Install

```sh
make install     # builds Pomodoro.app and copies it to /Applications
open /Applications/Pomodoro.app
```

Or just `make run` to build and launch it from the source directory.

To start it automatically, open the menu and tick **Start at Login**.

## Using it

The status item is a template image, so it takes the menu bar's own label
color — black on a light bar, white on a dark one or over a tinted wallpaper,
like the system icons beside it. Phase is carried by shape, not hue: a solid
tomato while you focus, an outline on a break, and a pause mark when paused.

| Menu bar | State |
| --- | --- |
| <picture><source media="(prefers-color-scheme: dark)" srcset="docs/status/focus-dark.png"><img src="docs/status/focus-light.png" height="24" alt="Solid tomato and countdown"></picture> | focusing |
| <picture><source media="(prefers-color-scheme: dark)" srcset="docs/status/focus-paused-dark.png"><img src="docs/status/focus-paused-light.png" height="24" alt="Solid tomato with a pause mark"></picture> | focus, paused |
| <picture><source media="(prefers-color-scheme: dark)" srcset="docs/status/break-dark.png"><img src="docs/status/break-light.png" height="24" alt="Outlined tomato and countdown"></picture> | on a break |
| <picture><source media="(prefers-color-scheme: dark)" srcset="docs/status/break-paused-dark.png"><img src="docs/status/break-paused-light.png" height="24" alt="Outlined tomato with a pause mark"></picture> | break, paused |
| <picture><source media="(prefers-color-scheme: dark)" srcset="docs/status/idle-dark.png"><img src="docs/status/idle-light.png" height="24" alt="Tomato and the number 3"></picture> | idle, three pomodoros done today |

Turn off **Show countdown in menu bar** and it shrinks to the tomato alone,
<picture><source media="(prefers-color-scheme: dark)" srcset="docs/status/icon-only-dark.png"><img src="docs/status/icon-only-light.png" height="24" alt="Tomato icon only"></picture>, which is the narrowest it gets — useful
on a crowded bar.

The menu holds everything else:

```
Focus  24:31 left
████████░░░░  33%  ·  round 2 of 4  ·  next: Short break
──────────────────────────────────────
Pause                              ⌃⌥Space
Skip to Short break                ⌃⌥S
Restart Focus
──────────────────────────────────────
Today         4 ·  1h 40m   ▸
This week    15 ·  6h 15m   ▸
This month   62 · 26h 12m   ▸
──────────────────────────────────────
Settings                     ▸
Start at Login
Quit Pomodoro
```

`⌃⌥Space` (start / pause) and `⌃⌥S` (skip) work system-wide, so you never have
to leave what you are doing to start a round.

Each of the three stat rows opens a breakdown: today lists every session with
its start and end time and tracks your daily goal; this week draws a bar per
day; this month draws a bar per week and holds an **All time** summary with
your streak and totals.

## Settings

Everything in the Settings submenu is saved immediately:

| Setting | Default | Notes |
| --- | --- | --- |
| Focus | 25 min | presets plus a **Custom…** prompt |
| Short break | 5 min | |
| Long break | 15 min | |
| Long break after | 4 rounds | only *completed* focus rounds count |
| Auto-start breaks | on | when a focus round runs out; **Skip** always starts the next phase |
| Auto-start next focus | off | when a break runs out |
| Play a sound | on | system sounds, no assets bundled |
| Show notifications | on | |
| Show countdown in menu bar | on | off shows just the tomato |
| Daily goal | 8 | drives the progress bar under Today |

Changing a length never disturbs a phase that is already running — it applies
from the next one.

## Where your data lives

```
~/Library/Application Support/Pomodoro/
├── config.json      settings
└── sessions.jsonl   one JSON object per session, append-only
```

The session log is plain JSON Lines, so it is easy to query yourself:

```sh
jq -s 'map(select(.kind=="focus" and .completed)) | length' \
  ~/Library/Application\ Support/Pomodoro/sessions.jsonl
```

Deleting the app leaves this folder alone; delete it by hand to start over.

### What counts as a session

- A focus round that reaches zero is recorded at its **planned** length. If the
  Mac sleeps through the deadline, the sleep is not banked as work.
- A focus round you abandon is recorded as interrupted, with the time it
  actually ran — unless it ran for under a minute, which is a misclick rather
  than history.
- Skipping a round does **not** count toward the long break.

## Changelog

See [CHANGELOG.md](CHANGELOG.md). The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## Development

```sh
make check     # gofmt, go vet, go test
make audit     # govulncheck: known vulnerabilities the code can reach
make preview   # dump the whole menu to menu-preview.json without opening a window
make bundle    # build and ad-hoc sign Pomodoro.app
```

```
main.go                      wiring
internal/pomodoro/config.go  settings, load/save, bounds
internal/pomodoro/timer.go   the state machine (deadline-based, sleep-safe)
internal/pomodoro/store.go   append-only session log and the stats over it
internal/ui/menu.go          status item and menu
internal/ui/settings.go      settings submenu
```

`make preview` is the quickest way to see a UI change: it renders the entire
menu, submenus expanded, as JSON — no GUI needed.

Notifications require a signed bundle, which `make bundle` handles with an
ad-hoc signature. To ship it to other machines, sign with a real identity:

```sh
make bundle IDENTITY="Developer ID Application: Your Name (TEAMID)"
```

The app reports it in the menu if macOS has notifications denied, rather than
silently dropping them.

## Licence

[MIT](LICENSE) © Michał Lipski
