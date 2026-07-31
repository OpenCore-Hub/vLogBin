// Package ratelimit implements a fixed-window in-memory rate limiter for
// the 4-level rate limiting required by spec Section 7.3:
// Provider, Environment, Credential, and Endpoint.
//
// The limiter is designed for single-instance deployments. For
// multi-instance deployments, a Redis-backed limiter should be used.
package ratelimit

import (
	"sync"
	"sync/atomic"
	"time"
)

// bucket is a fixed-window counter. count is incremented on each request;
// when the window expires, the counter resets.
type bucket struct {
	count     atomic.Int64
	windowEnd atomic.Int64 // unix nanoseconds
}

// Limiter is a thread-safe fixed-window rate limiter. Each key gets its
// own bucket. Expired buckets are cleaned up periodically by the cleanup
// goroutine started with StartCleanup.
type Limiter struct {
	buckets sync.Map // map[string]*bucket
}

// New creates a new in-memory rate limiter.
func New() *Limiter {
	return &Limiter{}
}

// Allow returns true if the request is within the limit for the given key.
// limit is the maximum number of requests per window. window is the
// duration of the fixed window (e.g., 1 minute).
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	now := time.Now().UnixNano()
	windowEnd := now + window.Nanoseconds()

	b, _ := l.buckets.LoadOrStore(key, &bucket{})
	bkt := b.(*bucket)

	// Check if the window has expired.
	currentEnd := bkt.windowEnd.Load()
	if now >= currentEnd {
		// Try to reset the window. Use CAS to avoid racing with another
		// goroutine that might also be resetting.
		if bkt.windowEnd.CompareAndSwap(currentEnd, windowEnd) {
			bkt.count.Store(0)
		}
	}

	count := bkt.count.Add(1)
	return count <= int64(limit)
}

// StartCleanup launches a background goroutine that periodically removes
// expired buckets to prevent unbounded memory growth. Callers should
// cancel the context to stop the cleanup.
func (l *Limiter) StartCleanup(interval time.Duration, stop <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				now := time.Now().UnixNano()
				l.buckets.Range(func(key, val any) bool {
					b := val.(*bucket)
					if now > b.windowEnd.Load()+int64(time.Minute) {
						l.buckets.Delete(key)
					}
					return true
				})
			}
		}
	}()
}

// Stats returns the number of tracked buckets (for monitoring/debugging).
func (l *Limiter) Stats() int {
	count := 0
	l.buckets.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
