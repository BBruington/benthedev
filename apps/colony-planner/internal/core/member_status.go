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
