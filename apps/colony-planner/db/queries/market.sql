-- The market (spec §3.3): the standing offers and special items, the party's one shared cart and its orders.

-- name: ListMarketItems :many
SELECT * FROM market_item
WHERE campaign_id = @campaign_id
  AND (@dm_projection::boolean OR NOT archived)
ORDER BY archived, sort_order, lower(name), id;

-- name: GetMarketItem :one
SELECT * FROM market_item WHERE campaign_id = @campaign_id AND id = @id;

-- Only special items count: offers are one per resource and role, so they need no ceiling. Items are
-- never deleted, so archiving is the only way under MarketItemLimit; the total still has a ceiling.
-- name: CountMarketItems :one
SELECT count(*) FILTER (WHERE NOT archived) AS unarchived, count(*) AS total
FROM market_item WHERE campaign_id = @campaign_id AND resource_key IS NULL AND role_key IS NULL;

-- name: CreateMarketItem :one
INSERT INTO market_item (campaign_id, name, description, price_gold, max_per_order)
VALUES (@campaign_id, @name, @description, @price_gold, sqlc.narg(max_per_order))
RETURNING *;

-- PUT semantics for a special item; an offer's ID matches no row. Archived changes only through
-- SetMarketItemArchived.
-- name: UpdateMarketItem :one
UPDATE market_item
SET name = @name, description = @description, price_gold = @price_gold,
    max_per_order = sqlc.narg(max_per_order), updated_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND resource_key IS NULL AND role_key IS NULL
RETURNING *;

-- name: SetMarketItemArchived :one
UPDATE market_item SET archived = @archived, updated_at = now() WHERE campaign_id = @campaign_id AND id = @id RETURNING *;

-- Setting a price puts the offer on sale, so a stopped offer comes back.
-- name: UpsertSupplyOffer :one
INSERT INTO market_item (campaign_id, resource_key, amount, price_gold)
VALUES (@campaign_id, @resource_key, @amount, @price_gold)
ON CONFLICT (campaign_id, resource_key) WHERE resource_key IS NOT NULL DO UPDATE
SET amount = excluded.amount, price_gold = excluded.price_gold, archived = false, updated_at = now()
RETURNING *;

-- name: UpsertColonistOffer :one
INSERT INTO market_item (campaign_id, role_key, price_gold)
VALUES (@campaign_id, @role_key, @price_gold)
ON CONFLICT (campaign_id, role_key) WHERE role_key IS NOT NULL DO UPDATE
SET price_gold = excluded.price_gold, archived = false, updated_at = now()
RETURNING *;

-- A new campaign's staple supply offers, for whichever of their resources it has. They are data, not rules.
-- name: CreateDefaultSupplyOffers :exec
INSERT INTO market_item (campaign_id, resource_key, amount, price_gold)
SELECT resource.campaign_id, defaults.key, defaults.amount, defaults.price_gold
FROM (VALUES ('food', 2, 1), ('materials', 1, 5)) AS defaults (key, amount, price_gold)
JOIN resource ON resource.campaign_id = @campaign_id AND resource.key = defaults.key
WHERE NOT resource.archived;

-- A colonist offer for every role that has none yet.
-- name: CreateColonistOffers :exec
INSERT INTO market_item (campaign_id, role_key, price_gold)
SELECT role.campaign_id, role.key, @price_gold::integer FROM role WHERE role.campaign_id = @campaign_id
ON CONFLICT (campaign_id, role_key) WHERE role_key IS NOT NULL DO NOTHING;

-- name: ListCartLines :many
SELECT sqlc.embed(market_cart_line), sqlc.embed(market_item)
FROM market_cart_line
JOIN market_item ON market_item.campaign_id = market_cart_line.campaign_id AND market_item.id = market_cart_line.item_id
WHERE market_cart_line.campaign_id = @campaign_id
ORDER BY market_item.sort_order, lower(market_item.name), market_item.id;

-- name: UpsertCartLine :exec
INSERT INTO market_cart_line (campaign_id, item_id, quantity)
VALUES (@campaign_id, @item_id, @quantity)
ON CONFLICT (campaign_id, item_id) DO UPDATE SET quantity = excluded.quantity;

-- name: DeleteCartLine :execrows
DELETE FROM market_cart_line WHERE campaign_id = @campaign_id AND item_id = @item_id;

-- An item's lower limit trims the cart to it, so the cart stays orderable.
-- name: ClampCartLine :execrows
UPDATE market_cart_line SET quantity = @max_per_order
WHERE campaign_id = @campaign_id AND item_id = @item_id AND quantity > @max_per_order;

