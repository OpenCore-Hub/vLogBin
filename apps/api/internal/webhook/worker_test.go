package webhook

import (
	"testing"
	"time"
)

func TestNextRetryAt(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		attempts int
		wantMin  time.Duration
		wantMax  time.Duration
	}{
		{"attempt 1", 1, 2 * time.Second, 3 * time.Second},
		{"attempt 2", 2, 4 * time.Second, 5 * time.Second},
		{"attempt 3", 3, 8 * time.Second, 9 * time.Second},
		{"attempt 10 (capped)", 10, 5 * time.Minute, 5*time.Minute + time.Second},
		{"attempt 100 (capped)", 100, 5 * time.Minute, 5*time.Minute + time.Second},
		{"attempt 0 (edge, overflow → capped)", 0, 4 * time.Minute, 6 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextRetryAt(now, tt.attempts)
			delay := got.Sub(now)
			if delay < tt.wantMin || delay > tt.wantMax {
				t.Fatalf("delay = %v, want between %v and %v", delay, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestNextRetryAtOverflow(t *testing.T) {
	// Very large attempt count could cause overflow; verify it's capped.
	now := time.Now()
	got := nextRetryAt(now, 1000)
	delay := got.Sub(now)
	if delay > 6*time.Minute {
		t.Fatalf("delay = %v, should be capped at ~5min", delay)
	}
}

func TestIntToPgInt4(t *testing.T) {
	tests := []struct {
		name  string
		input int
		valid bool
		val   int32
	}{
		{"zero", 0, false, 0},
		{"positive", 200, true, 200},
		{"negative", -1, true, -1},
		{"large", 99999, true, 99999},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := intToPgInt4(tt.input)
			if got.Valid != tt.valid {
				t.Fatalf("Valid = %v, want %v", got.Valid, tt.valid)
			}
			if got.Valid && got.Int32 != tt.val {
				t.Fatalf("Int32 = %v, want %v", got.Int32, tt.val)
			}
		})
	}
}

func TestStringToPgText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
		val   string
	}{
		{"empty", "", false, ""},
		{"non-empty", "hello", true, "hello"},
		{"whitespace", " ", true, " "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringToPgText(tt.input)
			if got.Valid != tt.valid {
				t.Fatalf("Valid = %v, want %v", got.Valid, tt.valid)
			}
			if got.Valid && got.String != tt.val {
				t.Fatalf("String = %q, want %q", got.String, tt.val)
			}
		})
	}
}

func TestNewWorkerDefaults(t *testing.T) {
	w := NewWorker(nil, nil, 0)

	if w.interval != 5*time.Second {
		t.Fatalf("interval = %v, want 5s (default)", w.interval)
	}
	if w.maxAttempts != defaultMaxAttempts {
		t.Fatalf("maxAttempts = %v, want %v", w.maxAttempts, defaultMaxAttempts)
	}
	if w.httpClient == nil {
		t.Fatal("httpClient must not be nil")
	}
	if w.httpClient.Timeout != httpTimeout {
		t.Fatalf("httpClient.Timeout = %v, want %v", w.httpClient.Timeout, httpTimeout)
	}
	if w.urlValidator == nil {
		t.Fatal("urlValidator must not be nil")
	}
	if w.log == nil {
		t.Fatal("log must not be nil (should default to slog.Default())")
	}
}

func TestNewWorkerCustomInterval(t *testing.T) {
	w := NewWorker(nil, nil, 10*time.Second)
	if w.interval != 10*time.Second {
		t.Fatalf("interval = %v, want 10s", w.interval)
	}
}

func TestSetURLValidator(t *testing.T) {
	w := NewWorker(nil, nil, 0)
	called := false
	w.SetURLValidator(func(s string) error {
		called = true
		return nil
	})
	_ = w.urlValidator("test")
	if !called {
		t.Fatal("custom urlValidator was not called")
	}
}

func TestSetURLValidatorNil(t *testing.T) {
	w := NewWorker(nil, nil, 0)
	// Setting nil should not crash; the validator stays as the default.
	w.SetURLValidator(nil)
	if w.urlValidator == nil {
		t.Fatal("urlValidator should remain non-nil after SetURLValidator(nil)")
	}
}
