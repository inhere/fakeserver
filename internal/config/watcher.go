package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultPollInterval is how often the watcher re-stats the watched files as a
// fallback to fsnotify.
//
// fsnotify (inotify on Linux) is unreliable on network and virtual filesystems
// — 9p, drvfs, SMB, NFS and most FUSE mounts: a watch can be accepted and then
// deliver nothing, without any error, so hot reload silently stops working.
// Observed on the Windows drive of a WSL2 container (/d/...): the watcher tests
// get zero callbacks there while passing under /tmp, and a long-running
// fakeserver stopped reloading after edits — yet a freshly started instance did
// pick edits up. Polling a handful of small config files once a second costs
// nothing and does not depend on whether events arrive. Where fsnotify does
// work, the event path refreshes the poll baseline first, so the poller does
// not reload the same save a second time.
const DefaultPollInterval = time.Second

// Watcher monitors a set of config files and invokes onChange after a
// debounce window of quiet on the filesystem.
//
// design §5.2: hot reload must coalesce editor save bursts (Windows
// editors emit Write+Chmod or Rename+Create event pairs on a single save).
// The debounce window collapses those into a single onChange call. The same
// window also absorbs a change seen by both fsnotify and the poller.
//
// Path strategy: subscribe to the *parent directories* of each path
// (deduped) because single-file watches in inotify drop their subscription
// after rename-in-place. Event names are then filtered against the
// originally requested path set.
type Watcher struct {
	inner    *fsnotify.Watcher // nil when fsnotify is disabled (tests only)
	debounce time.Duration
	onChange func()
	onError  func(error)
	poll     time.Duration

	wanted map[string]struct{} // absolute paths the caller asked us to watch
	snaps  map[string]fileSnap // poll baseline; touched only by the loop goroutine

	timer    *time.Timer
	timerMu  sync.Mutex
	stop     chan struct{}
	stopOnce sync.Once
}

// fileSnap is what the poller compares between ticks.
type fileSnap struct {
	exists  bool
	size    int64
	modTime time.Time
	errMsg  string // last stat error, so the same one is not reported every tick
}

// WatcherOption configures NewWatcher.
type WatcherOption func(*watcherConfig)

type watcherConfig struct {
	poll     time.Duration
	onError  func(error)
	noNotify bool
}

// WithPollInterval overrides DefaultPollInterval. d <= 0 disables polling and
// leaves fsnotify as the only change source.
func WithPollInterval(d time.Duration) WatcherOption {
	return func(c *watcherConfig) { c.poll = d }
}

// WithErrorHandler receives fsnotify errors and unexpected stat errors.
// Without it they are dropped, which is how a dead watch used to go unnoticed.
func WithErrorHandler(fn func(error)) WatcherOption {
	return func(c *watcherConfig) { c.onError = fn }
}

// withoutNotify disables fsnotify so tests can exercise the polling path on its
// own, the way it runs on a filesystem that never delivers events.
func withoutNotify() WatcherOption {
	return func(c *watcherConfig) { c.noNotify = true }
}

// NewWatcher starts a background goroutine that watches paths and calls
// onChange after debounce of filesystem quiet. Changes are picked up from
// fsnotify and, as a fallback, by polling every DefaultPollInterval (see
// WithPollInterval). Stop the watcher with (*Watcher).Stop().
func NewWatcher(paths []string, debounce time.Duration, onChange func(), opts ...WatcherOption) (*Watcher, error) {
	cfg := watcherConfig{poll: DefaultPollInterval}
	for _, o := range opts {
		o(&cfg)
	}

	wanted := make(map[string]struct{}, len(paths))
	dirs := make(map[string]struct{})
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, fmt.Errorf("abs %q: %w", p, err)
		}
		wanted[abs] = struct{}{}
		dirs[filepath.Dir(abs)] = struct{}{}
	}

	w := &Watcher{
		debounce: debounce,
		onChange: onChange,
		onError:  cfg.onError,
		poll:     cfg.poll,
		wanted:   wanted,
		snaps:    make(map[string]fileSnap, len(wanted)),
		stop:     make(chan struct{}),
	}

	if !cfg.noNotify {
		inner, err := fsnotify.NewWatcher()
		if err != nil {
			return nil, fmt.Errorf("fsnotify.NewWatcher: %w", err)
		}
		for d := range dirs {
			if werr := inner.Add(d); werr != nil {
				inner.Close()
				return nil, fmt.Errorf("watch dir %q: %w", d, werr)
			}
		}
		w.inner = inner
	}

	// Baseline, so the first poll tick does not mistake the current state for
	// a change. A stat error present from the start is reported here: the
	// poller only reports errors that differ from the baseline, so without
	// this it would never surface.
	for p := range wanted {
		snap, err := statSnap(p)
		if err != nil {
			snap.errMsg = err.Error()
			w.report(fmt.Errorf("poll %s: %w", p, err))
		}
		w.snaps[p] = snap
	}

	go w.loop()
	return w, nil
}

