package focusmode

import (
	"errors"
	"reflect"
	"sync"
	"testing"
)

type runs struct {
	mu    sync.Mutex
	names []string
	err   error
}

func (r *runs) run(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	return r.err
}

func (r *runs) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.names...)
}

func TestRepeatedSetRunsOnce(t *testing.T) {
	r := &runs{}
	c := manualController(r.run)
	for range 5 {
		c.Set(true)
		c.apply()
	}
	c.Close()

	want := []string{OnShortcut, OffShortcut}
	if got := r.got(); !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %q, want %q", got, want)
	}
}

func TestCloseDoesNothingWhenNeverOn(t *testing.T) {
	r := &runs{}
	c := manualController(r.run)
	c.Set(false)
	c.Close()
	if got := r.got(); len(got) != 0 {
		t.Fatalf("runs = %q, want none", got)
	}
}

func TestFailedRunIsRetriedOnNextChange(t *testing.T) {
	r := &runs{err: errors.New("no such shortcut")}
	c := manualController(r.run)
	c.Set(true)
	c.apply()
	c.Close() // never turned on, so nothing to turn off

	r.mu.Lock()
	r.err = nil
	r.mu.Unlock()
	c.Set(true)
	c.apply()
	want := []string{OnShortcut, OnShortcut}
	if got := r.got(); !reflect.DeepEqual(got, want) {
		t.Fatalf("runs = %q, want %q", got, want)
	}
}

func TestMissingFrom(t *testing.T) {
	list := []byte("Toggle the audio\n" + OnShortcut + "\nSomething else\n")
	if got := missingFrom(list); !reflect.DeepEqual(got, []string{OffShortcut}) {
		t.Fatalf("missing = %q, want only %q", got, OffShortcut)
	}
}
