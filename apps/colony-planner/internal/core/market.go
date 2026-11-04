package core

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// MarketItem is one catalog row: a supply offer (resourceKey and amount), a colonist offer (roleKey) or
// a special item (neither). An offer's name is its resource's or role's. The market has no secrets:
// archived rows, and offers on an archived resource or role, reach only the DM projection.
type MarketItem struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceGold   int64     `json:"priceGold" doc:"Gold per purchase"`
	ResourceKey *string   `json:"resourceKey" nullable:"true" doc:"The resource a supply offer sells; null otherwise"`
	Amount      *int64    `json:"amount" nullable:"true" doc:"How much of the resource one purchase brings; null unless a supply offer"`
	RoleKey     *string   `json:"roleKey" nullable:"true" doc:"The role a colonist offer's colonist joins; null otherwise"`
	MaxPerOrder *int64    `json:"maxPerOrder" nullable:"true" doc:"The most one order may carry; null is no limit"`
	Archived    bool      `json:"archived"`
	SortOrder   int64     `json:"sortOrder"`
}

// MarketCartLine is one item in the party's shared cart, at the item's current price. Quantity counts
// purchases: for a supply offer, lots of amount each.
type MarketCartLine struct {
	ItemID      uuid.UUID `json:"itemId" format:"uuid"`
	Name        string    `json:"name"`
	UnitPrice   int64     `json:"unitPrice"`
	Quantity    int64     `json:"quantity"`
	LineTotal   int64     `json:"lineTotal"`
	ResourceKey *string   `json:"resourceKey" nullable:"true"`
	Amount      *int64    `json:"amount" nullable:"true"`
	RoleKey     *string   `json:"roleKey" nullable:"true"`
}

type MarketCart struct {
	Lines     []MarketCartLine `json:"lines"`
	TotalGold int64            `json:"totalGold"`
	// UpdatedByName names whoever changed the cart last; null for an empty cart.
	UpdatedByName *string `json:"updatedByName" nullable:"true"`
}

// MarketOrderLine is one line of an order, snapshotted when it was placed.
type MarketOrderLine struct {
	ItemID    uuid.UUID        `json:"itemId" format:"uuid"`
	Name      string           `json:"name"`
	UnitPrice int64            `json:"unitPrice"`
	Quantity  int64            `json:"quantity"`
	Resources map[string]int64 `json:"resources"`
	Colonists int64            `json:"colonists"`
	RoleKey   *string          `json:"roleKey" nullable:"true"`
}

// MarketOrder is an open order: at sea until its arrival day, then docked until someone pays.
type MarketOrder struct {
	ID           uuid.UUID         `json:"id" format:"uuid"`
	Status       MarketOrderStatus `json:"status"`
	PlacedDay    int64             `json:"placedDay"`
	ArrivesDay   int64             `json:"arrivesDay"`
	Docked       bool              `json:"docked"`
	TotalGold    int64             `json:"totalGold"`
	Lines        []MarketOrderLine `json:"lines"`
	PlacedByName string            `json:"placedByName"`
	Summary      string            `json:"summary" doc:"e.g. 20 food, 3 Farmers, Boots of Striding"`
}

// MarketOrderSummary is one past order, paid or cancelled.
type MarketOrderSummary struct {
	ID           uuid.UUID         `json:"id" format:"uuid"`
	Status       MarketOrderStatus `json:"status"`
	PlacedDay    int64             `json:"placedDay"`
	ArrivesDay   int64             `json:"arrivesDay"`
	PaidDay      *int64            `json:"paidDay" nullable:"true"`
	CancelledDay *int64            `json:"cancelledDay" nullable:"true"`
	TotalGold    int64             `json:"totalGold"`
	Summary      string            `json:"summary"`
}

// MarketPage is /market: the catalog, the cart, the open orders (soonest to dock first) and the last 20
// past orders.
type MarketPage struct {
	Items      []MarketItem         `json:"items"`
	Cart       MarketCart           `json:"cart"`
	Orders     []MarketOrder        `json:"orders" doc:"The unpaid orders, soonest to dock first"`
	History    []MarketOrderSummary `json:"history"`
	VoyageDays int64                `json:"voyageDays"`
	ShipLimit  int64                `json:"shipLimit" doc:"How many orders may be out at once"`
	GoldOnHand int64                `json:"goldOnHand"`
}

// RoleChangePreview is one role's headcount line in a preview, e.g. "Farmer 4 → 7".
type RoleChangePreview struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Before int64  `json:"before"`
	After  int64  `json:"after"`
}

// MarketPayPreview is the Pay sheet's before → after, without writing. When gold is short it carries
// only the shortfall.
type MarketPayPreview struct {
	Affordable bool                    `json:"affordable"`
	Shortfall  string                  `json:"shortfall" doc:"Empty when the colony can pay"`
	Resources  []ResourceChangePreview `json:"resources"`
	Roles      []RoleChangePreview     `json:"roles"`
	Summary    string                  `json:"summary"`
	Version    int64                   `json:"version"`
}

// MarketItemInput is a special item's record; offers are priced through SetSupplyOffer and SetColonistOffer.
type MarketItemInput struct {
	Name        string `json:"name" doc:"1 to 80 characters"`
	Description string `json:"description" doc:"At most 4000 characters; may be blank"`
	PriceGold   int64  `json:"priceGold" doc:"Gold per unit, 0 or more"`
	MaxPerOrder *int64 `json:"maxPerOrder" required:"false" nullable:"true" doc:"The most one order may carry, 1 to 10000; null is no limit"`
}

type SupplyOfferInput struct {
	PriceGold int64 `json:"priceGold" doc:"Gold per purchase, 0 or more"`
	Amount    int64 `json:"amount" doc:"How much of the resource one purchase brings, 1 to 1000000"`
}

type ColonistOfferInput struct {
	PriceGold int64 `json:"priceGold" doc:"Gold per colonist, 0 or more"`
}

