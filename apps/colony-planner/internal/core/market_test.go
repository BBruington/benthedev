package core_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

func (test *colonyTest) market(viewer core.Viewer) core.MarketPage {
	test.t.Helper()
	page, err := test.service.GetMarket(test.context, viewer)
	if err != nil {
		test.t.Fatalf("get market: %v", err)
	}
	return page
}

// marketItemID finds a catalog row by name, archived ones included, as the DM sees it.
func (test *colonyTest) marketItemID(name string) uuid.UUID {
	test.t.Helper()
	for _, item := range test.market(test.dm).Items {
		if item.Name == name {
			return item.ID
		}
	}
	test.t.Fatalf("no market item %q", name)
	return uuid.Nil
}

// offerID finds the standing offer for a resource or a role, as the DM sees it.
func (test *colonyTest) offerID(viewer core.Viewer, resourceKey, roleKey string) (uuid.UUID, bool) {
	test.t.Helper()
	for _, item := range test.market(viewer).Items {
		if (item.ResourceKey != nil && *item.ResourceKey == resourceKey) || (item.RoleKey != nil && *item.RoleKey == roleKey) {
			return item.ID, true
		}
	}
	return uuid.Nil, false
}

func (test *colonyTest) farmerItem() uuid.UUID {
	test.t.Helper()
	itemID, found := test.offerID(test.dm, "", "FARMER")
	if !found {
		test.t.Fatal("approval made no Farmer offer")
	}
	return itemID
}

func (test *colonyTest) specialItem(name string, priceGold int64, maxPerOrder *int64) uuid.UUID {
	test.t.Helper()
	item, err := test.service.CreateMarketItem(test.context, test.dm, core.MarketItemInput{Name: name, PriceGold: priceGold, MaxPerOrder: maxPerOrder})
	if err != nil {
		test.t.Fatalf("create %s: %v", name, err)
	}
	return item.ID
}

func (test *colonyTest) setCart(viewer core.Viewer, itemID uuid.UUID, quantity int64) core.MarketPage {
	test.t.Helper()
	page, err := test.service.SetCartQuantity(test.context, viewer, itemID, core.CartQuantityInput{Quantity: quantity})
	if err != nil {
		test.t.Fatalf("set cart quantity %d: %v", quantity, err)
	}
	return page
}

// placeOrder returns the order it placed, found as the one open order that wasn't open before.
func (test *colonyTest) placeOrder(viewer core.Viewer) core.MarketOrder {
	test.t.Helper()
	before := test.market(viewer)
	page, err := test.service.PlaceMarketOrder(test.context, viewer, core.PlaceMarketOrderInput{ExpectedTotalGold: before.Cart.TotalGold})
	if err != nil {
		test.t.Fatalf("place order: %v", err)
	}
	for _, order := range page.Orders {
		if !slices.ContainsFunc(before.Orders, func(earlier core.MarketOrder) bool { return earlier.ID == order.ID }) {
			return order
		}
	}
	test.t.Fatal("placing left no new open order")
	return core.MarketOrder{}
}

func (test *colonyTest) payOrder(viewer core.Viewer, orderID uuid.UUID) (core.ColonyChange, error) {
	test.t.Helper()
	return test.service.PayMarketOrder(test.context, viewer, orderID, core.PayMarketOrderInput{ExpectedVersion: test.board().Version})
}

func (test *colonyTest) setShipping(voyageDays int64, shipLimit int64) {
	test.t.Helper()
	if _, err := test.service.SetMarketShipping(test.context, test.dm, core.MarketShippingInput{VoyageDays: voyageDays, ShipLimit: shipLimit}); err != nil {
		test.t.Fatalf("set shipping: %v", err)
	}
}

