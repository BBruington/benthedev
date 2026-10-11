package bot

import (
	"sync"
	"time"
)

// Ported from the old Beny Bot: each Discord user gets a burst of 3 commands, then one more every 2 seconds.
// The old version wrapped golang.org/x/time/rate; this is the same token-bucket arithmetic without
// that module, with an injectable clock for tests and no package-level goroutine.
const (
	commandRefillInterval = 2 * time.Second
	commandBurst          = 3
	idleLimiterLifetime   = 5 * time.Minute
	limiterSweepInterval  = time.Minute
)

type userRateLimiter struct {
	mutex     sync.Mutex
	now       func() time.Time
	buckets   map[string]*tokenBucket
	lastSweep time.Time
}

type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

func newUserRateLimiter(now func() time.Time) *userRateLimiter {
	return &userRateLimiter{now: now, buckets: map[string]*tokenBucket{}, lastSweep: now()}
}

func (limiter *userRateLimiter) allow(discordUserID string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	current := limiter.now()
	limiter.sweep(current)
	bucket, found := limiter.buckets[discordUserID]
	if !found {
		bucket = &tokenBucket{tokens: commandBurst, lastSeen: current}
		limiter.buckets[discordUserID] = bucket
	}
	if elapsed := current.Sub(bucket.lastSeen); elapsed > 0 {
		bucket.tokens = min(commandBurst, bucket.tokens+float64(elapsed)/float64(commandRefillInterval))
	}
	bucket.lastSeen = current
	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

// sweep forgets users idle long enough that their bucket is full again, so memory stays bounded by
// the people who used Beny Bot in the last few minutes.
func (limiter *userRateLimiter) sweep(current time.Time) {
	if current.Sub(limiter.lastSweep) < limiterSweepInterval {
		return
	}
	limiter.lastSweep = current
	for discordUserID, bucket := range limiter.buckets {
		if current.Sub(bucket.lastSeen) > idleLimiterLifetime {
			delete(limiter.buckets, discordUserID)
		}
	}
}
