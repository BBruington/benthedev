package bot

import (
	"testing"
	"time"
)

func TestUserRateLimiterAllowsABurstThenOneEveryTwoSeconds(t *testing.T) {
	clock := newFakeClock()
	limiter := newUserRateLimiter(clock.now)

	for attempt := range commandBurst {
		if !limiter.allow(activeUserID) {
			t.Fatalf("command %d of the burst was refused", attempt+1)
		}
	}
	if limiter.allow(activeUserID) {
		t.Fatal("the command after the burst was allowed")
	}
	clock.advance(commandRefillInterval / 2)
	if limiter.allow(activeUserID) {
		t.Fatal("a command one second later was allowed")
	}
	clock.advance(commandRefillInterval / 2)
	if !limiter.allow(activeUserID) {
		t.Fatal("a command two seconds later was refused")
	}
	if limiter.allow(activeUserID) {
		t.Fatal("two commands were allowed for one refill")
	}
}

func TestUserRateLimiterRefillsNoFurtherThanTheBurst(t *testing.T) {
	clock := newFakeClock()
	limiter := newUserRateLimiter(clock.now)
	limiter.allow(activeUserID)
	clock.advance(time.Hour)

	allowed := 0
	for range commandBurst + 2 {
		if limiter.allow(activeUserID) {
			allowed++
		}
	}
	if allowed != commandBurst {
		t.Fatalf("allowed %d commands after a long rest, want %d", allowed, commandBurst)
	}
}

func TestUserRateLimiterKeepsUsersApart(t *testing.T) {
	limiter := newUserRateLimiter(newFakeClock().now)
	for range commandBurst {
		limiter.allow(activeUserID)
	}
	if limiter.allow(activeUserID) {
		t.Fatal("the first user was not limited")
	}
	if !limiter.allow(dmUserID) {
		t.Fatal("the second user was limited by the first user's commands")
	}
}

func TestUserRateLimiterForgetsIdleUsers(t *testing.T) {
	clock := newFakeClock()
	limiter := newUserRateLimiter(clock.now)
	limiter.allow(activeUserID)
	clock.advance(idleLimiterLifetime + time.Second)
	limiter.allow(dmUserID)

	if _, remembered := limiter.buckets[activeUserID]; remembered {
		t.Error("an idle user is still remembered")
	}
	if _, remembered := limiter.buckets[dmUserID]; !remembered {
		t.Error("the current user is not remembered")
	}
}

func TestSlidingWindowLimiter(t *testing.T) {
	clock := newFakeClock()
	limiter := newSlidingWindowLimiter(2, time.Hour, clock.now)
	for attempt := range 2 {
		if !limiter.allow("campaign-a") {
			t.Fatalf("event %d was refused", attempt+1)
		}
	}
	if limiter.allow("campaign-a") {
		t.Fatal("a third event in the hour was allowed")
	}
	clock.advance(time.Hour - time.Nanosecond)
	if limiter.allow("campaign-a") {
		t.Fatal("an event just inside the hour was allowed")
	}
	clock.advance(time.Nanosecond)
	if !limiter.allow("campaign-a") {
		t.Fatal("an event an hour later was refused")
	}
}

func TestSlidingWindowLimiterKeepsKeysApartAndForgetsOldOnes(t *testing.T) {
	clock := newFakeClock()
	limiter := newSlidingWindowLimiter(1, time.Hour, clock.now)
	if !limiter.allow("campaign-a") || limiter.allow("campaign-a") {
		t.Fatal("campaign-a's allowance is wrong")
	}
	if !limiter.allow("campaign-b") {
		t.Fatal("campaign-a's events used up campaign-b's allowance")
	}
	clock.advance(time.Hour)
	limiter.allow("campaign-c")
	if _, remembered := limiter.sent["campaign-a"]; remembered {
		t.Error("a key whose hour has passed is still remembered")
	}
}
