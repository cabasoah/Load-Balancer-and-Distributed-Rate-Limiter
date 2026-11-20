package ratelimiter

import (
	"sync"
	"time"
)

type FixedWindowLimiter struct {
	mu sync.Mutex
	counts map[string]int
	currentWindow map[string]int64
	windowSize time.Duration
	threshold int
	now func() time.Time // swapped out in tests
}

func NewFixedWindowLimiter(windowSize time.Duration, threshold int) *FixedWindowLimiter {
	return &FixedWindowLimiter{
		counts: make(map[string]int),
		currentWindow: make(map[string]int64),
		windowSize: windowSize,
		threshold: threshold,
		now: time.Now,
	}
}

func (l *FixedWindowLimiter) Allow(key string) bool  {
	l.mu.Lock()
	defer l.mu.Unlock()

	window := l.now().Unix() / int64(l.windowSize.Seconds())

	if l.currentWindow[key] != window {
		l.currentWindow[key] = window
		l.counts[key] = 0

	}

	if l.counts[key] >= l.threshold {
		return false
	}

	l.counts[key]++
	return true

}