type MarketShippingInput struct {
	VoyageDays int64 `json:"voyageDays" doc:"Days at sea, 1 to 3650"`
	ShipLimit  int64 `json:"shipLimit" doc:"How many orders may be out at once, 1 to 100"`
}

type CartQuantityInput struct {
	Quantity int64 `json:"quantity" doc:"0 removes the line; otherwise 1 to 10000"`
}

type PlaceMarketOrderInput struct {
	// ExpectedTotalGold is the total the confirm sheet showed, so a cart someone changed meanwhile isn't ordered blind.
	ExpectedTotalGold int64 `json:"expectedTotalGold"`
}

type PayMarketOrderInput struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

// marketOrderBefore is a paid order's status before the payment, for the undo slot.
type marketOrderBefore struct {
	Status MarketOrderStatus `json:"status"`
}

const (
	marketItemNameMaxLength  = 80
	marketItemProseMaxLength = 4000
	marketMaxQuantity        = 10000
	marketMaxOfferAmount     = 1000000
	marketMaxVoyageDays      = 3650
	marketMaxShipLimit       = 100
	// defaultColonistPriceGold prices every role's colonist offer until the DM changes it.
	defaultColonistPriceGold = 30
)

var (
	errMarketItemNotFound  = NotFound("That item doesn't exist.")
	errMarketOrderNotFound = NotFound("That order doesn't exist.")
	errMarketNoResource    = NotFound("That resource doesn't exist.")
	errMarketNoRole        = NotFound("That role doesn't exist.")
	errMarketItemLimit     = Conflict("This campaign has " + formatNumber(MarketItemLimit) +
		" market items, the most it can hold. Archive one first.")
	errMarketItemTotalLimit = Conflict("This campaign has kept " + formatNumber(MarketItemLimitIncludingArchived) +
		" market items, archived ones included, the most it can hold.")
	errMarketCartChanged = Conflict("The cart changed. Review it and confirm again.")
	errMarketNotUnpaid   = Conflict("That order isn't waiting for payment.")
	errMarketNoGold      = Conflict("This colony has no gold to pay with.")
	errMarketCartEmpty   = Invalid("The cart is empty. Add something first.")
)

// shipsOutError refuses an order, or an undone payment's reopening, that would put more ships out
// than the DM allows.
func shipsOutError(shipLimit int32, action string) error {
	if shipLimit == 1 {
		return Conflict("The ship is still out. Pay for it at the docks or cancel it before " + action + ".")
	}
	return Conflict("All " + formatNumber(int64(shipLimit)) + " ships are out. Pay for one at the docks or cancel one before " + action + ".")
}

// GetMarket is /market as the viewer's projection sees it.
func (service *Service) GetMarket(requestContext context.Context, viewer Viewer) (MarketPage, error) {
	if err := requireActive(viewer); err != nil {
		return MarketPage{}, err
	}
	return readMarket(requestContext, service.queries, viewer)
}

func readMarket(requestContext context.Context, queries *store.Queries, viewer Viewer) (MarketPage, error) {
	colony, err := readColony(requestContext, queries, viewer.campaignID, false)
	if err != nil {
		return MarketPage{}, err
	}
	itemRows, err := queries.ListMarketItems(requestContext, store.ListMarketItemsParams{CampaignID: viewer.campaignID, DmProjection: viewer.SeesDmFields()})
	if err != nil {
		return MarketPage{}, err
	}
	sortMarketItems(colony, itemRows, func(row store.MarketItem) store.MarketItem { return row })
	page := MarketPage{Items: make([]MarketItem, 0, len(itemRows)), History: []MarketOrderSummary{}}
	for _, row := range itemRows {
		if viewer.SeesDmFields() || marketItemOnSale(colony, row) {
			page.Items = append(page.Items, marketItemFromRow(colony, row))
		}
	}
	if page.Cart, err = readCart(requestContext, queries, colony); err != nil {
		return MarketPage{}, err
	}
	settings, err := queries.GetMarketSettings(requestContext, viewer.campaignID)
	if err != nil {
		return MarketPage{}, campaignGone(err)
	}
	page.VoyageDays = int64(settings.MarketVoyageDays)
	page.ShipLimit = int64(settings.MarketShipLimit)
	page.GoldOnHand = colony.state.Resources[game.GoldResourceKey].Amount

	open, err := queries.ListOpenMarketOrders(requestContext, viewer.campaignID)
	if err != nil {
		return MarketPage{}, err
	}
	past, err := queries.ListRecentMarketOrders(requestContext, viewer.campaignID)
	if err != nil {
		return MarketPage{}, err
	}
	orderIDs := make([]uuid.UUID, 0, len(open)+len(past))
	for _, row := range open {
		orderIDs = append(orderIDs, row.MarketOrder.ID)
	}
	for _, order := range past {
		orderIDs = append(orderIDs, order.ID)
	}
	lines, err := listOrderLines(requestContext, queries, viewer.campaignID, orderIDs)
	if err != nil {
		return MarketPage{}, err
	}
	page.Orders = make([]MarketOrder, 0, len(open))
	for _, row := range open {
		order := row.MarketOrder
		orderLines := lines[order.ID]
		if orderLines == nil {
			orderLines = []MarketOrderLine{}
		}
		page.Orders = append(page.Orders, MarketOrder{
			ID:           order.ID,
			Status:       order.Status,
			PlacedDay:    int64(order.PlacedDay),
			ArrivesDay:   int64(order.ArrivesDay),
			Docked:       game.Docked(colony.state.Day, int64(order.ArrivesDay)),
			TotalGold:    int64(order.TotalGold),
			Lines:        orderLines,
			PlacedByName: memberName(row.PlacedByCharacterName, row.PlacedByDisplayName),
			Summary:      orderSummaryText(colony, orderLines),
		})
	}
	for _, order := range past {
		page.History = append(page.History, MarketOrderSummary{
			ID:           order.ID,
			Status:       order.Status,
			PlacedDay:    int64(order.PlacedDay),
			ArrivesDay:   int64(order.ArrivesDay),
			PaidDay:      int64Pointer(order.PaidDay),
			CancelledDay: int64Pointer(order.CancelledDay),
			TotalGold:    int64(order.TotalGold),
			Summary:      orderSummaryText(colony, lines[order.ID]),
		})
	}
	return page, nil
}

