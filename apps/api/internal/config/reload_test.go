package config

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDiffHotReloadableNoChange(t *testing.T) {
	old := Config{RateLimits: RateLimitConfig{Provider: 1}, LogLevel: "info"}
	got := diffHotReloadable(old, old)
	if !got.Empty() {
		t.Fatalf("expected empty change, got %v", got.Fields)
	}
}

func TestDiffHotReloadableDetectsEachField(t *testing.T) {
	old := Config{
		RateLimits:           RateLimitConfig{Provider: 1, Window: time.Minute},
		SlowRequestThreshold: time.Second,
		CORSAllowedOrigins:   []string{"https://a.example"},
		LogLevel:             "info",
	}
	new := old
	new.RateLimits.Provider = 2
	got := diffHotReloadable(old, new)
	if len(got.Fields) != 1 || got.Fields[0] != FieldRateLimits {
		t.Fatalf("expected only rate_limits, got %v", got.Fields)
	}

	new = old
	new.SlowRequestThreshold = 2 * time.Second
	got = diffHotReloadable(old, new)
	if len(got.Fields) != 1 || got.Fields[0] != FieldSlowRequestThreshold {
		t.Fatalf("expected only slow_request_threshold, got %v", got.Fields)
	}

	new = old
	new.CORSAllowedOrigins = []string{"https://b.example"}
	got = diffHotReloadable(old, new)
	if len(got.Fields) != 1 || got.Fields[0] != FieldCORSOrigins {
		t.Fatalf("expected only cors_allowed_origins, got %v", got.Fields)
	}

	new = old
	new.LogLevel = "debug"
	got = diffHotReloadable(old, new)
	if len(got.Fields) != 1 || got.Fields[0] != FieldLogLevel {
		t.Fatalf("expected only log_level, got %v", got.Fields)
	}
}

func TestDiffHotReloadableMultiple(t *testing.T) {
	old := Config{
		RateLimits:           RateLimitConfig{Provider: 1},
		CORSAllowedOrigins:   []string{"https://a.example"},
		SlowRequestThreshold: time.Second,
		LogLevel:             "info",
	}
	new := old
	new.RateLimits.Provider = 9
	new.CORSAllowedOrigins = append(new.CORSAllowedOrigins, "https://b.example")
	new.LogLevel = "error"
	got := diffHotReloadable(old, new)
	expect := map[HotReloadableField]bool{
		FieldRateLimits: true, FieldCORSOrigins: true, FieldLogLevel: true,
	}
	if len(got.Fields) != 3 {
		t.Fatalf("expected 3 changed fields, got %v", got.Fields)
	}
	for _, f := range got.Fields {
		if !expect[f] {
			t.Fatalf("unexpected changed field %q", f)
		}
	}
	if got.Config.LogLevel != "error" {
		t.Fatalf("expected new config carried in Change, got %v", got.Config)
	}
}

func TestDiffHotReloadableIgnoresNonReloadable(t *testing.T) {
	old := Config{DatabaseURL: "postgres://old", OperatorToken: "old"}
	new := old
	new.DatabaseURL = "postgres://new"
	new.OperatorToken = "new"
	new.HTTPAddr = ":9999"
	got := diffHotReloadable(old, new)
	if !got.Empty() {
		t.Fatalf("non-hot-reloadable changes must be ignored, got %v", got.Fields)
	}
}

func TestWatcherReloadAppliesChange(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://w")
	t.Setenv("OPERATOR_TOKEN", "op")
	t.Setenv("RL_PROVIDER_LIMIT", "100")
	t.Setenv("LOG_LEVEL", "info")

	initial, err := Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	var applied *Change
	w := NewWatcher(0, testLogger(), initial, func(c Change) {
		cc := c
		applied = &cc
	})

	// No environment change yet: nothing to report.
	w.Reload()
	if applied != nil {
		t.Fatalf("unexpected change reported: %v", applied.Fields)
	}

	// Mutate a hot-reloadable field and reload.
	t.Setenv("RL_PROVIDER_LIMIT", "250")
	t.Setenv("LOG_LEVEL", "debug")
	w.Reload()
	if applied == nil {
		t.Fatal("expected a change after env mutation")
	}
	hasRate := false
	hasLog := false
	for _, f := range applied.Fields {
		switch f {
		case FieldRateLimits:
			hasRate = true
		case FieldLogLevel:
			hasLog = true
		}
	}
	if !hasRate || !hasLog {
		t.Fatalf("expected rate_limits and log_level, got %v", applied.Fields)
	}
	if applied.Config.RateLimits.Provider != 250 {
		t.Fatalf("expected provider limit 250, got %d", applied.Config.RateLimits.Provider)
	}
	if applied.Config.LogLevel != "debug" {
		t.Fatalf("expected log level debug, got %q", applied.Config.LogLevel)
	}
}

func TestWatcherReloadKeepsLastOnLoadError(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://w")
	t.Setenv("OPERATOR_TOKEN", "op")

	initial, err := Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	var applied []Change
	w := NewWatcher(0, testLogger(), initial, func(c Change) {
		applied = append(applied, c)
	})

	// Break the environment, then reload: the error must be swallowed and no
	// change reported.
	t.Setenv("RL_ENDPOINT_LIMIT", "not-a-number")
	w.Reload()
	if len(applied) != 0 {
		t.Fatalf("expected no change on load error, got %v", applied)
	}
}

func TestWatcherRunPeriodicAndCancel(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://w")
	t.Setenv("OPERATOR_TOKEN", "op")

	initial, err := Load()
	if err != nil {
		t.Fatalf("initial Load: %v", err)
	}

	done := make(chan struct{})
	w := NewWatcher(10*time.Millisecond, testLogger(), initial, func(Change) {
		close(done)
	})

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	// Change a hot-reloadable field after Run starts: the periodic tick must
	// pick it up.
	time.Sleep(5 * time.Millisecond)
	t.Setenv("LOG_LEVEL", "debug")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("periodic reload never fired")
	}
	cancel()
}
