package core

import (
	"crypto/rand"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Link codes join a Discord server to a campaign (docs/design/multi-campaign.md §6.2). They live in
// memory, not in a table (single replica, Invariant 8): a restart forgets them and the DM gets a new
// one. One live code per campaign; a new one replaces it; each is used once.

// linkCodeAlphabet leaves out 0, 1, I, L and O, which read alike. Ten characters of it are about
// 49 bits.
const (
	linkCodeAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"
	linkCodeLength   = 10
	linkCodePrefix   = "BENY"
)

type linkCodeEntry struct {
	campaignID uuid.UUID
	expiresAt  time.Time
}

type linkCodeStore struct {
	mutex      sync.Mutex
	now        func() time.Time
	byCode     map[string]linkCodeEntry
	byCampaign map[uuid.UUID]string
}

func newLinkCodeStore(now func() time.Time) *linkCodeStore {
	if now == nil {
		now = time.Now
	}
	return &linkCodeStore{now: now, byCode: map[string]linkCodeEntry{}, byCampaign: map[uuid.UUID]string{}}
}

// issue makes a new code for the campaign, replacing its old one, and returns it with its expiry.
func (codes *linkCodeStore) issue(campaignID uuid.UUID) (string, time.Time, error) {
	var builder strings.Builder
	limit := big.NewInt(int64(len(linkCodeAlphabet)))
	for range linkCodeLength {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", time.Time{}, err
		}
		builder.WriteByte(linkCodeAlphabet[index.Int64()])
	}
	code := builder.String()
	codes.mutex.Lock()
	defer codes.mutex.Unlock()
	codes.dropLocked(campaignID)
	expiresAt := codes.now().UTC().Add(LinkCodeLifetime)
	codes.byCode[code] = linkCodeEntry{campaignID: campaignID, expiresAt: expiresAt}
	codes.byCampaign[campaignID] = code
	return code, expiresAt, nil
}

// take uses a code up: it answers the campaign and true once, while the code is live.
func (codes *linkCodeStore) take(typed string) (uuid.UUID, bool) {
	code := normalizeLinkCode(typed)
	codes.mutex.Lock()
	defer codes.mutex.Unlock()
	entry, found := codes.byCode[code]
	if !found {
		return uuid.Nil, false
	}
	codes.dropLocked(entry.campaignID)
	if !codes.now().UTC().Before(entry.expiresAt) {
		return uuid.Nil, false
	}
	return entry.campaignID, true
}

// drop forgets a campaign's live code: suspension and deletion.
func (codes *linkCodeStore) drop(campaignID uuid.UUID) {
	codes.mutex.Lock()
	defer codes.mutex.Unlock()
	codes.dropLocked(campaignID)
}

func (codes *linkCodeStore) dropLocked(campaignID uuid.UUID) {
	if code, found := codes.byCampaign[campaignID]; found {
		delete(codes.byCode, code)
		delete(codes.byCampaign, campaignID)
	}
}

// normalizeLinkCode accepts the code as shown (BENY-XXXXX-XXXXX), in any case and spacing.
func normalizeLinkCode(typed string) string {
	upper := strings.ToUpper(strings.TrimSpace(typed))
	upper = strings.TrimPrefix(upper, linkCodePrefix)
	return strings.NewReplacer("-", "", " ", "").Replace(upper)
}

// formatLinkCode shows a code as BENY-XXXXX-XXXXX.
func formatLinkCode(code string) string {
	return linkCodePrefix + "-" + code[:linkCodeLength/2] + "-" + code[linkCodeLength/2:]
}
