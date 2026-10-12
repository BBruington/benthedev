package core

import "github.com/BBruington/colony-planner/internal/store"

// QuestKind, QuestStatus and RequestStatus are Postgres enums (spec §4.3), re-exported so only core
// reaches into store. A quest's kind is a label with no logic (spec §5.6).
type (
	QuestKind     = store.QuestKind
	QuestStatus   = store.QuestStatus
	RequestStatus = store.RequestStatus
)

const (
	QuestKindMainland = store.QuestKindMAINLAND
	QuestKindColony   = store.QuestKindCOLONY

	QuestStatusActive    = store.QuestStatusACTIVE
	QuestStatusCompleted = store.QuestStatusCOMPLETED
	QuestStatusFailed    = store.QuestStatusFAILED

	RequestStatusPending   = store.RequestStatusPENDING
	RequestStatusApproved  = store.RequestStatusAPPROVED
	RequestStatusDeclined  = store.RequestStatusDECLINED
	RequestStatusWithdrawn = store.RequestStatusWITHDRAWN
)

// AllQuestKindValues lists every kind, for the API's enum schema.
func AllQuestKindValues() []QuestKind { return store.AllQuestKindValues() }

// AllQuestStatusValues lists every status, for the API's enum schema.
func AllQuestStatusValues() []QuestStatus { return store.AllQuestStatusValues() }

// AllRequestStatusValues lists every status, for the API's enum schema.
func AllRequestStatusValues() []RequestStatus { return store.AllRequestStatusValues() }

// RequestKind is what a request asks for. The column is text and this list is the one place a kind is
// written (spec §4.3); v1 has only PATRON_ACTION, and a future store adds its own.
type RequestKind string

const RequestKindPatronAction RequestKind = "PATRON_ACTION"

// AllRequestKinds lists every kind, for the API's enum schema.
func AllRequestKinds() []RequestKind { return []RequestKind{RequestKindPatronAction} }
