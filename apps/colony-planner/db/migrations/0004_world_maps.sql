-- +goose Up

-- World maps (spec §4.4, §5.10, §8.9; docs/design/multi-campaign.md §2.6). Every table is campaign-owned
-- and every link between campaign rows is a composite foreign key, so it can never point into another
-- campaign. No map, image or hex is inserted here: each DM uploads their own Mainland art (C6), and the
-- players chart the hex map themselves.

-- The uploaded art: map images and NPC portraits. Core sniffs the type from the bytes and reads the size
-- from the header, so the checks here only back it up. A copy is shared within its campaign, never across.
CREATE TABLE image (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    content_type text NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'image/webp', 'image/gif')),
    bytes bytea NOT NULL CHECK (octet_length(bytes) BETWEEN 1 AND 5242880),
    sha256 bytea NOT NULL CHECK (octet_length(sha256) = 32),
    width integer NOT NULL CHECK (width > 0),
    height integer NOT NULL CHECK (height > 0),
    uploaded_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    CONSTRAINT image_campaign_sha256_key UNIQUE (campaign_id, sha256),
    CONSTRAINT image_uploaded_by_fkey FOREIGN KEY (campaign_id, uploaded_by)
        REFERENCES campaign_member (campaign_id, id) ON DELETE SET NULL (uploaded_by)
);

-- The terrains of the players' hex map (spec §5.10).
CREATE TYPE terrain AS ENUM ('FOREST', 'PLAINS', 'WATER', 'MOUNTAIN', 'DESERT', 'SWAMP', 'TUNDRA', 'VOLCANIC');

-- An image map. Its art is this campaign's own upload or an https address; never the bundled
-- /eldari.webp, which serves only the sign-in page (C6). width and height are the art's natural pixels.
CREATE TABLE map (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name <> '' AND length(name) <= 80),
    image_id uuid,
    external_url text CHECK (external_url LIKE 'https://%' AND length(external_url) <= 2000),
    width integer NOT NULL CHECK (width > 0),
    height integer NOT NULL CHECK (height > 0),
    is_mainland boolean NOT NULL DEFAULT false,
    -- Players saw every region map before, so maps start visible (spec §5.9, Open Question 9).
    revealed boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, id),
    -- NO ACTION: core deletes an image only once nothing uses it, and a campaign delete takes both.
    CONSTRAINT map_image_fkey FOREIGN KEY (campaign_id, image_id) REFERENCES image (campaign_id, id),
    CONSTRAINT map_one_source CHECK ((image_id IS NULL) <> (external_url IS NULL))
);

CREATE UNIQUE INDEX map_one_mainland ON map (campaign_id) WHERE is_mainland;
CREATE INDEX map_campaign_image_idx ON map (campaign_id, image_id) WHERE image_id IS NOT NULL;

-- The players' hex map: pointy-top axial, no row means uncharted, and no attribution or history (F6).
-- The bound on q and r only refuses bad input; the map itself has no edge (decision 9).
CREATE TABLE hex (
    q integer NOT NULL CHECK (q BETWEEN -100000 AND 100000),
    r integer NOT NULL CHECK (r BETWEEN -100000 AND 100000),
    terrain terrain NOT NULL,
    label text CHECK (label <> '' AND length(label) <= 40),
    updated_at timestamptz NOT NULL DEFAULT now(),
    campaign_id uuid NOT NULL REFERENCES campaign (id) ON DELETE CASCADE,
    PRIMARY KEY (campaign_id, q, r)
);

-- Place pins, which M2 left out. A pin is normalized to 0..1 of the map's art (y from the top), so it
-- stays put when the art is replaced. Deleting a map clears only map_id; core clears the pin with it.
ALTER TABLE place
    ADD COLUMN map_id uuid,
    ADD COLUMN pin_x real CHECK (pin_x BETWEEN 0 AND 1),
    ADD COLUMN pin_y real CHECK (pin_y BETWEEN 0 AND 1),
    ADD CONSTRAINT place_map_fkey FOREIGN KEY (campaign_id, map_id)
        REFERENCES map (campaign_id, id) ON DELETE SET NULL (map_id),
    ADD CONSTRAINT place_pin_both CHECK ((pin_x IS NULL) = (pin_y IS NULL));

CREATE INDEX place_campaign_map_idx ON place (campaign_id, map_id) WHERE map_id IS NOT NULL;

-- NPC portraits (spec's npc.avatar_url), which M2 left out: one of this campaign's images.
ALTER TABLE npc
    ADD COLUMN portrait_image_id uuid,
    ADD CONSTRAINT npc_portrait_image_fkey FOREIGN KEY (campaign_id, portrait_image_id)
        REFERENCES image (campaign_id, id);

CREATE INDEX npc_campaign_portrait_image_idx ON npc (campaign_id, portrait_image_id) WHERE portrait_image_id IS NOT NULL;

-- +goose Down

ALTER TABLE npc DROP COLUMN portrait_image_id;
ALTER TABLE place DROP CONSTRAINT place_pin_both, DROP COLUMN pin_y, DROP COLUMN pin_x, DROP COLUMN map_id;
DROP TABLE hex;
DROP TABLE map;
DROP TYPE terrain;
DROP TABLE image;