func TestPlacingAMarketOrderSetsItsDockingDayAndLeavesTheVersion(t *testing.T) {
	test := newColonyTest(t)
	test.correctDay(10)
	food := test.marketItemID("Food")
	test.setCart(test.player, food, 10)

	_, err := test.service.PlaceMarketOrder(test.context, test.player, core.PlaceMarketOrderInput{ExpectedTotalGold: 5})
	requireMessage(t, err, core.KindConflict, "The cart changed. Review it and confirm again.")

	version := test.board().Version
	order := test.placeOrder(test.player)
	if order.PlacedDay != 10 || order.ArrivesDay != 24 || order.Docked || order.TotalGold != 10 {
		t.Fatalf("order = %+v; want placed Day 10, arriving Day 24 (the default 14-day voyage), at sea, 10 gold", order)
	}
	if after := test.board().Version; after != version {
		t.Fatalf("version %d after placing; want %d unchanged", after, version)
	}
	if page := test.market(test.player); len(page.Cart.Lines) != 0 || page.Cart.TotalGold != 0 {
		t.Fatalf("cart after placing = %+v; want empty", page.Cart)
	}
	if got := kinds(test.timeline(test.player, 10, 10)); len(got) == 0 || got[len(got)-1] != core.TimelineKindMarketOrderPlaced {
		t.Fatalf("player's timeline kinds on Day 10 = %v; want MARKET_ORDER_PLACED last", got)
	}

	test.setCart(test.player, food, 1)
	_, err = test.service.PlaceMarketOrder(test.context, test.player, core.PlaceMarketOrderInput{ExpectedTotalGold: 1})
	requireMessage(t, err, core.KindConflict, "The ship is still out. Pay for it at the docks or cancel it before placing another order.")
}

func TestAMarketOrderDocksPaysAndUndoes(t *testing.T) {
	test := newColonyTest(t)
	test.correctDay(10)
	test.setShipping(3, 1)
	farmer := test.farmerItem()
	test.setCart(test.player, test.marketItemID("Food"), 10)
	test.setCart(test.player, farmer, 1)
	order := test.placeOrder(test.player)
	if order.ArrivesDay != 13 || order.TotalGold != 40 {
		t.Fatalf("order = %+v; want arriving Day 13 for 40 gold", order)
	}

	_, err := test.payOrder(test.player, order.ID)
	requireMessage(t, err, core.KindConflict, "The ship hasn't docked yet. It docks Day 13.")

	countDocked := func() int {
		docked := 0
		for _, entry := range test.timeline(test.player, 11, 13) {
			if entry.Kind == core.TimelineKindMarketOrderDocked {
				docked++
				if entry.Day != 13 {
					t.Fatalf("docked entry on Day %d; want 13", entry.Day)
				}
			}
		}
		return docked
	}
	advanced := test.advance(3)
	if docked := countDocked(); docked != 1 {
		t.Fatalf("%d docked entries; want 1", docked)
	}
	if page := test.market(test.player); len(page.Orders) != 1 || !page.Orders[0].Docked {
		t.Fatalf("open orders after advancing = %+v; want one, docked", page.Orders)
	}
	test.undo(advanced)
	if page := test.market(test.player); len(page.Orders) != 1 || page.Orders[0].Docked {
		t.Fatalf("open orders after undoing the advance = %+v; want one, at sea again", page.Orders)
	}
	test.advance(3)
	if docked := countDocked(); docked != 1 {
		t.Fatalf("%d docked entries after undoing and advancing again; want 1", docked)
	}
	test.correctDay(11)
	test.advance(2)
	if docked := countDocked(); docked != 1 {
		t.Fatalf("%d docked entries after correcting the day back and advancing again; want 1", docked)
	}

	test.setTo(map[string]int64{"gold": 20, "food": 5})
	preview, err := test.service.PreviewPayMarketOrder(test.context, test.player, order.ID)
	if err != nil || preview.Affordable || !strings.Contains(preview.Shortfall, "gold") {
		t.Fatalf("preview with 20 gold = %+v, %v; want unaffordable, naming gold", preview, err)
	}
	_, err = test.payOrder(test.player, order.ID)
	if operationError, typed := core.AsError(err); !typed || operationError.Kind != core.KindConflict || !strings.Contains(operationError.Message, "gold") {
		t.Fatalf("paying with 20 gold = %v; want a conflict naming gold", err)
	}

	test.setTo(map[string]int64{"gold": 100})
	farmersBefore := test.headcount(test.dm, "FARMER").Total
	paid, err := test.payOrder(test.player, order.ID)
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	if gold, food, farmers := test.amount("gold"), test.amount("food"), test.headcount(test.dm, "FARMER").Total; gold != 60 || food != 25 || farmers != farmersBefore+1 {
		t.Fatalf("after paying: gold %d, food %d, farmers %d; want 60, 25, %d", gold, food, farmers, farmersBefore+1)
	}
	if page := test.market(test.player); len(page.Orders) != 0 || len(page.History) != 1 || page.History[0].Status != core.MarketOrderStatusPaid {
		t.Fatalf("market after paying: orders %+v, history %+v; want none open and one paid", page.Orders, page.History)
	}

	test.undo(paid)
	if gold, food, farmers := test.amount("gold"), test.amount("food"), test.headcount(test.dm, "FARMER").Total; gold != 100 || food != 5 || farmers != farmersBefore {
		t.Fatalf("after undoing the payment: gold %d, food %d, farmers %d; want 100, 5, %d", gold, food, farmers, farmersBefore)
	}
	page := test.market(test.player)
	if len(page.Orders) != 1 || page.Orders[0].ID != order.ID || page.Orders[0].Status != core.MarketOrderStatusUnpaid || !page.Orders[0].Docked {
		t.Fatalf("open orders after undoing the payment = %+v; want the same order docked and unpaid", page.Orders)
	}
}

