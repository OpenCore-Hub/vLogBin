package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OpenCore-Hub/vLogBin/apps/api/internal/ratelimit"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	redisOnce   sync.Once
	redisAddr   string
	redisErrMsg string
)

// ensureRedis lazily starts a shared Redis container for the rate-limiter
// tests. It starts on first use so the rest of the suite (which exercises the
// default in-memory backend) stays untouched. The container lives for the
// whole test process and is reaped by testcontainers' Ryuk at exit.
func ensureRedis(t *testing.T) string {
	t.Helper()
	redisOnce.Do(func() {
		ctr, err := testcontainers.GenericContainer(testCtx, testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        "redis:7-alpine",
				ExposedPorts: []string{"6379/tcp"},
				WaitingFor:   wait.ForLog("Ready to accept connections"),
			},
			Started: true,
		})
		if err != nil {
			redisErrMsg = fmt.Sprintf("start redis container: %v", err)
			return
		}
		port, err := ctr.MappedPort(testCtx, "6379/tcp")
		if err != nil {
			redisErrMsg = fmt.Sprintf("redis port: %v", err)
			return
		}
		redisAddr = fmt.Sprintf("localhost:%s", port.Port())
	})
	if redisErrMsg != "" {
		t.Fatalf("redis setup: %s", redisErrMsg)
	}
	return redisAddr
}

func newRedisLimiter(t *testing.T, opts ratelimit.RedisLimiterOptions) *ratelimit.RedisLimiter {
	t.Helper()
	client := redis.NewClient(&redis.Options{Addr: ensureRedis(t)})
	t.Cleanup(func() { _ = client.Close() })
	return ratelimit.NewRedisLimiterWithClient(client, opts)
}

// TestRedisRateLimiterSharedAcrossInstances proves the counters are shared:
// two limiter instances backed by the same Redis observe a single window, so
// a limit of 5 is exhausted across instances, not per instance.
func TestRedisRateLimiterSharedAcrossInstances(t *testing.T) {
	rlA := newRedisLimiter(t, ratelimit.RedisLimiterOptions{})
	rlB := newRedisLimiter(t, ratelimit.RedisLimiterOptions{})

	const limit = 5
	window := time.Minute
	for i := 0; i < limit; i++ {
		if !rlA.Allow("shared-key", limit, window) {
			t.Fatalf("request %d on instance A should be allowed", i+1)
		}
	}
	if ok, _ := rlB.AllowRetryAfter("shared-key", limit, window); ok {
		t.Fatal("instance B must observe the count accumulated on instance A")
	}
}

// TestRedisRateLimiterRetryAfter verifies the retry duration equals the
// remaining window, like the in-memory limiter's Retry-After.
func TestRedisRateLimiterRetryAfter(t *testing.T) {
	rl := newRedisLimiter(t, ratelimit.RedisLimiterOptions{})

	if ok, _ := rl.AllowRetryAfter("retry-key", 1, 10*time.Second); !ok {
		t.Fatal("first request should be allowed")
	}
	ok, retry := rl.AllowRetryAfter("retry-key", 1, 10*time.Second)
	if ok {
		t.Fatal("second request should be denied")
	}
	if retry <= 0 || retry > 10*time.Second {
		t.Fatalf("retry %v out of expected (0, 10s]", retry)
	}
}

// TestRedisRateLimiterWindowExpiry verifies the window resets after it
// elapses, recovering capacity for the next window.
func TestRedisRateLimiterWindowExpiry(t *testing.T) {
	rl := newRedisLimiter(t, ratelimit.RedisLimiterOptions{})

	if !rl.Allow("expiry-key", 1, time.Second) {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow("expiry-key", 1, time.Second) {
		t.Fatal("second request should be denied")
	}
	time.Sleep(1100 * time.Millisecond)
	if !rl.Allow("expiry-key", 1, time.Second) {
		t.Fatal("capacity should recover after the window expires")
	}
}

// TestRedisRateLimiterPrefixIsolation verifies that different prefixes (e.g.
// separate applications sharing one Redis) never see each other's counters.
func TestRedisRateLimiterPrefixIsolation(t *testing.T) {
	rlA := newRedisLimiter(t, ratelimit.RedisLimiterOptions{Prefix: "app-a"})
	rlB := newRedisLimiter(t, ratelimit.RedisLimiterOptions{Prefix: "app-b"})

	if !rlA.Allow("same-key", 1, time.Minute) {
		t.Fatal("app-a first request should be allowed")
	}
	if rlA.Allow("same-key", 1, time.Minute) {
		t.Fatal("app-a second request should be denied")
	}
	if !rlB.Allow("same-key", 1, time.Minute) {
		t.Fatal("app-b must have an independent counter despite the same key")
	}
}

// TestRedisRateLimiterStartupFailsFast verifies that an unreachable Redis is
// reported at construction time, not silently at request time.
func TestRedisRateLimiterStartupFailsFast(t *testing.T) {
	_, err := ratelimit.NewRedisLimiter(testCtx, "127.0.0.1:1", "", 0, ratelimit.RedisLimiterOptions{})
	if err == nil {
		t.Fatal("expected a connection error for an unreachable Redis")
	}
}

// TestRedisRateLimiterFailsOpen verifies that a Redis failure at request time
// admits the request (fail open) and reports the error through OnError,
// instead of 429-ing the caller or crashing.
func TestRedisRateLimiterFailsOpen(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 500 * time.Millisecond})
	t.Cleanup(func() { _ = client.Close() })

	var reported error
	rl := ratelimit.NewRedisLimiterWithClient(client, ratelimit.RedisLimiterOptions{
		OnError: func(err error) { reported = err },
	})
	ok, retry := rl.AllowRetryAfter("k", 5, time.Minute)
	if !ok || retry != 0 {
		t.Fatalf("expected fail-open allow, got ok=%v retry=%v", ok, retry)
	}
	if reported == nil {
		t.Fatal("expected OnError to be invoked on backend failure")
	}
}