// readCart leaves out lines whose offer's resource or role is archived, for everyone: placing the order
// skips them too, so the cart shows exactly what would sail.
func readCart(requestContext context.Context, queries *store.Queries, colony colonySnapshot) (MarketCart, error) {
	rows, err := queries.ListCartLines(requestContext, colony.campaignID)
	if err != nil {
		return MarketCart{}, err
	}
	sortMarketItems(colony, rows, func(row store.ListCartLinesRow) store.MarketItem { return row.MarketItem })
	cart := MarketCart{Lines: make([]MarketCartLine, 0, len(rows))}
	var latest *store.ListCartLinesRow
	for index, row := range rows {
		if !marketItemOnSale(colony, row.MarketItem) {
			continue
		}
		lineTotal := int64(row.MarketItem.PriceGold) * int64(row.MarketCartLine.Quantity)
		cart.Lines = append(cart.Lines, MarketCartLine{
			ItemID:      row.MarketItem.ID,
			Name:        marketItemName(colony, row.MarketItem),
			UnitPrice:   int64(row.MarketItem.PriceGold),
			Quantity:    int64(row.MarketCartLine.Quantity),
			LineTotal:   lineTotal,
			ResourceKey: row.MarketItem.ResourceKey,
			Amount:      int64Pointer(row.MarketItem.Amount),
			RoleKey:     row.MarketItem.RoleKey,
		})
		cart.TotalGold += lineTotal
		if latest == nil || row.MarketCartLine.UpdatedAt.After(latest.MarketCartLine.UpdatedAt) {
			latest = &rows[index]
		}
	}
	if latest != nil {
		name := memberName(latest.UpdatedByCharacterName, latest.UpdatedByDisplayName)
		cart.UpdatedByName = &name
	}
	return cart, nil
}

func listOrderLines(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, orderIDs []uuid.UUID) (map[uuid.UUID][]MarketOrderLine, error) {
	lines := map[uuid.UUID][]MarketOrderLine{}
	if len(orderIDs) == 0 {
		return lines, nil
	}
	rows, err := queries.ListMarketOrderLines(requestContext, store.ListMarketOrderLinesParams{CampaignID: campaignID, OrderIds: orderIDs})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		resources, err := decodeGrants(row.Resources)
		if err != nil {
			return nil, err
		}
		lines[row.OrderID] = append(lines[row.OrderID], MarketOrderLine{
			ItemID:    row.ItemID,
			Name:      row.Name,
			UnitPrice: int64(row.UnitPrice),
			Quantity:  int64(row.Quantity),
			Resources: resources,
			Colonists: int64(row.Colonists),
			RoleKey:   row.RoleKey,
		})
	}
	return lines, nil
}

func marketItemFromRow(colony colonySnapshot, row store.MarketItem) MarketItem {
	return MarketItem{
		ID:          row.ID,
		Name:        marketItemName(colony, row),
		Description: row.Description,
		PriceGold:   int64(row.PriceGold),
		ResourceKey: row.ResourceKey,
		Amount:      int64Pointer(row.Amount),
		RoleKey:     row.RoleKey,
		MaxPerOrder: int64Pointer(row.MaxPerOrder),
		Archived:    row.Archived,
		SortOrder:   int64(row.SortOrder),
	}
}

// marketItemName is a special item's own name, or the resource's or role's label for an offer
// ("Colonist" for Unassigned), so a renamed role renames its offer.
func marketItemName(colony colonySnapshot, row store.MarketItem) string {
	switch {
	case row.ResourceKey != nil:
		return colony.labels.of(*row.ResourceKey)
	case row.RoleKey != nil && *row.RoleKey == game.UnassignedRoleKey:
		return "Colonist"
	case row.RoleKey != nil:
		if role, found := colony.roles[*row.RoleKey]; found {
			return role.Label
		}
		return *row.RoleKey
	}
	return row.Name
}

// marketItemOnSale is false for an offer whose resource or role is archived; players never see it.
func marketItemOnSale(colony colonySnapshot, row store.MarketItem) bool {
	switch {
	case row.ResourceKey != nil:
		resource, found := colony.resources[*row.ResourceKey]
		return found && !resource.Archived
	case row.RoleKey != nil:
		role, found := colony.roles[*row.RoleKey]
		return found && !role.Archived
	}
	return true
}

// sortMarketItems puts supply offers first in board order, then colonist offers in People's order,
// then special items as SQL listed them.
func sortMarketItems[Row any](colony colonySnapshot, rows []Row, itemOf func(Row) store.MarketItem) {
	resourcePositions := make(map[string]int, len(colony.order))
	for position, key := range colony.order {
		resourcePositions[key] = position
	}
	rank := func(row store.MarketItem) (int, int64) {
		switch {
		case row.ResourceKey != nil:
			return 0, int64(resourcePositions[*row.ResourceKey])
		case row.RoleKey != nil:
			return 1, int64(colony.roles[*row.RoleKey].SortOrder)
		}
		return 2, 0
	}
	slices.SortStableFunc(rows, func(left, right Row) int {
		leftGroup, leftPosition := rank(itemOf(left))
		rightGroup, rightPosition := rank(itemOf(right))
		return cmp.Or(cmp.Compare(leftGroup, rightGroup), cmp.Compare(leftPosition, rightPosition))
	})
}

func decodeGrants(encoded []byte) (map[string]int64, error) {
	grants := map[string]int64{}
	if len(encoded) > 0 {
		if err := json.Unmarshal(encoded, &grants); err != nil {
			return nil, err
		}
	}
	return grants, nil
}

