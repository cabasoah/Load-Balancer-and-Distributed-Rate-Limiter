package ratelimiter

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock starts exactly at the beginning of a 60s window.
type fakeClock struct{ t time.Time }

func newClock() *fakeClock                   { return &fakeClock{t: time.Unix(1_000_000_020, 0)} }
func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

// expectAllowed sends one request per entry in want and checks each result in order.
func expectAllowed(t *testing.T, l Limiter, key string, want ...bool) {
	t.Helper()
	for i, w := range want {
		if got := l.Allow(key); got != w {
			t.Fatalf("request %d for %q: Allow = %v, want %v", i+1, key, got, w)
		}
	}
}

func TestTokenBucket(t *testing.T) {
	clock := newClock()
	l := NewTokenBucketLimiter(3, 1)
	l.now = clock.Now

	expectAllowed(t, l, "a", true, true, true, false) // starts full
	expectAllowed(t, l, "b", true)                    // keys are independent

	clock.Advance(time.Second) // one token refilled
	expectAllowed(t, l, "a", true, false)

	clock.Advance(1500 * time.Millisecond) // 1.5 tokens: one usable
	expectAllowed(t, l, "a", true, false)

	clock.Advance(time.Hour) // refill caps at capacity
	expectAllowed(t, l, "a", true, true, true, false)
}

func TestFixedWindow(t *testing.T) {
	clock := newClock()
	l := NewFixedWindowLimiter(60*time.Second, 3)
	l.now = clock.Now

	expectAllowed(t, l, "a", true, true, true, false)
	expectAllowed(t, l, "b", true)

	clock.Advance(59 * time.Second) // still the same window
	expectAllowed(t, l, "a", false)

	clock.Advance(time.Second) // new window resets the count
	expectAllowed(t, l, "a", true, true, true, false)
}

// The known flaw of fixed windows: up to 2x the limit across a window edge.
func TestFixedWindowAllowsBurstAtBoundary(t *testing.T) {
	clock := newClock()
	l := NewFixedWindowLimiter(60*time.Second, 3)
	l.now = clock.Now

	clock.Advance(59 * time.Second)
	expectAllowed(t, l, "a", true, true, true)
	clock.Advance(time.Second)
	expectAllowed(t, l, "a", true, true, true) // 6 requests in about one second
}

func TestSlidingWindowCounter(t *testing.T) {
	clock := newClock()
	l := NewSlidingWindowCounterLimiter(60*time.Second, 4)
	l.now = clock.Now

	expectAllowed(t, l, "a", true, true, true, true, false)
	expectAllowed(t, l, "b", true)

	// start of next window: previous count still weighs fully (4*1.0)
	clock.Advance(60 * time.Second)
	expectAllowed(t, l, "a", false)

	// halfway: estimate 4*0.5 = 2, so two more fit
	clock.Advance(30 * time.Second)
	expectAllowed(t, l, "a", true, true, false)
}

// The sliding window counter rejects the boundary burst a fixed window allows.
func TestSlidingWindowCounterBlocksBoundaryBurst(t *testing.T) {
	clock := newClock()
	l := NewSlidingWindowCounterLimiter(60*time.Second, 3)
	l.now = clock.Now

	clock.Advance(59 * time.Second)
	expectAllowed(t, l, "a", true, true, true)
	clock.Advance(time.Second)
	expectAllowed(t, l, "a", false)
}

// Regression: a client idle for more than one window used to be blocked forever.
func TestSlidingWindowCounterResetsAfterLongIdle(t *testing.T) {
	clock := newClock()
	l := NewSlidingWindowCounterLimiter(60*time.Second, 3)
	l.now = clock.Now

	expectAllowed(t, l, "a", true, true, true, false)
	clock.Advance(180 * time.Second)
	expectAllowed(t, l, "a", true, true, true, false)
}

// Run with -race: the mutex must keep the count exact under concurrent requests.
func TestInMemoryLimitersAreConcurrencySafe(t *testing.T) {
	limiters := map[string]Limiter{
		"token-bucket":           NewTokenBucketLimiter(50, 0.001),
		"fixed-window":           NewFixedWindowLimiter(time.Hour, 50),
		"sliding-window-counter": NewSlidingWindowCounterLimiter(time.Hour, 50),
	}
	for name, l := range limiters {
		t.Run(name, func(t *testing.T) {
			var allowed atomic.Int64
			var wg sync.WaitGroup
			for range 200 {
				wg.Go(func() {
					if l.Allow("a") {
						allowed.Add(1)
					}
				})
			}
			wg.Wait()
			if got := allowed.Load(); got != 50 {
				t.Fatalf("allowed %d of 200 concurrent requests, want exactly 50", got)
			}
		})
	}
}