-- name: ClearCart :exec
DELETE FROM market_cart_line WHERE campaign_id = @campaign_id;

-- name: TouchMarketCart :exec
INSERT INTO market_cart (campaign_id, updated_by) VALUES (@campaign_id, @updated_by)
ON CONFLICT (campaign_id) DO UPDATE SET updated_by = excluded.updated_by;

-- name: GetMarketCartEditor :one
SELECT member.character_name, member_account.display_name
FROM market_cart
JOIN campaign_member AS member ON member.campaign_id = market_cart.campaign_id AND member.id = market_cart.updated_by
JOIN account AS member_account ON member_account.id = member.account_id
WHERE market_cart.campaign_id = @campaign_id;

-- The soonest to dock first, so the board shows the next ship and a docked one before any at sea.
-- name: ListOpenMarketOrders :many
SELECT sqlc.embed(market_order),
       placer.character_name AS placed_by_character_name,
       placer_account.display_name AS placed_by_display_name
FROM market_order
JOIN campaign_member AS placer ON placer.campaign_id = market_order.campaign_id AND placer.id = market_order.placed_by
JOIN account AS placer_account ON placer_account.id = placer.account_id
WHERE market_order.campaign_id = @campaign_id AND market_order.status = 'UNPAID'
ORDER BY market_order.arrives_day, market_order.created_at, market_order.id;

-- name: CountOpenMarketOrders :one
SELECT count(*)::integer FROM market_order WHERE campaign_id = @campaign_id AND status = 'UNPAID';

-- An order docks once: a day corrected back and advanced past it again writes no second entry, but an
-- undone advance's entry doesn't count, so advancing again docks it anew.
-- name: MarketOrderHasDocked :one
SELECT EXISTS (
    SELECT 1 FROM timeline_event
    WHERE campaign_id = @campaign_id AND kind = 'MARKET_ORDER_DOCKED' AND undone_at IS NULL
      AND data->>'orderId' = @order_id::text
);

-- name: GetMarketOrder :one
SELECT * FROM market_order WHERE campaign_id = @campaign_id AND id = @id;

-- Pay, cancel and undo lock the order after the colony.
-- name: GetMarketOrderForUpdate :one
SELECT * FROM market_order WHERE campaign_id = @campaign_id AND id = @id FOR NO KEY UPDATE;

-- name: CreateMarketOrder :one
INSERT INTO market_order (campaign_id, placed_day, arrives_day, placed_by, total_gold)
VALUES (@campaign_id, @placed_day, @arrives_day, @placed_by, @total_gold)
RETURNING *;

-- name: CreateMarketOrderLine :exec
INSERT INTO market_order_line (campaign_id, order_id, position, item_id, name, unit_price, quantity, resources, colonists, role_key)
VALUES (@campaign_id, @order_id, @position, @item_id, @name, @unit_price, @quantity, @resources, @colonists, sqlc.narg(role_key));

-- name: ListMarketOrderLines :many
SELECT * FROM market_order_line
WHERE campaign_id = @campaign_id AND order_id = ANY(@order_ids::uuid[])
ORDER BY order_id, position;

-- name: ListRecentMarketOrders :many
SELECT * FROM market_order
WHERE campaign_id = @campaign_id AND status <> 'UNPAID'
ORDER BY created_at DESC, id DESC
LIMIT 20;

-- name: PayMarketOrder :exec
UPDATE market_order
SET status = 'PAID', paid_day = @paid_day, paid_by = @paid_by, paid_group_id = @paid_group_id
WHERE campaign_id = @campaign_id AND id = @id AND status = 'UNPAID';

-- name: CancelMarketOrder :exec
UPDATE market_order
SET status = 'CANCELLED', cancelled_day = @cancelled_day
WHERE campaign_id = @campaign_id AND id = @id AND status = 'UNPAID';

-- Undoing a payment puts the ship back at the docks.
-- name: ReopenPaidMarketOrder :exec
UPDATE market_order
SET status = 'UNPAID', paid_day = NULL, paid_by = NULL, paid_group_id = NULL
WHERE campaign_id = @campaign_id AND id = @id AND status = 'PAID';

-- name: GetMarketSettings :one
SELECT market_voyage_days, market_ship_limit FROM settings WHERE campaign_id = @campaign_id;

-- name: SetMarketSettings :exec
UPDATE settings SET market_voyage_days = @market_voyage_days, market_ship_limit = @market_ship_limit
WHERE campaign_id = @campaign_id;
