// Package focusmode switches a macOS Focus on and off around focus rounds.
//
// macOS has no public API for an app to set a Focus, so this runs two
// shortcuts the user creates in the Shortcuts app, each holding a single
// "Set Focus" action. That also leaves the choice of which Focus to the user.
package focusmode

import (
	"bufio"
	"bytes"
	"log"
	"os/exec"
	"sync"
)

// The shortcuts the user creates. Their names are the whole contract.
const (
	OnShortcut  = "Pomodoro Focus On"
	OffShortcut = "Pomodoro Focus Off"
)

const shortcutsTool = "/usr/bin/shortcuts"

// Controller drives the Focus toward the most recently requested state. Set is
// cheap and never blocks, so it can be called on every UI refresh; the
// shortcuts themselves run on a background goroutine, one at a time, and only
// when the requested state actually changes.
type Controller struct {
	run func(name string) error

	mu   sync.Mutex
	want bool // what the timer asks for
	have bool // what the last successful shortcut run set

	runMu sync.Mutex // serializes shortcut runs
	kick  chan struct{}
}

// New starts a controller that runs shortcuts with the Shortcuts CLI.
func New() *Controller {
	return newController(runShortcut)
}

func newController(run func(string) error) *Controller {
	c := manualController(run)
	go func() {
		for range c.kick {
			c.apply()
		}
	}()
	return c
}

// Set requests the Focus on or off.
func (c *Controller) Set(on bool) {
	c.mu.Lock()
	changed := c.want != on
	c.want = on
	c.mu.Unlock()
	if changed {
		select {
		case c.kick <- struct{}{}:
		default: // a run is already pending and will read the latest want
		}
	}
}

// Close turns the Focus off if this controller turned it on, and waits for
// that to finish, so quitting mid-round does not leave it stuck on.
func (c *Controller) Close() {
	c.Set(false)
	c.apply()
}

func (c *Controller) apply() {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	c.mu.Lock()
	want, have := c.want, c.have
	c.mu.Unlock()
	if want == have {
		return
	}

	name := OffShortcut
	if want {
		name = OnShortcut
	}
	if err := c.run(name); err != nil {
		// Leave have as it was: the next change of want tries again, and a
		// missing shortcut is surfaced in the menu via Missing.
		log.Printf("pomodoro: shortcut %q: %v", name, err)
		return
	}
	c.mu.Lock()
	c.have = want
	c.mu.Unlock()
}

// manualController has no background loop: runs happen only on apply. Tests
// use it to stay deterministic.
func manualController(run func(string) error) *Controller {
	return &Controller{run: run, kick: make(chan struct{}, 1)}
}

func runShortcut(name string) error {
	return exec.Command(shortcutsTool, "run", name).Run()
}

// Missing lists which of the two shortcuts do not exist yet.
func Missing() ([]string, error) {
	out, err := exec.Command(shortcutsTool, "list").Output()
	if err != nil {
		return nil, err
	}
	return missingFrom(out), nil
}

func missingFrom(list []byte) []string {
	have := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(list))
	for sc.Scan() {
		have[sc.Text()] = true
	}
	var missing []string
	for _, name := range []string{OnShortcut, OffShortcut} {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing
}
