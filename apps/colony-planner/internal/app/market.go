package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerMarket(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "getMarket",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/market",
		Summary:     "The market: catalog, the party's cart, the open order and past orders",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.MarketPage], error) {
		page, err := handlers.core.GetMarket(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createMarketItem",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/market/items",
		Summary:       "Add an item to the catalog",
		Tags:          []string{"market"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.MarketItemInput]) (*bodyOutput[core.MarketItem], error) {
		item, err := handlers.core.CreateMarketItem(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketItem]{Body: item}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateMarketItem",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/market/items/{itemId}",
		Summary:     "Replace an item's record (archiving has its own operation); placed orders keep their snapshot",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *marketItemBodyInput[core.MarketItemInput]) (*bodyOutput[core.MarketItem], error) {
		item, err := handlers.core.UpdateMarketItem(requestContext, viewerFrom(requestContext), input.ItemID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketItem]{Body: item}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setMarketItemArchived",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/market/items/{itemId}/archived",
		Summary:     "Archive an item (gone from the catalog and the cart, history kept) or bring it back",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *marketItemBodyInput[ArchiveInput]) (*bodyOutput[core.MarketItem], error) {
		item, err := handlers.core.SetMarketItemArchived(requestContext, viewerFrom(requestContext), input.ItemID, input.Body.Archived)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketItem]{Body: item}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setMarketVoyage",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/market/voyage",
		Summary:     "Set how many days a new order is at sea",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *campaignBodyInput[core.MarketVoyageInput]) (*bodyOutput[core.MarketPage], error) {
		page, err := handlers.core.SetMarketVoyageDays(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setSupplyOffer",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/market/supplies/{resourceKey}",
		Summary:     "Price a resource's standing offer, putting it on sale",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *resourceBodyInput[core.SupplyOfferInput]) (*bodyOutput[core.MarketPage], error) {
		page, err := handlers.core.SetSupplyOffer(requestContext, viewerFrom(requestContext), input.ResourceKey, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPage]{Body: page}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setColonistOffer",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/market/colonists/{roleKey}",
		Summary:     "Price a role's colonist offer, putting it on sale",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *roleBodyInput[core.ColonistOfferInput]) (*bodyOutput[core.MarketPage], error) {
		page, err := handlers.core.SetColonistOffer(requestContext, viewerFrom(requestContext), input.RoleKey, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPage]{Body: page}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "setCartQuantity",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/market/cart/{itemId}",
		Summary:     "Set how many of an item the party's cart holds; 0 removes it",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *marketItemBodyInput[core.CartQuantityInput]) (*bodyOutput[core.MarketPage], error) {
		page, err := handlers.core.SetCartQuantity(requestContext, viewerFrom(requestContext), input.ItemID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPage]{Body: page}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID:   "placeMarketOrder",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/market/orders",
		Summary:       "Send the cart to the mainland; it docks after the voyage and is paid for then",
		Tags:          []string{"market"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.PlaceMarketOrderInput]) (*bodyOutput[core.MarketPage], error) {
		page, err := handlers.core.PlaceMarketOrder(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPage]{Body: page}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "previewPayMarketOrder",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/market/orders/{orderId}/pay-preview",
		Summary:     "What paying for a docked order would change, without writing",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *marketOrderPath) (*bodyOutput[core.MarketPayPreview], error) {
		preview, err := handlers.core.PreviewPayMarketOrder(requestContext, viewerFrom(requestContext), input.OrderID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPayPreview]{Body: preview}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "payMarketOrder",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/market/orders/{orderId}/pay",
		Summary:     "Pay for a docked order in full and unload it; the DM may undo it",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *marketOrderBodyInput[core.PayMarketOrderInput]) (*bodyOutput[core.ColonyChange], error) {
		change, err := handlers.core.PayMarketOrder(requestContext, viewerFrom(requestContext), input.OrderID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "cancelMarketOrder",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/market/orders/{orderId}/cancel",
		Summary:     "Cancel the open order; nothing was charged",
		Tags:        []string{"market"},
	}, func(requestContext context.Context, input *marketOrderPath) (*bodyOutput[core.MarketPage], error) {
		page, err := handlers.core.CancelMarketOrder(requestContext, viewerFrom(requestContext), input.OrderID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MarketPage]{Body: page}, nil
	})
}
