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
	// Discord sends an autocomplete request as a name is typed, so suggestions get their own, larger
	// allowance; sharing the commands' would leave none for the command itself.
	suggestionRefillInterval = 500 * time.Millisecond
	suggestionBurst          = 10
	idleLimiterLifetime      = 5 * time.Minute
	limiterSweepInterval     = time.Minute
)

type userRateLimiter struct {
	mutex          sync.Mutex
	now            func() time.Time
	burst          float64
	refillInterval time.Duration
	buckets        map[string]*tokenBucket
	lastSweep      time.Time
}

type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

func newUserRateLimiter(now func() time.Time, burst int, refillInterval time.Duration) *userRateLimiter {
	return &userRateLimiter{now: now, burst: float64(burst), refillInterval: refillInterval, buckets: map[string]*tokenBucket{}, lastSweep: now()}
}

func (limiter *userRateLimiter) allow(discordUserID string) bool {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	current := limiter.now()
	limiter.sweep(current)
	bucket, found := limiter.buckets[discordUserID]
	if !found {
		bucket = &tokenBucket{tokens: limiter.burst, lastSeen: current}
		limiter.buckets[discordUserID] = bucket
	}
	if elapsed := current.Sub(bucket.lastSeen); elapsed > 0 {
		bucket.tokens = min(limiter.burst, bucket.tokens+float64(elapsed)/float64(limiter.refillInterval))
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