func TestUndoingAPaymentIsRefusedWhileEveryShipIsOut(t *testing.T) {
	test := newColonyTest(t)
	test.setShipping(1, 1)
	food := test.marketItemID("Food")
	test.setTo(map[string]int64{"gold": 100})
	test.setCart(test.player, food, 4)
	first := test.placeOrder(test.player)
	test.advance(1)
	paid, err := test.payOrder(test.player, first.ID)
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	test.setCart(test.player, food, 2)
	test.placeOrder(test.player)

	_, err = test.service.UndoLastChange(test.context, test.dm, core.UndoInput{GroupID: paid.GroupID})
	requireMessage(t, err, core.KindConflict, "The ship is still out. Pay for it at the docks or cancel it before undoing this payment.")

	test.setShipping(1, 2)
	test.undo(paid)
	if page := test.market(test.player); len(page.Orders) != 2 {
		t.Fatalf("open orders after undoing with room for two ships = %+v; want both", page.Orders)
	}
}

func TestTheShipLimitCapsOpenOrdersAndEachShipDocks(t *testing.T) {
	test := newColonyTest(t)
	test.correctDay(10)
	test.setShipping(5, 2)
	food := test.marketItemID("Food")
	test.setCart(test.player, food, 2)
	first := test.placeOrder(test.player)
	test.advance(1)
	test.setCart(test.player, food, 4)
	second := test.placeOrder(test.player)
	if first.ArrivesDay != 15 || second.ArrivesDay != 16 {
		t.Fatalf("arrival days %d and %d; want 15 and 16", first.ArrivesDay, second.ArrivesDay)
	}
	test.setCart(test.player, food, 1)
	_, err := test.service.PlaceMarketOrder(test.context, test.player, core.PlaceMarketOrderInput{ExpectedTotalGold: 1})
	requireMessage(t, err, core.KindConflict, "All 2 ships are out. Pay for one at the docks or cancel one before placing another order.")

	if _, err := test.service.SetMarketShipping(test.context, test.dm, core.MarketShippingInput{VoyageDays: 5, ShipLimit: 0}); !isKind(err, core.KindInvalid) {
		t.Fatalf("a ship limit of 0 = %v; want invalid", err)
	}
	test.setShipping(5, 1)
	if page := test.market(test.player); len(page.Orders) != 2 || page.Orders[0].ID != first.ID || page.Orders[1].ID != second.ID {
		t.Fatalf("open orders after lowering the limit = %+v; want both still out, soonest first", page.Orders)
	}

	preview, err := test.service.PreviewAdvance(test.context, test.dm, core.AdvancePreviewInput{Days: 5})
	if err != nil || !slices.Equal(preview.MarketDocksDays, []int64{15, 16}) {
		t.Fatalf("preview = %+v, %v; want docking on Days 15 and 16", preview.MarketDocksDays, err)
	}
	test.advance(5)
	dockedDays := []int64{}
	for _, entry := range test.timeline(test.player, 12, 16) {
		if entry.Kind == core.TimelineKindMarketOrderDocked {
			dockedDays = append(dockedDays, entry.Day)
		}
	}
	slices.Sort(dockedDays)
	if !slices.Equal(dockedDays, []int64{15, 16}) {
		t.Fatalf("docked entries on days %v; want 15 and 16", dockedDays)
	}
}

