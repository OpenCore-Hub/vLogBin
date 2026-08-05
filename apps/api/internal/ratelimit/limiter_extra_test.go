package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestLimiterStats(t *testing.T) {
	l := New()
	l.Allow("key1", 10, time.Minute)
	l.Allow("key2", 10, time.Minute)
	l.Allow("key3", 10, time.Minute)

	if s := l.Stats(); s != 3 {
		t.Fatalf("Stats = %d, want 3", s)
	}
}

func TestLimiterStatsEmpty(t *testing.T) {
	l := New()
	if s := l.Stats(); s != 0 {
		t.Fatalf("Stats = %d, want 0", s)
	}
}

func TestAllowRetryAfterZeroWhenAllowed(t *testing.T) {
	l := New()
	ok, retry := l.AllowRetryAfter("k", 2, time.Minute)
	if !ok {
		t.Fatal("call within limit must be allowed")
	}
	if retry != 0 {
		t.Fatalf("retry = %v, want 0 when allowed", retry)
	}
}

func TestAllowRetryAfterPositiveOnRejection(t *testing.T) {
	l := New()
	l.AllowRetryAfter("k", 1, time.Minute)
	ok, retry := l.AllowRetryAfter("k", 1, time.Minute)
	if ok {
		t.Fatal("second call within limit 1 must be rejected")
	}
	if retry <= 0 {
		t.Fatalf("retry = %v, want > 0", retry)
	}
	if retry > time.Minute {
		t.Fatalf("retry = %v, must not exceed window", retry)
	}
	// An unrelated key gets its own bucket.
	if ok, _ := l.AllowRetryAfter("other", 1, time.Minute); !ok {
		t.Fatal("unrelated key must be allowed")
	}
}

func TestAllowRetryAfterWindowReset(t *testing.T) {
	l := New()
	window := 10 * time.Millisecond
	l.AllowRetryAfter("k", 1, window)
	if ok, _ := l.AllowRetryAfter("k", 1, window); ok {
		t.Fatal("second call within window must be rejected")
	}
	time.Sleep(15 * time.Millisecond)
	if ok, _ := l.AllowRetryAfter("k", 1, window); !ok {
		t.Fatal("call after window expiry must be allowed")
	}
}

func TestAllowDelegatesToRetryAfter(t *testing.T) {
	l := New()
	l.Allow("k", 1, time.Minute)
	if l.Allow("k", 1, time.Minute) {
		t.Fatal("Allow must delegate to the shared bucket")
	}
}

func TestLimiterConcurrent(t *testing.T) {
	l := New()
	var wg sync.WaitGroup
	allowed := int32(0)
	denied := int32(0)
	var mu sync.Mutex

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("shared", 10, time.Minute) {
				mu.Lock()
				allowed++
				mu.Unlock()
			} else {
				mu.Lock()
				denied++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 10 {
		t.Fatalf("allowed = %d, want 10", allowed)
	}
	if denied != 90 {
		t.Fatalf("denied = %d, want 90", denied)
	}
}

func TestLimiterCleanup(t *testing.T) {
	l := New()
	l.Allow("key1", 1, time.Minute)
	l.Allow("key2", 1, time.Minute)

	if s := l.Stats(); s != 2 {
		t.Fatalf("initial Stats = %d, want 2", s)
	}

	// Force both buckets' windowEnd into the distant past so they become
	// eligible for cleanup (cleanup removes buckets whose windowEnd is
	// more than 1 minute in the past).
	past := time.Now().Add(-2 * time.Minute).UnixNano()
	l.buckets.Range(func(key, val any) bool {
		val.(*bucket).windowEnd.Store(past)
		return true
	})

	stop := make(chan struct{})
	l.StartCleanup(20*time.Millisecond, stop)

	// Wait for at least one cleanup tick.
	time.Sleep(80 * time.Millisecond)
	close(stop)

	if s := l.Stats(); s != 0 {
		t.Fatalf("Stats after cleanup = %d, want 0 (expired buckets should be removed)", s)
	}
}

func TestLimiterReusesBucket(t *testing.T) {
	l := New()
	l.Allow("key1", 5, time.Minute)
	l.Allow("key1", 5, time.Minute)

	// Only 1 bucket should exist.
	if s := l.Stats(); s != 1 {
		t.Fatalf("Stats = %d, want 1 (reused bucket)", s)
	}
}
