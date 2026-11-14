package core

import "github.com/BBruington/colony-planner/internal/store"

// BridgeOutboxKind is the Postgres bridge_outbox_kind enum: what one outbox row tells Foundry.
type BridgeOutboxKind = store.BridgeOutboxKind

const (
	BridgeOutboxKindColonySummary  = store.BridgeOutboxKindCOLONYSUMMARY
	BridgeOutboxKindDaysAdvanced   = store.BridgeOutboxKindDAYSADVANCED
	BridgeOutboxKindNpcUpsert      = store.BridgeOutboxKindNPCUPSERT
	BridgeOutboxKindNpcWithdraw    = store.BridgeOutboxKindNPCWITHDRAW
	BridgeOutboxKindPlaceUpsert    = store.BridgeOutboxKindPLACEUPSERT
	BridgeOutboxKindPlaceWithdraw  = store.BridgeOutboxKindPLACEWITHDRAW
	BridgeOutboxKindQuestUpsert    = store.BridgeOutboxKindQUESTUPSERT
	BridgeOutboxKindQuestWithdraw  = store.BridgeOutboxKindQUESTWITHDRAW
	BridgeOutboxKindDelivery       = store.BridgeOutboxKindDELIVERY
	BridgeOutboxKindDeliveryUndone = store.BridgeOutboxKindDELIVERYUNDONE
	BridgeOutboxKindPollPosted     = store.BridgeOutboxKindPOLLPOSTED
	BridgeOutboxKindPollClosed     = store.BridgeOutboxKindPOLLCLOSED
)

// AllBridgeOutboxKindValues lists every outbox kind, for the API's enum schema.
func AllBridgeOutboxKindValues() []BridgeOutboxKind {
	return store.AllBridgeOutboxKindValues()
}

// BridgeEventKind is the Postgres bridge_event_kind enum: what one inbound event reports.
type BridgeEventKind = store.BridgeEventKind

const (
	BridgeEventKindStatus       = store.BridgeEventKindSTATUS
	BridgeEventKindTimeAdvanced = store.BridgeEventKindTIMEADVANCED
)

// AllBridgeEventKindValues lists every inbound kind, for the API's enum schema.
func AllBridgeEventKindValues() []BridgeEventKind {
	return store.AllBridgeEventKindValues()
}

// BridgeEventStatus is the Postgres bridge_event_status enum: a DM inbox item's decision.
type BridgeEventStatus = store.BridgeEventStatus

const (
	BridgeEventStatusPending   = store.BridgeEventStatusPENDING
	BridgeEventStatusAccepted  = store.BridgeEventStatusACCEPTED
	BridgeEventStatusDismissed = store.BridgeEventStatusDISMISSED
)

// AllBridgeEventStatusValues lists every status, for the API's enum schema.
func AllBridgeEventStatusValues() []BridgeEventStatus {
	return store.AllBridgeEventStatusValues()
}
