package core

import "github.com/BBruington/colony-planner/internal/store"

// MemberStatus is the Postgres member_status enum. Packages outside core use these names, so only core
// reaches into internal/store (spec §8.6).
type MemberStatus = store.MemberStatus

const (
	MemberStatusPending = store.MemberStatusPENDING
	MemberStatusActive  = store.MemberStatusACTIVE
	MemberStatusRemoved = store.MemberStatusREMOVED
)

// AllMemberStatusValues lists every status, for the API's enum schema.
func AllMemberStatusValues() []MemberStatus {
	return store.AllMemberStatusValues()
}

// RemovalKind says how a REMOVED membership got there, for the DM's /players: LEFT (the member left,
// decision C5), DENIED (turned away before they were ever let in) or REMOVED (taken out after being let
// in). It isn't stored: left_on_own and approved_at decide it, so the web never has to.
type RemovalKind string

const (
	RemovalKindLeft    RemovalKind = "LEFT"
	RemovalKindDenied  RemovalKind = "DENIED"
	RemovalKindRemoved RemovalKind = "REMOVED"
)

// AllRemovalKinds lists every kind, for the API's enum schema.
func AllRemovalKinds() []RemovalKind {
	return []RemovalKind{RemovalKindLeft, RemovalKindDenied, RemovalKindRemoved}
}

// removalKindOf is nil for a membership that isn't REMOVED. Re-admission sets approved_at, so someone
// denied, let back in and removed again reads REMOVED.
func removalKindOf(membership store.CampaignMember) *RemovalKind {
	if membership.Status != store.MemberStatusREMOVED {
		return nil
	}
	kind := RemovalKindRemoved
	switch {
	case membership.LeftOnOwn:
		kind = RemovalKindLeft
	case membership.ApprovedAt == nil:
		kind = RemovalKindDenied
	}
	return &kind
}