// CreateMarketItem adds a special item: a record only, granting nothing.
func (service *Service) CreateMarketItem(requestContext context.Context, viewer Viewer, input MarketItemInput) (MarketItem, error) {
	if err := requireDm(viewer); err != nil {
		return MarketItem{}, err
	}
	cleaned, err := cleanMarketItemInput(input)
	if err != nil {
		return MarketItem{}, err
	}
	var item MarketItem
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		counts, err := queries.CountMarketItems(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if counts.Unarchived >= MarketItemLimit {
			return errMarketItemLimit
		}
		if counts.Total >= MarketItemLimitIncludingArchived {
			return errMarketItemTotalLimit
		}
		row, err := queries.CreateMarketItem(requestContext, store.CreateMarketItemParams{
			CampaignID:  viewer.campaignID,
			Name:        cleaned.Name,
			Description: cleaned.Description,
			PriceGold:   cleaned.PriceGold,
			MaxPerOrder: cleaned.MaxPerOrder,
		})
		if err != nil {
			return err
		}
		item = marketItemFromRow(colonySnapshot{}, row)
		return nil
	})
	if err != nil {
		return MarketItem{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket)
	return item, nil
}

// UpdateMarketItem replaces a special item's record; an offer's ID is the same 404 as a missing item,
// since offers are priced only through their own endpoints. Orders already placed keep their snapshot;
// cart lines over a new limit are trimmed to it.
func (service *Service) UpdateMarketItem(requestContext context.Context, viewer Viewer, itemID uuid.UUID, input MarketItemInput) (MarketItem, error) {
	if err := requireDm(viewer); err != nil {
		return MarketItem{}, err
	}
	cleaned, err := cleanMarketItemInput(input)
	if err != nil {
		return MarketItem{}, err
	}
	cleaned.CampaignID = viewer.campaignID
	cleaned.ID = itemID
	var item MarketItem
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		row, err := queries.UpdateMarketItem(requestContext, cleaned)
		if err != nil {
			return marketItemGone(err)
		}
		if row.MaxPerOrder != nil {
			if err := queries.ClampCartLine(requestContext, store.ClampCartLineParams{MaxPerOrder: *row.MaxPerOrder, CampaignID: viewer.campaignID, ItemID: itemID}); err != nil {
				return err
			}
		}
		item = marketItemFromRow(colonySnapshot{}, row)
		return nil
	})
	if err != nil {
		return MarketItem{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket)
	return item, nil
}

// SetMarketItemArchived withdraws any item, an offer included (or brings one back); items are never
// deleted. Archiving takes the item out of the cart too.
func (service *Service) SetMarketItemArchived(requestContext context.Context, viewer Viewer, itemID uuid.UUID, archived bool) (MarketItem, error) {
	if err := requireDm(viewer); err != nil {
		return MarketItem{}, err
	}
	var item MarketItem
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := readColony(requestContext, queries, viewer.campaignID, true)
		if err != nil {
			return err
		}
		current, err := queries.GetMarketItem(requestContext, store.GetMarketItemParams{CampaignID: viewer.campaignID, ID: itemID})
		if err != nil {
			return marketItemGone(err)
		}
		isSpecial := current.ResourceKey == nil && current.RoleKey == nil
		if !archived && current.Archived && isSpecial {
			counts, err := queries.CountMarketItems(requestContext, viewer.campaignID)
			if err != nil {
				return err
			}
			if counts.Unarchived >= MarketItemLimit {
				return errMarketItemLimit
			}
		}
		row, err := queries.SetMarketItemArchived(requestContext, store.SetMarketItemArchivedParams{CampaignID: viewer.campaignID, ID: itemID, Archived: archived})
		if err != nil {
			return marketItemGone(err)
		}
		if archived {
			if err := queries.DeleteCartLine(requestContext, store.DeleteCartLineParams{CampaignID: viewer.campaignID, ItemID: itemID}); err != nil {
				return err
			}
		}
		item = marketItemFromRow(colony, row)
		return nil
	})
	if err != nil {
		return MarketItem{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket)
	return item, nil
}

func cleanMarketItemInput(input MarketItemInput) (store.UpdateMarketItemParams, error) {
	var fieldErrors []FieldError
	collect := func(fieldError *FieldError) {
		if fieldError != nil {
			fieldErrors = append(fieldErrors, *fieldError)
		}
	}
	var parameters store.UpdateMarketItemParams
	var fieldError *FieldError
	parameters.Name, fieldError = cleanText("name", input.Name, marketItemNameMaxLength)
	collect(fieldError)
	if fieldError == nil && parameters.Name == "" {
		collect(&FieldError{Field: "name", Message: "Give the item a name."})
	}
	parameters.Description, fieldError = cleanMultilineText("description", input.Description, marketItemProseMaxLength)
	collect(fieldError)
	collect(checkMarketPrice(input.PriceGold))
	if input.MaxPerOrder != nil && (*input.MaxPerOrder < 1 || *input.MaxPerOrder > marketMaxQuantity) {
		collect(&FieldError{Field: "maxPerOrder", Message: "Use a limit from 1 to 10,000, or leave it blank."})
	}
	if len(fieldErrors) > 0 {
		return store.UpdateMarketItemParams{}, Invalid("Check the item's details.", fieldErrors...)
	}
	parameters.PriceGold = int32(input.PriceGold)
	if input.MaxPerOrder != nil {
		parameters.MaxPerOrder = int32Pointer(*input.MaxPerOrder)
	}
	return parameters, nil
}

func checkMarketPrice(priceGold int64) *FieldError {
	if priceGold < 0 || priceGold > game.MaxStored {
		return &FieldError{Field: "priceGold", Message: "Use a whole number of gold from 0 up."}
	}
	return nil
}