func (w *Watcher) loop() {
	var (
		events <-chan fsnotify.Event
		errs   <-chan error
		tick   <-chan time.Time
	)
	if w.inner != nil {
		events, errs = w.inner.Events, w.inner.Errors
	}
	if w.poll > 0 {
		t := time.NewTicker(w.poll)
		defer t.Stop()
		tick = t.C
	}

	for {
		select {
		case <-w.stop:
			return
		case ev, ok := <-events:
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
			// fsnotify already saw this change; move the poll baseline so the
			// next tick does not report it again.
			if snap, err := statSnap(abs); err == nil {
				w.snaps[abs] = snap
			}
			w.armOrReset()
		case err, ok := <-errs:
			if !ok {
				return
			}
			w.report(fmt.Errorf("fsnotify: %w", err))
		case <-tick:
			w.pollOnce()
		}
	}
}

// pollOnce re-stats every watched file and arms the debounce timer if any of
// them changed since the last baseline.
func (w *Watcher) pollOnce() {
	changed := false
	for p := range w.wanted {
		prev := w.snaps[p]
		cur, err := statSnap(p)
		if err != nil {
			if err.Error() != prev.errMsg {
				w.report(fmt.Errorf("poll %s: %w", p, err))
			}
			w.snaps[p] = fileSnap{errMsg: err.Error()}
			continue
		}
		if cur.exists != prev.exists || cur.size != prev.size || !cur.modTime.Equal(prev.modTime) {
			changed = true
		}
		w.snaps[p] = cur
	}
	if changed {
		w.armOrReset()
	}
}

// statSnap returns the file's current snapshot. A missing file is a valid
// state (exists=false), not an error: editors that save by delete+create pass
// through it.
func statSnap(p string) (fileSnap, error) {
	fi, err := os.Stat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fileSnap{}, nil
		}
		return fileSnap{}, err
	}
	return fileSnap{exists: true, size: fi.Size(), modTime: fi.ModTime()}, nil
}

func (w *Watcher) report(err error) {
	if w.onError != nil {
		w.onError(err)
	}
}

// armOrReset arms the debounce timer or resets it if already armed.
//
// Note on Reset semantics: if the timer has already expired and its
// AfterFunc callback is mid-execution when Reset is called, the timer
// restarts but the in-flight callback completes — potentially producing
// a duplicate onChange near the edge of the debounce window. For our
// hot-reload use case this is benign (a duplicate reload is idempotent),
// but onChange implementations that have expensive side effects should
// be aware.
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
// fsnotify watcher. Safe to call multiple times (idempotent).
//
// Caveat: if an event armed the debounce timer just before Stop, the
// timer's onChange callback may still fire AFTER Stop returns. Callers
// that need a hard "no more callbacks" guarantee should serialize their
// own state after Stop, or check for a "stopped" flag inside onChange.
// time.Timer.Stop()'s return value indicates whether the timer was active
// but does NOT wait for an in-flight AfterFunc callback to finish.
func (w *Watcher) Stop() error {
	var err error
	w.stopOnce.Do(func() {
		close(w.stop)
		if w.inner != nil {
			err = w.inner.Close()
		}
		w.timerMu.Lock()
		if w.timer != nil {
			w.timer.Stop()
		}
		w.timerMu.Unlock()
	})
	return err
}
