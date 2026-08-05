package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// fakeEvaler records the script invocation and returns a canned result so the
// result-parsing and fail-open logic can be unit-tested without a live Redis.
// The Lua script itself is additionally exercised against a real Redis in the
// integration suite.
type fakeEvaler struct {
	res   any
	err   error
	keys  []string
	args  []any
	calls int
}

func (f *fakeEvaler) Eval(_ context.Context, _ string, keys []string, args ...any) *redis.Cmd {
	f.calls++
	f.keys = keys
	f.args = args
	cmd := redis.NewCmd(context.Background())
	if f.err != nil {
		cmd.SetErr(f.err)
	} else {
		cmd.SetVal(f.res)
	}
	return cmd
}

func (f *fakeEvaler) Ping(_ context.Context) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(context.Background())
	if f.err != nil {
		cmd.SetErr(f.err)
	} else {
		cmd.SetVal("PONG")
	}
	return cmd
}

func newFakeLimiter(f *fakeEvaler, onErr func(error)) *RedisLimiter {
	return &RedisLimiter{client: f, prefix: "ratelimit", onErr: onErr}
}

func TestRedisLimiterAllow(t *testing.T) {
	f := &fakeEvaler{res: []any{int64(1), int64(60)}}
	rl := newFakeLimiter(f, nil)

	if ok := rl.Allow("provider:abc", 5, time.Minute); !ok {
		t.Fatal("first request within limit should be allowed")
	}
	if f.calls != 1 {
		t.Fatalf("expected 1 eval call, got %d", f.calls)
	}
	if got := f.keys[0]; got != "ratelimit:provider:abc" {
		t.Fatalf("unexpected key %q", got)
	}
	if got := f.args[0]; got != int64(60) {
		t.Fatalf("unexpected window seconds %v", got)
	}
}

func TestRedisLimiterWindowFloor(t *testing.T) {
	// Sub-second windows are clamped to 1s so EXPIRE never deletes the key.
	f := &fakeEvaler{res: []any{int64(1), int64(1)}}
	rl := newFakeLimiter(f, nil)
	if ok := rl.Allow("k", 5, 100*time.Millisecond); !ok {
		t.Fatal("should be allowed")
	}
	if got := f.args[0]; got != int64(1) {
		t.Fatalf("expected window clamped to 1s, got %v", got)
	}
}

func TestRedisLimiterDeny(t *testing.T) {
	f := &fakeEvaler{res: []any{int64(6), int64(45)}}
	rl := newFakeLimiter(f, nil)

	ok, retry := rl.AllowRetryAfter("provider:abc", 5, time.Minute)
	if ok {
		t.Fatal("count above limit should be denied")
	}
	if retry != 45*time.Second {
		t.Fatalf("expected retry 45s, got %v", retry)
	}
}

func TestRedisLimiterRetryFloor(t *testing.T) {
	// A zero TTL (edge race) is floored to 1ms so Retry-After is never 0.
	f := &fakeEvaler{res: []any{int64(6), int64(0)}}
	rl := newFakeLimiter(f, nil)

	ok, retry := rl.AllowRetryAfter("k", 5, time.Minute)
	if ok || retry != time.Millisecond {
		t.Fatalf("expected denied with 1ms retry, got ok=%v retry=%v", ok, retry)
	}
}

func TestRedisLimiterPing(t *testing.T) {
	f := &fakeEvaler{res: "PONG"}
	rl := newFakeLimiter(f, nil)
	if err := rl.Ping(context.Background()); err != nil {
		t.Fatalf("Ping with healthy backend: %v", err)
	}

	f = &fakeEvaler{err: errors.New("connection refused")}
	rl = newFakeLimiter(f, nil)
	if err := rl.Ping(context.Background()); err == nil {
		t.Fatal("Ping must fail when Redis is unreachable")
	}
}

func TestLimiterPingAlwaysNil(t *testing.T) {
	l := New()
	if err := l.Ping(context.Background()); err != nil {
		t.Fatalf("in-memory limiter Ping = %v, want nil", err)
	}
}

func TestRedisLimiterFailsOpenOnError(t *testing.T) {
	backendErr := errors.New("connection refused")
	f := &fakeEvaler{err: backendErr}
	var reported error
	rl := newFakeLimiter(f, func(err error) { reported = err })

	ok, retry := rl.AllowRetryAfter("k", 5, time.Minute)
	if !ok || retry != 0 {
		t.Fatalf("expected fail-open allow, got ok=%v retry=%v", ok, retry)
	}
	if reported == nil {
		t.Fatal("expected OnError callback to be invoked")
	}
}

func TestRedisLimiterFailsOpenOnUnexpectedResult(t *testing.T) {
	cases := []any{
		"not-a-list",
		[]any{int64(6)},       // missing ttl
		[]any{"x", int64(45)}, // bad count type
		[]any{int64(6), "y"},  // bad ttl type
	}
	for _, res := range cases {
		f := &fakeEvaler{res: res}
		var reported error
		rl := newFakeLimiter(f, func(err error) { reported = err })

		ok, retry := rl.AllowRetryAfter("k", 5, time.Minute)
		if !ok || retry != 0 {
			t.Fatalf("result %v: expected fail-open allow, got ok=%v retry=%v", res, ok, retry)
		}
		if reported == nil {
			t.Fatalf("result %v: expected OnError callback", res)
		}
	}
}