func TestOnlyTheDmRunsTheMarket(t *testing.T) {
	test := newColonyTest(t)
	food := test.marketItemID("Food")
	test.setCart(test.player, food, 2)
	order := test.placeOrder(test.player)

	if _, err := test.service.CancelMarketOrder(test.context, test.player, order.ID); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player cancelling = %v; want forbidden", err)
	}
	if _, err := test.service.SetSupplyOffer(test.context, test.player, "food", core.SupplyOfferInput{PriceGold: 0, Amount: 2}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player pricing food = %v; want forbidden", err)
	}
	if _, err := test.service.SetColonistOffer(test.context, test.player, "FARMER", core.ColonistOfferInput{PriceGold: 0}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player pricing farmers = %v; want forbidden", err)
	}
	if _, err := test.service.CreateMarketItem(test.context, test.player, core.MarketItemInput{Name: "Gift"}); !isKind(err, core.KindForbidden) {
		t.Fatalf("a player creating an item = %v; want forbidden", err)
	}

	page, err := test.service.CancelMarketOrder(test.context, test.dm, order.ID)
	if err != nil || len(page.Orders) != 0 || len(page.History) != 1 || page.History[0].Status != core.MarketOrderStatusCancelled {
		t.Fatalf("cancel = %+v, %v; want no open order and one cancelled", page, err)
	}
}

