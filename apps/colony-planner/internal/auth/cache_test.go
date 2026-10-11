package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

func cachedFor(accountID uuid.UUID, now time.Time) cachedSession {
	return cachedSession{
		accountID: accountID,
		expiresAt: now.Add(core.SessionLifetime),
		cachedAt:  now,
	}
}

func TestSessionCacheEviction(t *testing.T) {
	cache := NewSessionCache()
	now := time.Now()
	member := uuid.New()
	otherMember := uuid.New()
	firstSession, secondSession, otherSession := []byte("first"), []byte("second"), []byte("other")

	cache.store(firstSession, cachedFor(member, now), cache.currentGeneration())
	cache.store(secondSession, cachedFor(member, now), cache.currentGeneration())
	cache.store(otherSession, cachedFor(otherMember, now), cache.currentGeneration())
	if cache.Len() != 3 {
		t.Fatalf("cached %d sessions, want 3", cache.Len())
	}

	cache.EvictSession(firstSession)
	if _, found := cache.lookup(firstSession, now); found {
		t.Fatal("EvictSession left the session cached")
	}
	if _, found := cache.lookup(secondSession, now); !found {
		t.Fatal("EvictSession dropped another session of the same member")
	}

	cache.EvictAccount(member)
	if _, found := cache.lookup(secondSession, now); found {
		t.Fatal("EvictAccount left one of the member's sessions cached")
	}
	if _, found := cache.lookup(otherSession, now); !found {
		t.Fatal("EvictAccount dropped another member's session")
	}
}

// A lookup that read the database before an eviction must not cache what it read: a member removed in
// between would otherwise keep their old access for a day.
func TestSessionCacheRefusesLookupsThatStartedBeforeAnEviction(t *testing.T) {
	cache := NewSessionCache()
	now := time.Now()
	member := uuid.New()
	session := []byte("session")

	generation := cache.currentGeneration()
	cache.EvictAccount(member)
	cache.store(session, cachedFor(member, now), generation)
	if _, found := cache.lookup(session, now); found {
		t.Fatal("a lookup that raced an eviction was cached")
	}
}

func TestSessionCacheExpiry(t *testing.T) {
	cache := NewSessionCache()
	now := time.Now()
	member := uuid.New()

	cache.store([]byte("stale"), cachedFor(member, now.Add(-sessionCacheLifetime)), cache.currentGeneration())
	if _, found := cache.lookup([]byte("stale"), now); found {
		t.Fatal("an entry older than the cache lifetime was served")
	}

	expiring := cachedFor(member, now)
	expiring.expiresAt = now.Add(time.Minute)
	cache.store([]byte("expiring"), expiring, cache.currentGeneration())
	if _, found := cache.lookup([]byte("expiring"), now.Add(2*time.Minute)); found {
		t.Fatal("an entry past its session's expiry was served")
	}
}