// SetSupplyOffer prices one resource's standing offer ("2 food for 1 gp"), creating it or putting a
// stopped one back on sale.
func (service *Service) SetSupplyOffer(requestContext context.Context, viewer Viewer, resourceKey string, input SupplyOfferInput) (MarketPage, error) {
	if err := requireDm(viewer); err != nil {
		return MarketPage{}, err
	}
	var fieldErrors []FieldError
	if fieldError := checkMarketPrice(input.PriceGold); fieldError != nil {
		fieldErrors = append(fieldErrors, *fieldError)
	}
	if input.Amount < 1 || input.Amount > marketMaxOfferAmount {
		fieldErrors = append(fieldErrors, FieldError{Field: "amount", Message: "Use an amount from 1 to 1,000,000."})
	}
	if len(fieldErrors) > 0 {
		return MarketPage{}, Invalid("Check the offer's details.", fieldErrors...)
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := readColony(requestContext, queries, viewer.campaignID, true)
		if err != nil {
			return err
		}
		resource, found := colony.resources[resourceKey]
		switch {
		case !found:
			return errMarketNoResource
		case resourceKey == game.GoldResourceKey || resourceKey == game.MoraleResourceKey:
			return Invalid(resource.Label + " can't be bought at the market.")
		case resource.Archived:
			return Conflict(resource.Label + " is archived.")
		}
		_, err = queries.UpsertSupplyOffer(requestContext, store.UpsertSupplyOfferParams{
			CampaignID:  viewer.campaignID,
			ResourceKey: &resourceKey,
			Amount:      int32Pointer(input.Amount),
			PriceGold:   int32(input.PriceGold),
		})
		return err
	})
	if err != nil {
		return MarketPage{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket)
	return readMarket(requestContext, service.queries, viewer)
}

// SetColonistOffer prices one role's standing offer: one colonist of that role per purchase.
func (service *Service) SetColonistOffer(requestContext context.Context, viewer Viewer, roleKey string, input ColonistOfferInput) (MarketPage, error) {
	if err := requireDm(viewer); err != nil {
		return MarketPage{}, err
	}
	if fieldError := checkMarketPrice(input.PriceGold); fieldError != nil {
		return MarketPage{}, Invalid("Check the offer's details.", *fieldError)
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := readColony(requestContext, queries, viewer.campaignID, true)
		if err != nil {
			return err
		}
		role, found := colony.roles[roleKey]
		if !found {
			return errMarketNoRole
		}
		if role.Archived {
			return Conflict(role.Label + " is archived.")
		}
		_, err = queries.UpsertColonistOffer(requestContext, store.UpsertColonistOfferParams{
			CampaignID: viewer.campaignID,
			RoleKey:    &roleKey,
			PriceGold:  int32(input.PriceGold),
		})
		return err
	})
	if err != nil {
		return MarketPage{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket)
	return readMarket(requestContext, service.queries, viewer)
}

// createDefaultMarketOffers gives a new campaign its staple supply offers and a colonist offer for every role.
func createDefaultMarketOffers(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) error {
	if err := queries.CreateDefaultSupplyOffers(requestContext, campaignID); err != nil {
		return err
	}
	return queries.CreateColonistOffers(requestContext, store.CreateColonistOffersParams{CampaignID: campaignID, PriceGold: defaultColonistPriceGold})
}

// createColonistOffer puts a new role's colonists on sale at the default price.
func createColonistOffer(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, roleKey string) error {
	_, err := queries.UpsertColonistOffer(requestContext, store.UpsertColonistOfferParams{CampaignID: campaignID, RoleKey: &roleKey, PriceGold: defaultColonistPriceGold})
	return err
}

func marketItemGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errMarketItemNotFound
	}
	return err
}

func marketOrderGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return errMarketOrderNotFound
	}
	return err
}