func TestArchivingAnItemClearsItsCartLines(t *testing.T) {
	test := newColonyTest(t)
	food := test.marketItemID("Food")
	test.setCart(test.player, food, 3)
	if _, err := test.service.SetMarketItemArchived(test.context, test.dm, food, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	page := test.market(test.player)
	if len(page.Cart.Lines) != 0 {
		t.Fatalf("cart after archiving = %+v; want empty", page.Cart.Lines)
	}
	for _, item := range page.Items {
		if item.ID == food {
			t.Fatal("the player still sees the archived item")
		}
	}
	if _, err := test.service.SetCartQuantity(test.context, test.player, food, core.CartQuantityInput{Quantity: 1}); !isKind(err, core.KindNotFound) {
		t.Fatalf("adding an archived item = %v; want not found", err)
	}
}

func TestSettingAnOfferPutsItBackOnSale(t *testing.T) {
	test := newColonyTest(t)
	food := test.marketItemID("Food")
	if _, err := test.service.SetMarketItemArchived(test.context, test.dm, food, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	page, err := test.service.SetSupplyOffer(test.context, test.dm, "food", core.SupplyOfferInput{PriceGold: 3, Amount: 5})
	if err != nil {
		t.Fatalf("price food: %v", err)
	}
	for _, item := range page.Items {
		if item.ID == food && (item.Archived || item.PriceGold != 3 || item.Amount == nil || *item.Amount != 5 || item.Name != "Food") {
			t.Fatalf("food offer after pricing = %+v; want 5 food for 3 gp, on sale", item)
		}
	}
	if _, found := test.offerID(test.player, "food", ""); !found {
		t.Fatal("the player doesn't see the repriced food offer")
	}
	_, err = test.service.SetSupplyOffer(test.context, test.dm, "food", core.SupplyOfferInput{PriceGold: 3, Amount: 0})
	requireFieldMessage(t, err, "amount", "Use an amount from 1 to 1,000,000.")
	_, err = test.service.SetSupplyOffer(test.context, test.dm, "spice", core.SupplyOfferInput{PriceGold: 3, Amount: 1})
	requireMessage(t, err, core.KindNotFound, "That resource doesn't exist.")
	_, err = test.service.SetColonistOffer(test.context, test.dm, "BARD", core.ColonistOfferInput{PriceGold: 3})
	requireMessage(t, err, core.KindNotFound, "That role doesn't exist.")
}

func TestAnArchivedRoleTakesItsOfferOffSale(t *testing.T) {
	test := newColonyTest(t)
	mage, found := test.offerID(test.dm, "", "MAGE")
	if !found {
		t.Fatal("approval made no Mage offer")
	}
	test.setCart(test.player, mage, 2)
	if _, err := test.service.UpdateRole(test.context, test.dm, "MAGE", core.RoleInput{Label: "Mage", Icon: core.ColonyIconWandSparkles, SortOrder: 80, Archived: true}); err != nil {
		t.Fatalf("archive Mage: %v", err)
	}
	if _, found := test.offerID(test.player, "", "MAGE"); found {
		t.Fatal("the player still sees the archived role's offer")
	}
	if _, found := test.offerID(test.dm, "", "MAGE"); !found {
		t.Fatal("the DM lost the archived role's offer")
	}
	if cart := test.market(test.player).Cart; len(cart.Lines) != 0 || cart.TotalGold != 0 {
		t.Fatalf("cart after archiving Mage = %+v; want the line left out", cart)
	}
	if _, err := test.service.SetCartQuantity(test.context, test.player, mage, core.CartQuantityInput{Quantity: 1}); !isKind(err, core.KindNotFound) {
		t.Fatalf("adding the archived role's offer = %v; want not found", err)
	}
	_, err := test.service.SetColonistOffer(test.context, test.dm, "MAGE", core.ColonistOfferInput{PriceGold: 50})
	requireMessage(t, err, core.KindConflict, "Mage is archived.")
}

func TestANewRoleGoesOnSale(t *testing.T) {
	test := newColonyTest(t)
	role, err := test.service.CreateRole(test.context, test.dm, core.NewRoleInput{Label: "Bard", Icon: core.ColonyIconWheat})
	if err != nil {
		t.Fatalf("create Bard: %v", err)
	}
	for _, item := range test.market(test.player).Items {
		if item.RoleKey != nil && *item.RoleKey == role.Key {
			if item.Name != "Bard" || item.PriceGold != 30 {
				t.Fatalf("the new role's offer = %+v; want Bard at 30 gp", item)
			}
			return
		}
	}
	t.Fatal("the new role has no offer")
}

func TestOffersAreNotEditedAsItems(t *testing.T) {
	test := newColonyTest(t)
	_, err := test.service.UpdateMarketItem(test.context, test.dm, test.marketItemID("Food"), core.MarketItemInput{Name: "Cheap food", PriceGold: 1})
	requireMessage(t, err, core.KindNotFound, "That item doesn't exist.")
}

func TestMarketItemsKeepTheirLimitPerOrder(t *testing.T) {
	test := newColonyTest(t)
	most := int64(1)
	boots := test.specialItem("Boots of Striding", 120, &most)
	_, err := test.service.SetCartQuantity(test.context, test.player, boots, core.CartQuantityInput{Quantity: 2})
	requireMessage(t, err, core.KindInvalid, "An order carries at most 1 of Boots of Striding.")
	test.setCart(test.player, boots, 1)
}

func TestMarketOffersNeverSellGoldOrMorale(t *testing.T) {
	test := newColonyTest(t)
	for key, label := range map[string]string{"gold": "Gold", "morale": "Morale"} {
		_, err := test.service.SetSupplyOffer(test.context, test.dm, key, core.SupplyOfferInput{PriceGold: 5, Amount: 1})
		requireMessage(t, err, core.KindInvalid, label+" can't be bought at the market.")
	}
	if _, err := test.service.SetSupplyOffer(test.context, test.dm, "materials", core.SupplyOfferInput{PriceGold: 5, Amount: 3}); err != nil {
		t.Fatalf("an offer of materials = %v; want it saved", err)
	}
}

func TestLoweringAnItemsLimitTrimsTheCart(t *testing.T) {
	test := newColonyTest(t)
	lantern := test.specialItem("Lantern", 1, nil)
	test.setCart(test.player, lantern, 10)
	most := int64(4)
	if _, err := test.service.UpdateMarketItem(test.context, test.dm, lantern, core.MarketItemInput{Name: "Lantern", PriceGold: 1, MaxPerOrder: &most}); err != nil {
		t.Fatalf("lower the limit: %v", err)
	}
	if cart := test.market(test.player).Cart; len(cart.Lines) != 1 || cart.Lines[0].Quantity != 4 || cart.TotalGold != 4 {
		t.Fatalf("cart after lowering the limit to 4 = %+v; want 4 lanterns for 4 gold", cart)
	}
	test.placeOrder(test.player)
}

// A player's payment answers with the player's board: a hidden named colonist stays unnamed.
func TestAPlayersPaymentReturnsThePlayersBoard(t *testing.T) {
	test := newColonyTest(t)
	mason := test.npc(core.NewNpcInput{Revealed: false, NpcInput: core.NpcInput{Name: "Edric Vael"}})
	test.join(mason.ID, "FARMER", core.JoinModeNewColonist)
	test.setShipping(1, 1)
	test.setTo(map[string]int64{"gold": 100})
	test.setCart(test.player, test.farmerItem(), 1)
	order := test.placeOrder(test.player)
	test.advance(1)

	paid, err := test.payOrder(test.player, order.ID)
	if err != nil {
		t.Fatalf("pay: %v", err)
	}
	playerBoard, err := test.service.GetColonyBoard(test.context, test.player)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paid.Board, playerBoard) {
		t.Fatalf("the payment's board = %+v; want the player's own board %+v", paid.Board, playerBoard)
	}
}
