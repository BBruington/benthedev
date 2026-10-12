package core

import "github.com/BBruington/colony-planner/internal/store"

// NpcStatus, NpcHealth and NpcRelation are the old app's NPC enums, kept as Postgres enums (spec §4.3)
// and re-exported so only core reaches into store. Marking an NPC DEAD changes no counts (spec §5.5).
type (
	NpcStatus   = store.NpcStatus
	NpcHealth   = store.NpcHealth
	NpcRelation = store.NpcRelation
)

const (
	NpcStatusAlive   = store.NpcStatusALIVE
	NpcStatusDead    = store.NpcStatusDEAD
	NpcStatusMissing = store.NpcStatusMISSING
	NpcStatusUnknown = store.NpcStatusUNKNOWN

	NpcHealthHealthy = store.NpcHealthHEALTHY
	NpcHealthInjured = store.NpcHealthINJURED
	NpcHealthSick    = store.NpcHealthSICK
	NpcHealthUnknown = store.NpcHealthUNKNOWN
	NpcHealthDead    = store.NpcHealthDEAD

	NpcRelationAlly       = store.NpcRelationALLY
	NpcRelationNeutral    = store.NpcRelationNEUTRAL
	NpcRelationSuspicious = store.NpcRelationSUSPICIOUS
	NpcRelationEnemy      = store.NpcRelationENEMY
	NpcRelationUnknown    = store.NpcRelationUNKNOWN
)

// AllNpcStatusValues lists every status, for the API's enum schema.
func AllNpcStatusValues() []NpcStatus { return store.AllNpcStatusValues() }

// AllNpcHealthValues lists every health, for the API's enum schema.
func AllNpcHealthValues() []NpcHealth { return store.AllNpcHealthValues() }

// AllNpcRelationValues lists every relation, for the API's enum schema.
func AllNpcRelationValues() []NpcRelation { return store.AllNpcRelationValues() }

// JoinMode is how a named NPC joins the colony (spec §5.5): as a new colonist (the total rises by 1)
// or by naming one of the role's unnamed colonists (the total is unchanged).
type JoinMode string

const (
	JoinModeNewColonist    JoinMode = "NEW_COLONIST"
	JoinModePromoteUnnamed JoinMode = "PROMOTE_UNNAMED"
)

// AllJoinModes lists every mode, for the API's enum schema.
func AllJoinModes() []JoinMode { return []JoinMode{JoinModeNewColonist, JoinModePromoteUnnamed} }

// LeaveMode is how a named colonist leaves (spec §5.5): removed from the colony (the total falls by 1)
// or returned to their role's unnamed pool (the total is unchanged).
type LeaveMode string

const (
	LeaveModeRemove       LeaveMode = "REMOVE"
	LeaveModeReturnToPool LeaveMode = "RETURN_TO_POOL"
)

// AllLeaveModes lists every mode, for the API's enum schema.
func AllLeaveModes() []LeaveMode { return []LeaveMode{LeaveModeRemove, LeaveModeReturnToPool} }