// SetMarketShipping sets how many days an order is at sea and how many may be out at once. Orders
// already placed keep their day, and a lower limit leaves the ships already out sailing.
func (service *Service) SetMarketShipping(requestContext context.Context, viewer Viewer, input MarketShippingInput) (MarketPage, error) {
	if err := requireDm(viewer); err != nil {
		return MarketPage{}, err
	}
	fieldErrors := []FieldError{}
	if input.VoyageDays < 1 || input.VoyageDays > marketMaxVoyageDays {
		fieldErrors = append(fieldErrors, FieldError{Field: "voyageDays", Message: "Use a voyage from 1 to 3,650 days."})
	}
	if input.ShipLimit < 1 || input.ShipLimit > marketMaxShipLimit {
		fieldErrors = append(fieldErrors, FieldError{Field: "shipLimit", Message: "Use from 1 to 100 ships."})
	}
	if len(fieldErrors) > 0 {
		return MarketPage{}, Invalid(fieldErrors[0].Message, fieldErrors...)
	}
	if err := service.queries.SetMarketSettings(requestContext, store.SetMarketSettingsParams{
		CampaignID:       viewer.campaignID,
		MarketVoyageDays: int32(input.VoyageDays),
		MarketShipLimit:  int32(input.ShipLimit),
	}); err != nil {
		return MarketPage{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket)
	return readMarket(requestContext, service.queries, viewer)
}

// SetCartQuantity sets one line of the party's shared cart; 0 removes it. The cart is no colony
// change: no lock, no timeline entry.
func (service *Service) SetCartQuantity(requestContext context.Context, viewer Viewer, itemID uuid.UUID, input CartQuantityInput) (MarketPage, error) {
	if err := requireActive(viewer); err != nil {
		return MarketPage{}, err
	}
	if input.Quantity < 0 || input.Quantity > marketMaxQuantity {
		message := "Use a quantity from 0 to 10,000."
		return MarketPage{}, Invalid(message, FieldError{Field: "quantity", Message: message})
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Placing clears the cart and archiving drops a line under this lock, so a cart edit can't slip between.
		colony, err := readColony(requestContext, queries, viewer.campaignID, true)
		if err != nil {
			return err
		}
		item, err := queries.GetMarketItem(requestContext, store.GetMarketItemParams{CampaignID: viewer.campaignID, ID: itemID})
		if err != nil {
			return marketItemGone(err)
		}
		if input.Quantity == 0 {
			return queries.DeleteCartLine(requestContext, store.DeleteCartLineParams{CampaignID: viewer.campaignID, ItemID: itemID})
		}
		name := marketItemName(colony, item)
		if item.Archived || !marketItemOnSale(colony, item) {
			if !viewer.SeesDmFields() {
				return errMarketItemNotFound
			}
			return Conflict(name + " is archived.")
		}
		if item.MaxPerOrder != nil && input.Quantity > int64(*item.MaxPerOrder) {
			message := "An order carries at most " + formatNumber(int64(*item.MaxPerOrder)) + " of " + name + "."
			return Invalid(message, FieldError{Field: "quantity", Message: message})
		}
		return queries.UpsertCartLine(requestContext, store.UpsertCartLineParams{
			CampaignID: viewer.campaignID,
			ItemID:     itemID,
			Quantity:   int32(input.Quantity),
			UpdatedBy:  viewer.memberID,
		})
	})
	if err != nil {
		return MarketPage{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket)
	return readMarket(requestContext, service.queries, viewer)
}

// PlaceMarketOrder sends the cart to the mainland. Like Mark played it takes the colony lock for the
// day and writes a PUBLIC entry, but it moves no numbers, so the version and the undo slot stay put.
func (service *Service) PlaceMarketOrder(requestContext context.Context, viewer Viewer, input PlaceMarketOrderInput) (MarketPage, error) {
	if err := requireActive(viewer); err != nil {
		return MarketPage{}, err
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := readColony(requestContext, queries, viewer.campaignID, true)
		if err != nil {
			return err
		}
		settings, err := queries.GetMarketSettings(requestContext, viewer.campaignID)
		if err != nil {
			return campaignGone(err)
		}
		// The colony lock above serializes placing, so two members can't both take the last ship.
		shipsOut, err := queries.CountOpenMarketOrders(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		if shipsOut >= settings.MarketShipLimit {
			return shipsOutError(settings.MarketShipLimit, "placing another order")
		}
		cart, err := queries.ListCartLines(requestContext, viewer.campaignID)
		if err != nil {
			return err
		}
		sortMarketItems(colony, cart, func(row store.ListCartLinesRow) store.MarketItem { return row.MarketItem })
		purchases := make([]game.PurchaseLine, 0, len(cart))
		lines := make([]MarketOrderLine, 0, len(cart))
		for _, row := range cart {
			item := row.MarketItem
			// The cart shows no line whose resource or role is archived, so none sails.
			if !marketItemOnSale(colony, item) {
				continue
			}
			quantity := int64(row.MarketCartLine.Quantity)
			if item.Archived || (item.MaxPerOrder != nil && quantity > int64(*item.MaxPerOrder)) {
				return errMarketCartChanged
			}
			line := MarketOrderLine{
				ItemID: item.ID, Name: marketItemName(colony, item), UnitPrice: int64(item.PriceGold), Quantity: quantity,
				Resources: map[string]int64{}, RoleKey: item.RoleKey,
			}
			if item.ResourceKey != nil && item.Amount != nil {
				line.Resources[*item.ResourceKey] = int64(*item.Amount)
			}
			if item.RoleKey != nil {
				line.Colonists = 1
			}
			purchases = append(purchases, game.PurchaseLine{UnitPrice: line.UnitPrice, Quantity: quantity, Resources: line.Resources, Colonists: line.Colonists})
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			return errMarketCartEmpty
		}
		total, err := game.OrderTotal(purchases)
		if err != nil {
			return gameError(err, nil)
		}
		if total != input.ExpectedTotalGold {
			return errMarketCartChanged
		}
		if err := requireGold(colony); err != nil {
			return err
		}
		arrivesDay, err := game.ArrivesDay(colony.state.Day, int64(settings.MarketVoyageDays))
		if err != nil {
			return gameError(err, nil)
		}
		order, err := queries.CreateMarketOrder(requestContext, store.CreateMarketOrderParams{
			CampaignID: viewer.campaignID,
			PlacedDay:  int32(colony.state.Day),
			ArrivesDay: int32(arrivesDay),
			PlacedBy:   viewer.memberID,
			TotalGold:  int32(total),
		})
		if err != nil {
			return err
		}
		for position, line := range lines {
			resources, err := json.Marshal(line.Resources)
			if err != nil {
				return err
			}
			if err := queries.CreateMarketOrderLine(requestContext, store.CreateMarketOrderLineParams{
				CampaignID: viewer.campaignID,
				OrderID:    order.ID,
				Position:   int32(position),
				ItemID:     line.ItemID,
				Name:       line.Name,
				UnitPrice:  int32(line.UnitPrice),
				Quantity:   int32(line.Quantity),
				Resources:  resources,
				Colonists:  int32(line.Colonists),
				RoleKey:    line.RoleKey,
			}); err != nil {
				return err
			}
		}
		if err := queries.ClearCart(requestContext, viewer.campaignID); err != nil {
			return err
		}
		return writeLooseEntry(requestContext, queries, viewer, plannedEntry{
			day:        colony.state.Day,
			kind:       TimelineKindMarketOrderPlaced,
			title:      "Order placed: " + orderSummaryText(colony, lines) + " · docks " + dayText(arrivesDay),
			visibility: TimelineVisibilityPublic,
		})
	})
	if err != nil {
		return MarketPage{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket, TopicTimeline, TopicColony)
	return readMarket(requestContext, service.queries, viewer)
}

// requireGold refuses an order the colony could never pay for, with game.Unload's own wording.
func requireGold(colony colonySnapshot) error {
	gold, found := colony.state.Resources[game.GoldResourceKey]
	if !found {
		return errMarketNoGold
	}
	if gold.Archived {
		return Conflict(gold.Label + " is archived.")
	}
	return nil
}

// PreviewPayMarketOrder is the Pay sheet's before → after, without writing.
func (service *Service) PreviewPayMarketOrder(requestContext context.Context, viewer Viewer, orderID uuid.UUID) (MarketPayPreview, error) {
	if err := requireActive(viewer); err != nil {
		return MarketPayPreview{}, err
	}
	colony, err := readColony(requestContext, service.queries, viewer.campaignID, false)
	if err != nil {
		return MarketPayPreview{}, err
	}
	order, err := service.queries.GetMarketOrder(requestContext, store.GetMarketOrderParams{CampaignID: viewer.campaignID, ID: orderID})
	if err != nil {
		return MarketPayPreview{}, marketOrderGone(err)
	}
	if order.Status != MarketOrderStatusUnpaid {
		return MarketPayPreview{}, errMarketNotUnpaid
	}
	if !game.Docked(colony.state.Day, int64(order.ArrivesDay)) {
		return MarketPayPreview{}, notDockedError(int64(order.ArrivesDay))
	}
	purchases, err := orderPurchases(requestContext, service.queries, viewer.campaignID, orderID)
	if err != nil {
		return MarketPayPreview{}, err
	}
	result, err := game.Unload(colony.state, purchases)
	var shortfallError *game.ShortfallError
	if errors.As(err, &shortfallError) {
		return MarketPayPreview{
			Shortfall: shortfallError.Error(),
			Resources: []ResourceChangePreview{},
			Roles:     []RoleChangePreview{},
			Version:   colony.row.Version,
		}, nil
	}
	if err != nil {
		return MarketPayPreview{}, gameError(err, nil)
	}
	resources, parts := effectsPreview(colony, game.EffectsResult{Before: result.Before, After: result.After, Changes: result.Changes, Applied: result.Applied})
	roles := roleChangePreviews(colony, result)
	for _, role := range roles {
		parts = append(parts, role.Label+" "+formatNumber(role.Before)+" → "+formatNumber(role.After))
	}
	return MarketPayPreview{
		Affordable: true,
		Resources:  resources,
		Roles:      roles,
		Summary:    joinParts(parts...),
		Version:    colony.row.Version,
	}, nil
}

func roleChangePreviews(colony colonySnapshot, result game.UnloadResult) []RoleChangePreview {
	roles := make([]RoleChangePreview, 0, len(result.Colonists))
	for _, key := range sortedRoleKeys(colony, result.Colonists) {
		before := result.Before.Roles[key]
		after := result.After.Roles[key]
		roles = append(roles, RoleChangePreview{
			Key:    key,
			Label:  before.Label,
			Before: before.UnnamedCount + before.NamedCount,
			After:  after.UnnamedCount + after.NamedCount,
		})
	}
	return roles
}

// sortedRoleKeys lists the roles in board order, so text and previews read the way People does.
func sortedRoleKeys(colony colonySnapshot, colonists map[string]int64) []string {
	return slices.SortedFunc(maps.Keys(colonists), func(left, right string) int {
		return cmp.Or(cmp.Compare(colony.roles[left].SortOrder, colony.roles[right].SortOrder), cmp.Compare(left, right))
	})
}

func notDockedError(arrivesDay int64) error {
	return Conflict("The ship hasn't docked yet. It docks " + dayText(arrivesDay) + ".")
}

func orderPurchases(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, orderID uuid.UUID) ([]game.PurchaseLine, error) {
	lines, err := listOrderLines(requestContext, queries, campaignID, []uuid.UUID{orderID})
	if err != nil {
		return nil, err
	}
	purchases := make([]game.PurchaseLine, 0, len(lines[orderID]))
	for _, line := range lines[orderID] {
		purchase := game.PurchaseLine{UnitPrice: line.UnitPrice, Quantity: line.Quantity, Resources: line.Resources, Colonists: line.Colonists}
		if line.RoleKey != nil {
			purchase.RoleKey = *line.RoleKey
		}
		purchases = append(purchases, purchase)
	}
	return purchases, nil
}

// PayMarketOrder unloads a docked ship: STRICT gold, grants capped at their max, colonists into their
// roles. It is a colony change any active member may make, and the DM may undo it.
func (service *Service) PayMarketOrder(requestContext context.Context, viewer Viewer, orderID uuid.UUID, input PayMarketOrderInput) (ColonyChange, error) {
	return service.runColonyMutation(requestContext, viewer, requireActive, &input.ExpectedVersion, func(colony colonySnapshot, queries *store.Queries) (colonyOutcome, error) {
		order, err := queries.GetMarketOrderForUpdate(requestContext, store.GetMarketOrderForUpdateParams{CampaignID: viewer.campaignID, ID: orderID})
		if err != nil {
			return colonyOutcome{}, marketOrderGone(err)
		}
		if order.Status != MarketOrderStatusUnpaid {
			return colonyOutcome{}, errMarketNotUnpaid
		}
		if !game.Docked(colony.state.Day, int64(order.ArrivesDay)) {
			return colonyOutcome{}, notDockedError(int64(order.ArrivesDay))
		}
		lines, err := listOrderLines(requestContext, queries, viewer.campaignID, []uuid.UUID{orderID})
		if err != nil {
			return colonyOutcome{}, err
		}
		purchases, err := orderPurchases(requestContext, queries, viewer.campaignID, orderID)
		if err != nil {
			return colonyOutcome{}, err
		}
		result, err := game.Unload(colony.state, purchases)
		var shortfallError *game.ShortfallError
		if errors.As(err, &shortfallError) {
			return colonyOutcome{}, Conflict(shortfallError.Error() + ".")
		}
		if err != nil {
			return colonyOutcome{}, gameError(err, nil)
		}
		groupID := uuid.New()
		if err := queries.PayMarketOrder(requestContext, store.PayMarketOrderParams{
			CampaignID:  viewer.campaignID,
			ID:          orderID,
			PaidDay:     int32Pointer(colony.state.Day),
			PaidBy:      memberReference(viewer),
			PaidGroupID: &groupID,
		}); err != nil {
			return colonyOutcome{}, err
		}
		title := "Goods unloaded · paid " + formatNumber(result.Total) + " gold"
		return colonyOutcome{
			after:   result.After,
			label:   title,
			groupID: groupID,
			entries: []plannedEntry{{
				day:        colony.state.Day,
				kind:       TimelineKindMarketOrderPaid,
				title:      title,
				body:       paidBody(colony, lines[orderID], result.Colonists),
				visibility: TimelineVisibilityPublic,
				deltas:     result.Applied,
				data:       marketPaidData{OrderID: orderID, Colonists: result.Colonists},
			}},
			marketOrders: map[uuid.UUID]marketOrderBefore{orderID: {Status: MarketOrderStatusUnpaid}},
			topics:       []Topic{TopicMarket},
		}, nil
	})
}

// marketPaidData is MARKET_ORDER_PAID's data: the colonists each role gained.
type marketPaidData struct {
	OrderID   uuid.UUID        `json:"orderId"`
	Colonists map[string]int64 `json:"colonists"`
}

func paidBody(colony colonySnapshot, lines []MarketOrderLine, colonists map[string]int64) string {
	joined := make([]string, 0, len(colonists))
	for _, key := range sortedRoleKeys(colony, colonists) {
		if colonists[key] > 0 {
			joined = append(joined, roleColonistsText(colony, colonists[key], key))
		}
	}
	special := []string{}
	for _, line := range lines {
		if len(line.Resources) == 0 && line.Colonists == 0 {
			special = append(special, specialItemText(line))
		}
	}
	parts := []string{}
	if len(joined) > 0 {
		parts = append(parts, "Joined: "+strings.Join(joined, ", "))
	}
	if len(special) > 0 {
		parts = append(parts, "Delivered: "+strings.Join(special, ", "))
	}
	return joinParts(parts...)
}

// CancelMarketOrder withdraws the open order. Nothing was charged, so nothing is refunded, and the
// cart stays as it is.
func (service *Service) CancelMarketOrder(requestContext context.Context, viewer Viewer, orderID uuid.UUID) (MarketPage, error) {
	if err := requireDm(viewer); err != nil {
		return MarketPage{}, err
	}
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID)
		if err != nil {
			return campaignGone(err)
		}
		order, err := queries.GetMarketOrderForUpdate(requestContext, store.GetMarketOrderForUpdateParams{CampaignID: viewer.campaignID, ID: orderID})
		if err != nil {
			return marketOrderGone(err)
		}
		if order.Status != MarketOrderStatusUnpaid {
			return errMarketNotUnpaid
		}
		if err := queries.CancelMarketOrder(requestContext, store.CancelMarketOrderParams{CampaignID: viewer.campaignID, ID: orderID, CancelledDay: &colony.Day}); err != nil {
			return err
		}
		return writeLooseEntry(requestContext, queries, viewer, plannedEntry{
			day:        int64(colony.Day),
			kind:       TimelineKindMarketOrderCancelled,
			title:      "Order cancelled",
			visibility: TimelineVisibilityPublic,
		})
	})
	if err != nil {
		return MarketPage{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicMarket, TopicTimeline, TopicColony)
	return readMarket(requestContext, service.queries, viewer)
}

