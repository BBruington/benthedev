-- Uploaded art (spec §8.9). Players read an image only while something they can see uses it: a revealed
-- map, or the portrait of a revealed NPC. The DM reads every image of the campaign.

-- name: GetImageView :one
SELECT * FROM image
WHERE image.campaign_id = @campaign_id
  AND image.id = @id
  AND (@dm_projection::boolean
       OR EXISTS (SELECT 1 FROM map WHERE map.campaign_id = image.campaign_id AND map.image_id = image.id AND map.revealed)
       OR EXISTS (SELECT 1 FROM npc WHERE npc.campaign_id = image.campaign_id AND npc.portrait_image_id = image.id AND npc.revealed));

-- An image ID from a request body is loaded here with the viewer's campaign, so another campaign's
-- image is simply not found. The bytes stay behind.
-- name: GetImageInfo :one
SELECT id, content_type, width, height, octet_length(bytes)::integer AS byte_size
FROM image WHERE campaign_id = @campaign_id AND id = @id;

-- The same bytes uploaded again hand out the stored image, so an unused one gets a full hour again before
-- the sweep below: created_at is when the image was last handed out.
-- name: ReofferImageBySha256 :one
UPDATE image SET created_at = now()
WHERE campaign_id = @campaign_id AND sha256 = @sha256
RETURNING id, content_type, width, height, octet_length(bytes)::integer AS byte_size;

-- The 50 MB budget counts every stored byte of the campaign's images.
-- name: SumImageBytes :one
SELECT coalesce(sum(octet_length(bytes)), 0)::bigint FROM image WHERE campaign_id = @campaign_id;

-- name: CreateImage :one
INSERT INTO image (campaign_id, content_type, bytes, sha256, width, height, uploaded_by)
VALUES (@campaign_id, @content_type, @bytes, @sha256, @width, @height, @uploaded_by)
RETURNING id, content_type, width, height, octet_length(bytes)::integer AS byte_size;

-- Deletes the image once no map and no portrait uses it, so the budget counts only art in use.
-- name: DeleteImageIfUnused :execrows
DELETE FROM image
WHERE image.campaign_id = @campaign_id
  AND image.id = @id
  AND NOT EXISTS (SELECT 1 FROM map WHERE map.campaign_id = image.campaign_id AND map.image_id = image.id)
  AND NOT EXISTS (SELECT 1 FROM npc WHERE npc.campaign_id = image.campaign_id AND npc.portrait_image_id = image.id);

-- An upload nothing took up within the hour (a form closed after its upload) stops counting against the
-- budget: each new upload sweeps them, so no timer has to.
-- name: DeleteStaleUnusedImages :execrows
DELETE FROM image
WHERE image.campaign_id = @campaign_id
  AND image.created_at < now() - interval '1 hour'
  AND NOT EXISTS (SELECT 1 FROM map WHERE map.campaign_id = image.campaign_id AND map.image_id = image.id)
  AND NOT EXISTS (SELECT 1 FROM npc WHERE npc.campaign_id = image.campaign_id AND npc.portrait_image_id = image.id);
