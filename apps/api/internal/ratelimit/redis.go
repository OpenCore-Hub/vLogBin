package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// fixedWindowScript atomically increments the counter for a key, opens the
// window (sets the key TTL) on the first request, and returns the new count
// and the remaining window in seconds. Running as a single Lua script keeps
// the read-modify-write atomic across every instance sharing the Redis, so
// the semantics match the in-memory Limiter exactly: the window starts at
// the first request for a key and ends `window` later.
const fixedWindowScript = `
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('TTL', KEYS[1])
if ttl < 0 then
    ttl = tonumber(ARGV[1])
end
return {count, ttl}
`

// RedisLimiterOptions configure a RedisLimiter.
type RedisLimiterOptions struct {
	// Prefix namespaces all keys so a shared Redis instance can serve
	// multiple applications without collisions. Default "ratelimit".
	Prefix string
	// OnError is invoked whenever a Redis call fails. The limiter fails
	// open on backend errors (the request is admitted), so a Redis outage
	// degrades distributed rate limiting to unthrottled instead of taking
	// the API down; operators observe the outage through this callback
	// (typically a metric increment plus a structured log line).
	OnError func(err error)
}

// RedisLimiter is a fixed-window rate limiter backed by Redis, used for
// multi-instance deployments where the in-memory Limiter's per-process
// counters would not be shared. Its Allow/AllowRetryAfter semantics match
// the in-memory Limiter (window starts at the first request for a key).
type RedisLimiter struct {
	client evalClient
	prefix string
	onErr  func(error)
}

// evalClient is the subset of *redis.Client that RedisLimiter needs. It is
// kept minimal so unit tests can substitute a fake, while integration tests
// exercise the real Lua script against a live Redis. Ping is used by /ready
// to surface rate-limit backend health.
type evalClient interface {
	Eval(ctx context.Context, script string, keys []string, args ...any) *redis.Cmd
	Ping(ctx context.Context) *redis.StatusCmd
}

// NewRedisLimiter connects to the Redis server at addr and returns a
// RedisLimiter ready for use. The connection is verified with a Ping so a
// misconfigured deployment fails fast at startup rather than degrading
// silently at request time.
func NewRedisLimiter(ctx context.Context, addr, password string, db int, opts RedisLimiterOptions) (*RedisLimiter, error) {
	client := redis.NewClient(&redis.Options{Addr: addr, Password: password, DB: db})
	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("connect redis %s: %w", addr, err)
	}
	return NewRedisLimiterWithClient(client, opts), nil
}

// Ping verifies the Redis connection with a round trip, used by /ready to
// surface rate-limit backend health. Note the limiter itself fails open at
// request time — a Redis outage degrades throttling silently — so this check
// is the only place operators see the degradation before it matters.
func (r *RedisLimiter) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// NewRedisLimiterWithClient wraps an existing *redis.Client (e.g. one shared
// with other components) in a RedisLimiter.
func NewRedisLimiterWithClient(client *redis.Client, opts RedisLimiterOptions) *RedisLimiter {
	prefix := opts.Prefix
	if prefix == "" {
		prefix = "ratelimit"
	}
	return &RedisLimiter{client: client, prefix: prefix, onErr: opts.OnError}
}

// Allow reports whether the request is within the limit for the given key.
func (r *RedisLimiter) Allow(key string, limit int, window time.Duration) bool {
	ok, _ := r.AllowRetryAfter(key, limit, window)
	return ok
}

// AllowRetryAfter reports whether the request is within the limit for the
// given key and, when rejected, the remaining window as the retry duration.
// Redis failures fail open: the request is admitted and onErr is invoked.
func (r *RedisLimiter) AllowRetryAfter(key string, limit int, window time.Duration) (bool, time.Duration) {
	windowSec := max(window/time.Second, 1) // EXPIRE 0 would delete the key
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := r.client.Eval(ctx, fixedWindowScript, []string{r.prefix + ":" + key}, int64(windowSec)).Result()
	if err != nil {
		r.report(fmt.Errorf("rate limiter backend: %w", err))
		return true, 0
	}
	vals, ok := res.([]any)
	if !ok || len(vals) < 2 {
		r.report(fmt.Errorf("rate limiter backend: unexpected eval result %v", res))
		return true, 0
	}
	count, ok := vals[0].(int64)
	if !ok {
		r.report(fmt.Errorf("rate limiter backend: unexpected count type %T", vals[0]))
		return true, 0
	}
	if count <= int64(limit) {
		return true, 0
	}
	ttl, ok := vals[1].(int64)
	if !ok {
		r.report(fmt.Errorf("rate limiter backend: unexpected ttl type %T", vals[1]))
		return true, 0
	}
	return false, max(time.Duration(ttl)*time.Second, time.Millisecond)
}

func (r *RedisLimiter) report(err error) {
	if r.onErr != nil {
		r.onErr(err)
	}
}

// Close releases the underlying Redis client. Fakes without a Close are
// left untouched.
func (r *RedisLimiter) Close() error {
	type closer interface{ Close() error }
	if c, ok := r.client.(closer); ok {
		return c.Close()
	}
	return nil
}
