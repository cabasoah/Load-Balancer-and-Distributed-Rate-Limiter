package ratelimiter

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisClient connects to REDIS_ADDR (default localhost:6379). Without Redis
// the test is skipped locally but fails in CI, so CI can't silently skip it.
func redisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("redis not reachable at %s: %v", addr, err)
		}
		t.Skipf("redis not reachable at %s (run `docker compose up -d`): %v", addr, err)
	}
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func uniqueKey(t *testing.T) string {
	return fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
}

func TestRedisSlidingWindow(t *testing.T) {
	rdb := redisClient(t)
	clock := newClock()
	l := NewRedisSlidingWindowLimiter(rdb, 60*time.Second, 4)
	l.now = clock.Now
	key := uniqueKey(t)

	expectAllowed(t, l, key, true, true, true, true, false)

	clock.Advance(60 * time.Second) // previous window weighs fully
	expectAllowed(t, l, key, false)

	clock.Advance(30 * time.Second) // estimate 4*0.5 = 2
	expectAllowed(t, l, key, true, true, false)
}

// The point of the Redis version: two instances enforce one limit.
func TestRedisLimitIsSharedAcrossInstances(t *testing.T) {
	rdb := redisClient(t)
	clock := newClock()
	a := NewRedisSlidingWindowLimiter(rdb, 60*time.Second, 3)
	b := NewRedisSlidingWindowLimiter(rdb, 60*time.Second, 3)
	a.now, b.now = clock.Now, clock.Now
	key := uniqueKey(t)

	expectAllowed(t, a, key, true, true)
	expectAllowed(t, b, key, true, false) // b sees a's two requests
	expectAllowed(t, a, key, false)
}
