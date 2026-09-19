# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A macOS status-bar Pomodoro timer in Go. No window, no Dock icon — the whole UI
is an `NSStatusItem` and its menu, via [menuet](https://github.com/caseymrm/menuet)
(`github.com/caseymrm/menuet/v2`), which wraps AppKit through cgo.

## Commands

```sh
make check          # gofmt check + go vet + go test — run this before calling work done
make test           # tests only
make bundle         # build and ad-hoc sign Pomodoro.app
make run            # bundle, kill any running instance, launch
make install        # copy to /Applications
make preview        # dump the entire menu to menu-preview.json without a GUI
```

Single test: `go test ./internal/pomodoro -run TestLongBreakEveryFourthRound -v`
Race detector: `go test -race -count=2 ./...` (the engine is driven from a
ticker goroutine while the AppKit thread reads it, so races are real here).

### Seeing UI changes without a GUI

`make preview` is the main development loop for anything visual. menuet honors
`MENUET_SNAPSHOT_PATH`, which resolves `MenuState` plus every menu item with
submenus recursively expanded, writes it as JSON, and exits instead of entering
the AppKit run loop. Point `HOME` at a scratch directory with a seeded
`Library/Application Support/Pomodoro/sessions.jsonl` to render stats views
against known data:

```sh
HOME=/tmp/scratch MENUET_SNAPSHOT_PATH=/tmp/menu.json MENUET_SNAPSHOT_DELAY=1s \
  ./Pomodoro.app/Contents/MacOS/pomodoro
```

Note the snapshot JSON uses Go's default field names for `TextRun`
(`"Text"`, capitalized), not the lowercase `SnapshotItem` tags — both appear in
the same file.

## Architecture

Three layers, one direction of dependency: `ui` → `pomodoro` → nothing.

- `internal/pomodoro/timer.go` — the state machine. Owns all mutable timer state.
- `internal/pomodoro/store.go` — append-only session log and every statistic.
- `internal/pomodoro/config.go` — settings, load/save, bounds.
- `internal/ui/` — renders a snapshot of the above into menu items. Holds no
  timer state of its own.
- `main.go` — wiring only.

### The menu is a pure function of engine state

menuet's `Children func() []MenuItem` is re-invoked on every menu open and on
every `MenuChanged()`. So the UI never mutates and never caches: each call does
`engine.Snapshot()` plus `store.Range(...)` and builds items from scratch. To
make something appear in the menu, change engine or store state and call
`UI.Refresh()` — do not try to patch menu items in place.

`Engine.Snapshot()` returns a `View` value under lock; everything in `ui` reads
that, never the engine's fields.

### Event wiring and the lock invariant

`main.go` constructs the engine with callbacks that close over a `*ui.UI`
assigned *after* construction. This is safe only because callbacks first fire
from `Engine.Run`, started later. Preserve that ordering if you touch `main.go`.

**Every callback is invoked outside the engine mutex** (see `notify`, `session`,
`emit`). Handlers call back into the engine — `UI.Refresh` calls `Snapshot()` —
so firing one under the lock deadlocks. Any new event must follow the same
pattern: mutate under lock, release, then emit.

menuet already runs `Clicked` callbacks on their own goroutine, so blocking in
one (e.g. `App().Alert`, which is modal) is fine and does not stall the menu.

### Timer semantics worth preserving

These are deliberate and have tests; don't "simplify" them away.

- **Deadline-based, not tick-counting.** Running state is a `deadline`; idle and
  paused state is a `remaining`. Survives a lid close without drift.
- A phase that completes naturally records its **planned** length, not
  wall-clock elapsed — otherwise sleeping through a deadline banks hours of
  "focus". See `expire`.
- Only a focus round that ran to zero increments `focusDone` (the long-break
  cycle counter). Skipping must not earn a long break. `nextPhaseLocked` counts
  the in-flight round because it answers "what happens when this finishes?";
  `advanceLocked` uses `breakAfter(focusDone)` with the already-updated count.
  These two must not be conflated — doing so double-counts and fires the long
  break a round early.
- Abandoned focus under `minRecorded` (1 minute) is not logged — a misclick is
  not history.
- `SetConfig` leaves a running phase alone so the on-screen countdown never
  jumps; new lengths apply from the next phase.
- `Engine.Run` ticks at 200ms while running and 2s otherwise, to stay off the
  battery when idle.

`Engine.now` is a `func() time.Time` field so tests can inject a fake clock; use
it everywhere instead of calling `time.Now()` directly inside the engine.

### Storage

`~/Library/Application Support/Pomodoro/` holds `config.json` and
`sessions.jsonl`. The log is append-only JSON Lines, fsynced per write, and
unparseable lines are skipped on load — a corrupt tail must never stop the app
starting. All statistics derive from `Store.Range(from, to)`; add new views on
top of it rather than re-walking the slice.

Day/week/month boundaries use `StartOfDay`/`StartOfWeek`/`StartOfMonth`, which
build dates in the local zone. Never use `time.Truncate` for this — it operates
in UTC and lands on the wrong instant outside it. Weeks start Monday.

Config loaded from disk is clamped (`Config.clamp`) before use. `UI.apply`
mutates, hands it to `SetConfig` (which clamps), then saves *the clamped value
back*, so what is on disk is what is running.

## macOS constraints

- **Notifications require a signed bundle.** `make bundle` ad-hoc signs
  (`codesign -s -`). Running the bare binary gets no notifications. The UI
  checks `NotificationAuthorization()` once at startup and shows a menu row when
  denied, because `Notification()` silently no-ops in that state.
- The plist sets `LSUIElement`; menuet additionally sets
  `NSApplicationActivationPolicyAccessory`.
- **menuet appends its own separator, "Start at Login" and "Quit"** to the root
  menu. Do not add a Quit item.
- New status items land in the *leftmost* third-party slot, which on a notched
  MacBook with a busy menu bar means invisible. Position can be nudged with
  `defaults write com.tallica.pomodoro "NSStatusItem Preferred Position Item-0" -float <x>`.

### UI conventions

- **The status item and menu are monochrome.** Use only `LabelPrimary`,
  `LabelSecondary`, `LabelTertiary` — never `System*` hues, which wash out
  against the menu's translucent material and read badly in dark mode. State is
  carried by shape (solid dot focusing, hollow on a break) and weight (dimmed
  when idle or paused), never color. No emoji.
- **Do not use `Regular.Subtitle`.** It maps to `NSMenuItem.subtitle`, a plain
  string that drops run styling, and in practice renders as an empty band that
  still reserves height. Use a second `Regular` row instead.
- Menu-bar title runs and any aligned column use `Monospaced: true` so the item
  keeps a constant width instead of jittering each second. `format.go` holds the
  `clock`/`human`/`bar`/`pad` helpers — `clock` rounds *up* so a fresh 25-minute
  timer reads `25:00`.
