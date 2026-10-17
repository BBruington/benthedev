package auth

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// sessionCacheLifetime bounds how long a lookup is reused. Every write that changes a member's status
// or sessions evicts explicitly, so the bound only matters for the daily expiry and last-seen touch.
const sessionCacheLifetime = 24 * time.Hour

// SessionCache keeps session lookups in memory, so an ordinary request or an SSE reconnect runs no
// session query (spec §8.5). It holds no role and no membership: status is per campaign and read by
// core.EnterCampaign. Core evicts through the core.SessionEvictor interface.
type SessionCache struct {
	mutex   sync.Mutex
	entries map[string]cachedSession
	// generation changes on every eviction. A lookup that started before an eviction must not store
	// what it read, or a signed-out session could stay cached for a day.
	generation uint64
}

type cachedSession struct {
	accountID     uuid.UUID
	discordUserID string
	viewAsPlayer  bool
	expiresAt     time.Time
	cachedAt      time.Time
}

var _ core.SessionEvictor = (*SessionCache)(nil)

func NewSessionCache() *SessionCache {
	return &SessionCache{entries: map[string]cachedSession{}}
}

func (cache *SessionCache) lookup(tokenHash []byte, now time.Time) (cachedSession, bool) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	entry, found := cache.entries[string(tokenHash)]
	if !found {
		return cachedSession{}, false
	}
	if now.Sub(entry.cachedAt) >= sessionCacheLifetime || !now.Before(entry.expiresAt) {
		delete(cache.entries, string(tokenHash))
		return cachedSession{}, false
	}
	return entry, true
}

func (cache *SessionCache) currentGeneration() uint64 {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	return cache.generation
}

// store keeps entry only if nothing was evicted since the caller read generation.
func (cache *SessionCache) store(tokenHash []byte, entry cachedSession, generation uint64) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if generation != cache.generation {
		return
	}
	for key, existing := range cache.entries {
		if entry.cachedAt.Sub(existing.cachedAt) >= sessionCacheLifetime || !entry.cachedAt.Before(existing.expiresAt) {
			delete(cache.entries, key)
		}
	}
	cache.entries[string(tokenHash)] = entry
}

func (cache *SessionCache) EvictSession(tokenHash []byte) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.generation++
	delete(cache.entries, string(tokenHash))
}

func (cache *SessionCache) EvictAccount(accountID uuid.UUID) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.generation++
	for key, entry := range cache.entries {
		if entry.accountID == accountID {
			delete(cache.entries, key)
		}
	}
}

func (cache *SessionCache) EvictAll() {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.generation++
	clear(cache.entries)
}

// Len counts cached sessions; tests use it.
func (cache *SessionCache) Len() int {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	return len(cache.entries)
}