// ordersDockingDuring is the open orders that reach the docks on a day after fromDay, up to toDay.
func ordersDockingDuring(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, fromDay int64, toDay int64) ([]store.MarketOrder, error) {
	open, err := queries.ListOpenMarketOrders(requestContext, campaignID)
	if err != nil {
		return nil, err
	}
	docking := []store.MarketOrder{}
	for _, row := range open {
		if game.DocksDuring(fromDay, toDay, int64(row.MarketOrder.ArrivesDay)) {
			docking = append(docking, row.MarketOrder)
		}
	}
	return docking, nil
}

// restoreMarketOrders puts paid ships back at the docks. A reopened ship counts against the DM's limit,
// so ships ordered since the payment block the undo rather than putting more out than allowed.
func restoreMarketOrders(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, before map[uuid.UUID]marketOrderBefore) error {
	if len(before) == 0 {
		return nil
	}
	settings, err := queries.GetMarketSettings(requestContext, campaignID)
	if err != nil {
		return err
	}
	for _, orderID := range slices.SortedFunc(maps.Keys(before), compareIDs) {
		if _, err := queries.GetMarketOrderForUpdate(requestContext, store.GetMarketOrderForUpdateParams{CampaignID: campaignID, ID: orderID}); err != nil {
			return err
		}
		shipsOut, err := queries.CountOpenMarketOrders(requestContext, campaignID)
		if err != nil {
			return err
		}
		if shipsOut >= settings.MarketShipLimit {
			return shipsOutError(settings.MarketShipLimit, "undoing this payment")
		}
		if err := queries.ReopenPaidMarketOrder(requestContext, store.ReopenPaidMarketOrderParams{CampaignID: campaignID, ID: orderID}); err != nil {
			return err
		}
	}
	return nil
}

