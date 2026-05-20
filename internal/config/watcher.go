package config

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a set of config files and invokes onChange after a
// debounce window of quiet on the filesystem.
//
// design §5.2: hot reload must coalesce editor save bursts (Windows
// editors emit Write+Chmod or Rename+Create event pairs on a single save).
// The debounce window collapses those into a single onChange call.
//
// Path strategy: subscribe to the *parent directories* of each path
// (deduped) because single-file watches in inotify drop their subscription
// after rename-in-place. Event names are then filtered against the
// originally requested path set.
type Watcher struct {
	inner    *fsnotify.Watcher
	debounce time.Duration
	onChange func()

	wanted   map[string]struct{} // absolute paths the caller asked us to watch
	timer    *time.Timer
	timerMu  sync.Mutex
	stop     chan struct{}
	stopOnce sync.Once
}

// NewWatcher starts a background goroutine that watches paths and calls
// onChange after debounce of filesystem quiet. Stop the watcher with
// (*Watcher).Stop().
func NewWatcher(paths []string, debounce time.Duration, onChange func()) (*Watcher, error) {
	inner, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("fsnotify.NewWatcher: %w", err)
	}

	wanted := make(map[string]struct{}, len(paths))
	dirs := make(map[string]struct{})
	for _, p := range paths {
		abs, aerr := filepath.Abs(p)
		if aerr != nil {
			inner.Close()
			return nil, fmt.Errorf("abs %q: %w", p, aerr)
		}
		wanted[abs] = struct{}{}
		dirs[filepath.Dir(abs)] = struct{}{}
	}
	for d := range dirs {
		if werr := inner.Add(d); werr != nil {
			inner.Close()
			return nil, fmt.Errorf("watch dir %q: %w", d, werr)
		}
	}

	w := &Watcher{
		inner:    inner,
		debounce: debounce,
		onChange: onChange,
		wanted:   wanted,
		stop:     make(chan struct{}),
	}
	go w.loop()
	return w, nil
}

func (w *Watcher) loop() {
	for {
		select {
		case <-w.stop:
			return
		case ev, ok := <-w.inner.Events:
			if !ok {
				return
			}
			abs, aerr := filepath.Abs(ev.Name)
			if aerr != nil {
				continue
			}
			if _, want := w.wanted[abs]; !want {
				continue
			}
			w.armOrReset()
		case _, ok := <-w.inner.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) armOrReset() {
	w.timerMu.Lock()
	defer w.timerMu.Unlock()
	if w.timer == nil {
		w.timer = time.AfterFunc(w.debounce, w.onChange)
		return
	}
	w.timer.Reset(w.debounce)
}

// Stop terminates the watcher goroutine and closes the underlying
// fsnotify watcher. Safe to call multiple times.
func (w *Watcher) Stop() error {
	var err error
	w.stopOnce.Do(func() {
		close(w.stop)
		err = w.inner.Close()
		w.timerMu.Lock()
		if w.timer != nil {
			w.timer.Stop()
		}
		w.timerMu.Unlock()
	})
	return err
}
