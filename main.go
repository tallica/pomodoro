// Command pomodoro is a status-bar Pomodoro timer for macOS: it lives in the
// menu bar, counts down focus and break rounds, and keeps a session log it
// summarises by day, week and month.
package main

import (
	"log"
	"path/filepath"

	"github.com/caseymrm/menuet/v2"

	"github.com/tallica/pomodoro/internal/focusmode"
	"github.com/tallica/pomodoro/internal/pomodoro"
	"github.com/tallica/pomodoro/internal/ui"
)

// version is set via -ldflags "-X main.version=..." by the Makefile, from
// `git describe`. Left at its default for `go build`/`go run` without it.
var version = "dev"

func main() {
	log.SetFlags(0)

	dir, err := pomodoro.DataDir()
	if err != nil {
		log.Fatalf("pomodoro: cannot open data directory: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	logPath := filepath.Join(dir, "sessions.jsonl")

	cfg := pomodoro.LoadConfig(cfgPath)
	store, err := pomodoro.OpenStore(logPath)
	if err != nil {
		// A partially readable log still beats refusing to start: whatever
		// parsed is loaded, and new sessions append after it.
		log.Printf("pomodoro: session log %s: %v", logPath, err)
	}

	app := menuet.App()
	app.Name = "Pomodoro"
	app.Label = "pl.tallica.pomodoro"
	app.QuitLabel = "Quit Pomodoro"

	// The engine's callbacks fire only once the run loop is going, by which
	// point face is assigned.
	var face *ui.UI
	engine := pomodoro.NewEngine(cfg, pomodoro.Events{
		OnUpdate:   func() { face.Refresh() },
		OnFinished: func(f pomodoro.Finished) { face.OnFinished(f) },
		OnSession:  func(s pomodoro.Session) { face.OnSession(s) },
	})

	focus := focusmode.New()
	face = ui.New(app, engine, store, focus, dir, cfgPath, version)
	face.Install()

	wg, ctx := app.GracefulShutdownHandles()
	wg.Add(1)
	go func() {
		defer wg.Done()
		engine.Run(ctx)
	}()
	// Quitting mid-round must not leave the macOS Focus on.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		focus.Close()
	}()

	app.RunApplication()
}
