package ratelimit

import (
	"testing"
	"time"
)

func TestLimiterAllow(t *testing.T) {
	limiter := New()

	// First 5 requests should be allowed.
	for i := 0; i < 5; i++ {
		if !limiter.Allow("key1", 5, time.Minute) {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	// 6th request should be denied.
	if limiter.Allow("key1", 5, time.Minute) {
		t.Fatal("request 6 should be denied (rate limit exceeded)")
	}
}

func TestLimiterDifferentKeys(t *testing.T) {
	limiter := New()

	// Key1 uses both slots.
	limiter.Allow("key1", 2, time.Minute)
	limiter.Allow("key1", 2, time.Minute)

	// Key1 is now blocked.
	if limiter.Allow("key1", 2, time.Minute) {
		t.Fatal("key1 should be blocked after 2 requests")
	}

	// Key2 still has capacity.
	if !limiter.Allow("key2", 2, time.Minute) {
		t.Fatal("key2 should be allowed (independent counter)")
	}
}

func TestLimiterWindowReset(t *testing.T) {
	limiter := New()

	// First request allowed.
	if !limiter.Allow("key1", 1, 50*time.Millisecond) {
		t.Fatal("first request should be allowed")
	}

	// Second request denied (window still active).
	if limiter.Allow("key1", 1, 50*time.Millisecond) {
		t.Fatal("second request should be denied within window")
	}

	// Wait for window to reset.
	time.Sleep(60 * time.Millisecond)

	// After window reset, request should be allowed again.
	if !limiter.Allow("key1", 1, 50*time.Millisecond) {
		t.Fatal("request after window reset should be allowed")
	}
}

func TestLimiterZeroLimit(t *testing.T) {
	limiter := New()

	// With limit=0, all requests should be denied.
	if limiter.Allow("key1", 0, time.Minute) {
		t.Fatal("request with limit=0 should be denied")
	}
}
