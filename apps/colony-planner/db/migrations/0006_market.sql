-- +goose Up

-- The market (spec §3.3): standing offers on supplies and colonists the DM prices, special items the
-- DM adds, one shared party cart, and orders that each sail for the campaign's voyage length and unload
-- when someone pays in full; the DM sets how many ships may be out at once. Items are archived, never
-- deleted, so an order's snapshot lines keep their link (NO ACTION, as patrons are in 0003).

-- At sea and docked are both UNPAID: docked is derived from the colony day.
CREATE TYPE market_order_status AS ENUM ('UNPAID', 'PAID', 'CANCELLED');

ALTER TABLE settings
    ADD COLUMN market_voyage_days integer NOT NULL DEFAULT 14
        CONSTRAINT settings_market_voyage_days_range CHECK (market_voyage_days BETWEEN 1 AND 3650),
    ADD COLUMN market_ship_limit integer NOT NULL DEFAULT 1
        CONSTRAINT settings_market_ship_limit_range CHECK (market_ship_limit BETWEEN 1 AND 100);

-- Three kinds in one table, so the cart and orders point at one thing: a supply offer (resource_key and
-- amount: "2 food for 1 gp"), a colonist offer (role_key: one colonist of that role per purchase) and a
-- special item (neither: a record only). Offers take their name from the resource or role.
CREATE TABLE market_item (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL DEFAULT '' CHECK (length(name) <= 80),
    description text NOT NULL DEFAULT '',
    price_gold integer NOT NULL CHECK (price_gold >= 0),
    resource_key text,
    -- How much of the resource one purchase brings.
    amount integer CHECK (amount BETWEEN 1 AND 1000000),
    role_key text,
    max_per_order integer CHECK (max_per_order >= 1),
    archived boolean NOT NULL DEFAULT false,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT market_item_one_kind CHECK (resource_key IS NULL OR role_key IS NULL),
    CONSTRAINT market_item_amount_with_resource CHECK ((resource_key IS NULL) = (amount IS NULL)),
    CONSTRAINT market_item_special_named CHECK (resource_key IS NOT NULL OR role_key IS NOT NULL OR name <> ''),
    CONSTRAINT market_item_offer_plain CHECK (
        (resource_key IS NULL AND role_key IS NULL)
        OR (name = '' AND description = '' AND max_per_order IS NULL)),
    CONSTRAINT market_item_resource_fkey FOREIGN KEY (campaign_id, resource_key) REFERENCES resource (campaign_id, key),
    CONSTRAINT market_item_role_fkey FOREIGN KEY (campaign_id, role_key) REFERENCES role (campaign_id, key)
);

CREATE INDEX market_item_campaign_sort_idx ON market_item (campaign_id, archived, sort_order, lower(name));
-- One offer per resource and per role: setting a price upserts it.
CREATE UNIQUE INDEX market_item_supply_offer ON market_item (campaign_id, resource_key) WHERE resource_key IS NOT NULL;
CREATE UNIQUE INDEX market_item_colonist_offer ON market_item (campaign_id, role_key) WHERE role_key IS NOT NULL;

CREATE TABLE market_cart_line (
    item_id uuid NOT NULL,
    quantity integer NOT NULL CHECK (quantity BETWEEN 1 AND 10000),
    updated_by uuid NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, item_id),
    CONSTRAINT market_cart_line_item_fkey FOREIGN KEY (campaign_id, item_id) REFERENCES market_item (campaign_id, id),
    CONSTRAINT market_cart_line_updated_by_fkey FOREIGN KEY (campaign_id, updated_by) REFERENCES campaign_member (campaign_id, id)
);

CREATE TABLE market_order (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    status market_order_status NOT NULL DEFAULT 'UNPAID',
    placed_day integer NOT NULL CHECK (placed_day >= 0),
    arrives_day integer NOT NULL,
    placed_by uuid NOT NULL,
    total_gold integer NOT NULL CHECK (total_gold >= 0),
    paid_day integer,
    paid_by uuid,
    -- The timeline group the payment wrote, so undo can reopen the order.
    paid_group_id uuid,
    cancelled_day integer,
    created_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT market_order_arrives_after_placed CHECK (arrives_day > placed_day),
    CONSTRAINT market_order_paid_whole CHECK (
        (status = 'PAID') = (paid_day IS NOT NULL)
        AND (paid_day IS NULL) = (paid_by IS NULL)
        AND (paid_day IS NULL) = (paid_group_id IS NULL)),
    CONSTRAINT market_order_cancelled_whole CHECK ((status = 'CANCELLED') = (cancelled_day IS NOT NULL)),
    CONSTRAINT market_order_placed_by_fkey FOREIGN KEY (campaign_id, placed_by) REFERENCES campaign_member (campaign_id, id),
    CONSTRAINT market_order_paid_by_fkey FOREIGN KEY (campaign_id, paid_by) REFERENCES campaign_member (campaign_id, id)
);

CREATE INDEX market_order_campaign_unpaid_idx ON market_order (campaign_id, arrives_day) WHERE status = 'UNPAID';
CREATE INDEX market_order_campaign_created_idx ON market_order (campaign_id, created_at);

-- A snapshot taken when the order is placed: later edits to the item never change it.
CREATE TABLE market_order_line (
    order_id uuid NOT NULL,
    position integer NOT NULL CHECK (position >= 0),
    item_id uuid NOT NULL,
    name text NOT NULL CHECK (name <> ''),
    unit_price integer NOT NULL CHECK (unit_price >= 0),
    quantity integer NOT NULL CHECK (quantity >= 1),
    resources jsonb NOT NULL DEFAULT '{}',
    colonists integer NOT NULL DEFAULT 0 CHECK (colonists >= 0),
    role_key text,
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, order_id, position),
    CONSTRAINT market_order_line_order_fkey FOREIGN KEY (campaign_id, order_id)
        REFERENCES market_order (campaign_id, id) ON DELETE CASCADE,
    CONSTRAINT market_order_line_item_fkey FOREIGN KEY (campaign_id, item_id) REFERENCES market_item (campaign_id, id),
    CONSTRAINT market_order_line_role_fkey FOREIGN KEY (campaign_id, role_key) REFERENCES role (campaign_id, key)
);

CREATE INDEX market_order_line_campaign_item_idx ON market_order_line (campaign_id, item_id);

-- Every existing campaign starts with the staple offers; new ones get them at approval
-- (CreateDefaultSupplyOffers, CreateColonistOffers) and a new role at creation. They are data, not
-- rules: the DM reprices or stops selling them. 30 gp is core's defaultColonistPriceGold.
INSERT INTO market_item (campaign_id, resource_key, amount, price_gold)
SELECT resource.campaign_id, defaults.key, defaults.amount, defaults.price_gold
FROM (VALUES ('food', 2, 1), ('materials', 1, 5)) AS defaults (key, amount, price_gold)
JOIN resource ON resource.key = defaults.key AND NOT resource.archived;

INSERT INTO market_item (campaign_id, role_key, price_gold)
SELECT role.campaign_id, role.key, 30 FROM role;

-- +goose Down

DROP TABLE market_order_line;
DROP TABLE market_order;
DROP TABLE market_cart_line;
DROP TABLE market_item;
DROP TYPE market_order_status;
ALTER TABLE settings DROP COLUMN market_ship_limit, DROP COLUMN market_voyage_days;
