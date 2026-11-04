package core

import "github.com/BBruington/colony-planner/internal/store"

// MarketOrderStatus is the Postgres market_order_status enum. An UNPAID order is at sea until its
// arrival day and docked from then on.
type MarketOrderStatus = store.MarketOrderStatus

const (
	MarketOrderStatusUnpaid    = store.MarketOrderStatusUNPAID
	MarketOrderStatusPaid      = store.MarketOrderStatusPAID
	MarketOrderStatusCancelled = store.MarketOrderStatusCANCELLED
)

// AllMarketOrderStatusValues lists every status, for the API's enum schema.
func AllMarketOrderStatusValues() []MarketOrderStatus {
	return store.AllMarketOrderStatusValues()
}
