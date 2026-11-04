package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// The sample market, played through core on Day 22 after the last advance. Approval priced Food,
// Materials and every role's colonists; the DM raises Mages to 80 gp and adds the Boots of Striding.
// Thorin orders 10 lots of food (20 food) and two Farmers for 70 gold, which docks on Day 36 at the
// default 14-day voyage, and Sela leaves the Boots and 5 lots of food in the cart. Placing moves no
// numbers, so every earlier figure and the undo slot stay as they were.
func seedMarket(requestContext context.Context, service *core.Service, dm core.Viewer, enter enterAs) error {
	page, err := service.SetColonistOffer(requestContext, dm, "MAGE", core.ColonistOfferInput{PriceGold: 80})
	if err != nil {
		return fmt.Errorf("the mage offer: %w", err)
	}
	mostPerOrder := int64(1)
	boots, err := service.CreateMarketItem(requestContext, dm, core.MarketItemInput{
		Name:        "Boots of Striding",
		Description: "Uncommon wondrous item, sent from a Thornwall enchanter. The DM hands it to whoever claims it.",
		PriceGold:   120,
		MaxPerOrder: &mostPerOrder,
	})
	if err != nil {
		return fmt.Errorf("the boots item: %w", err)
	}
	food, farmer := uuid.Nil, uuid.Nil
	for _, item := range page.Items {
		if item.ResourceKey != nil && *item.ResourceKey == "food" {
			food = item.ID
		}
		if item.RoleKey != nil && *item.RoleKey == "FARMER" {
			farmer = item.ID
		}
	}
	if food == uuid.Nil || farmer == uuid.Nil {
		return fmt.Errorf("the default food or farmer offer is missing")
	}

	addToCart := func(player core.Viewer, itemID uuid.UUID, quantity int64) error {
		_, err := service.SetCartQuantity(requestContext, player, itemID, core.CartQuantityInput{Quantity: quantity})
		return err
	}
	thorin, err := enter("seed:ana")
	if err != nil {
		return err
	}
	if err := addToCart(thorin, food, 10); err != nil {
		return fmt.Errorf("thorin's food: %w", err)
	}
	if err := addToCart(thorin, farmer, 2); err != nil {
		return fmt.Errorf("thorin's farmers: %w", err)
	}
	if _, err := service.PlaceMarketOrder(requestContext, thorin, core.PlaceMarketOrderInput{ExpectedTotalGold: 70}); err != nil {
		return fmt.Errorf("thorin's order: %w", err)
	}
	sela, err := enter("seed:bram")
	if err != nil {
		return err
	}
	if err := addToCart(sela, boots.ID, 1); err != nil {
		return fmt.Errorf("sela's boots: %w", err)
	}
	if err := addToCart(sela, food, 5); err != nil {
		return fmt.Errorf("sela's food: %w", err)
	}
	return nil
}
