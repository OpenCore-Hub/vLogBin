package config

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// HotReloadableField identifies a config field that can be applied at runtime
// without a process restart. Everything else (database URL, ports, tokens)
// still requires a restart; the watcher only reports the subset below.
type HotReloadableField string

const (
	// FieldRateLimits maps to httpapi Server.SetRateLimits.
	FieldRateLimits HotReloadableField = "rate_limits"
	// FieldSlowRequestThreshold maps to Server.SetSlowRequestThreshold.
	FieldSlowRequestThreshold HotReloadableField = "slow_request_threshold"
	// FieldCORSOrigins maps to Server.SetCORSOrigins.
	FieldCORSOrigins HotReloadableField = "cors_allowed_origins"
	// FieldLogLevel maps to the slog.LevelVar used by the root logger.
	FieldLogLevel HotReloadableField = "log_level"
)

// Change describes which hot-reloadable fields changed during one reload and
// carries the full new config so the caller can apply the updated values.
type Change struct {
	Fields []HotReloadableField
	Config Config
}

// Empty reports whether no hot-reloadable field changed.
func (c Change) Empty() bool { return len(c.Fields) == 0 }

// Watcher periodically re-reads the environment (config.Load) and reports
// changes to hot-reloadable fields. It never mutates anything itself: the
// caller receives a Change and applies it (e.g. to the httpapi server and the
// logger). A watcher with a zero interval only reacts to the external
// Reload trigger (SIGHUP handler in main); the periodic path is skipped.
type Watcher struct {
	mu       sync.Mutex
	interval time.Duration
	log      *slog.Logger
	onChange func(Change)
	last     Config
}

// NewWatcher returns a watcher seeded with the initial config. interval is
// the polling period; zero disables periodic polling (signal-triggered
// reloads still work). onChange is invoked on every reload that changes at
// least one hot-reloadable field; it must not block (runs on the watcher
// goroutine).
func NewWatcher(interval time.Duration, log *slog.Logger, initial Config, onChange func(Change)) *Watcher {
	if log == nil {
		log = slog.Default()
	}
	if onChange == nil {
		onChange = func(Change) {}
	}
	return &Watcher{interval: interval, log: log, onChange: onChange, last: initial}
}

// Run polls until ctx is done. With a positive interval it reloads every
// interval; with a zero interval it blocks until ctx is done (a SIGHUP
// handler in main calls Reload directly on the same watcher).
func (w *Watcher) Run(ctx context.Context) {
	if w.interval <= 0 {
		<-ctx.Done()
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.Reload()
		}
	}
}

// Reload re-reads the environment, diffs the hot-reloadable fields against
// the previous snapshot and reports any change via onChange. It is safe to
// call concurrently with Run or from a SIGHUP handler: reloads are
// serialized by an internal mutex. A Load error is logged and leaves the
// previous config in place: misconfiguration must not take the server down
// at runtime.
func (w *Watcher) Reload() {
	cfg, err := Load()
	if err != nil {
		w.log.Error("config reload failed; keeping previous config",
			"error", err)
		return
	}
	w.mu.Lock()
	change := diffHotReloadable(w.last, cfg)
	if change.Empty() {
		w.mu.Unlock()
		return
	}
	w.last = cfg
	w.mu.Unlock()
	w.log.Info("config reload detected changes",
		"fields", change.Fields)
	// onChange runs outside the lock so an apply callback that re-enters
	// Reload (e.g. via a signal-triggered reload) cannot deadlock.
	w.onChange(change)
}

// diffHotReloadable returns the hot-reloadable fields that differ between
// old and new. Fields outside the hot-reloadable set are deliberately
// ignored even when they change (they require a restart).
func diffHotReloadable(old, new Config) Change {
	var fields []HotReloadableField
	if old.RateLimits != new.RateLimits {
		fields = append(fields, FieldRateLimits)
	}
	if old.SlowRequestThreshold != new.SlowRequestThreshold {
		fields = append(fields, FieldSlowRequestThreshold)
	}
	if !slices.Equal(old.CORSAllowedOrigins, new.CORSAllowedOrigins) {
		fields = append(fields, FieldCORSOrigins)
	}
	if old.LogLevel != new.LogLevel {
		fields = append(fields, FieldLogLevel)
	}
	return Change{Fields: fields, Config: new}
}
