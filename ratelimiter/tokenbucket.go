package ratelimiter

import (
	"sync"
	"time"
)

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

type TokenBucketLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*tokenBucket
	capacity   float64
	refillRate float64 // token added per second
	now        func() time.Time // swapped out in tests
}

func NewTokenBucketLimiter(capacity, refillRate float64) *TokenBucketLimiter {
	return &TokenBucketLimiter{
		buckets:    make(map[string]*tokenBucket),
		capacity:   capacity,
		refillRate: refillRate,
		now:        time.Now,
	}
}

func (l *TokenBucketLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()

	b, exists := l.buckets[key]
	if !exists {
		b = &tokenBucket{tokens: l.capacity, lastRefill: now}
		l.buckets[key] = b
	}

	b.tokens = min(l.capacity, b.tokens+now.Sub(b.lastRefill).Seconds()*l.refillRate)
	b.lastRefill = now

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false

}