func dockedTitle(totalGold int64) string {
	return "Goods ship docked · " + formatNumber(totalGold) + " gold due"
}

// orderSummaryText is "20 food, 3 Farmers, Boots of Striding".
func orderSummaryText(colony colonySnapshot, lines []MarketOrderLine) string {
	parts := []string{}
	for _, line := range lines {
		for _, key := range slices.Sorted(maps.Keys(line.Resources)) {
			parts = append(parts, formatNumber(line.Resources[key]*line.Quantity)+" "+strings.ToLower(colony.labels.of(key)))
		}
		if line.Colonists > 0 {
			roleKey := game.UnassignedRoleKey
			if line.RoleKey != nil {
				roleKey = *line.RoleKey
			}
			parts = append(parts, roleColonistsText(colony, line.Colonists*line.Quantity, roleKey))
		}
		if len(line.Resources) == 0 && line.Colonists == 0 {
			parts = append(parts, specialItemText(line))
		}
	}
	return strings.Join(parts, ", ")
}

// roleColonistsText is "3 Farmers", or "2 colonists" for Unassigned.
func roleColonistsText(colony colonySnapshot, count int64, roleKey string) string {
	role, found := colony.roles[roleKey]
	if roleKey == game.UnassignedRoleKey || !found {
		return formatNumber(count) + " " + plural(count, "colonist", "colonists")
	}
	return formatNumber(count) + " " + plural(count, role.Label, role.Label+"s")
}

func specialItemText(line MarketOrderLine) string {
	if line.Quantity > 1 {
		return line.Name + " × " + formatNumber(line.Quantity)
	}
	return line.Name
}
