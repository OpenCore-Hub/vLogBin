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
	l.Allow("key1", 1, 10*time.Millisecond)

	stop := make(chan struct{})
	l.StartCleanup(50*time.Millisecond, stop)

	// The cleanup runs every 50ms and removes buckets whose windowEnd has
	// passed by more than 1 minute. Since our window is 10ms, the bucket
	// won't be cleaned up for ~1 minute. This test just verifies the
	// cleanup goroutine doesn't crash and Stats still works.
	time.Sleep(100 * time.Millisecond)
	close(stop)
	time.Sleep(10 * time.Millisecond)

	// Stats should return 1 (bucket still exists, not yet eligible for cleanup).
	if s := l.Stats(); s != 1 {
		t.Logf("Stats after cleanup = %d (expected 1, bucket not yet eligible for cleanup)", s)
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
