-- The Foundry bridge (docs/design/foundry-bridge.md): links, pairing codes, the outbox and inbound
-- events. Two reads find a campaign without knowing it first: the token and the pairing code. Every
-- instant comes from core's clock as @now.

-- EnterBridge: a live token of an ACTIVE campaign; a revoked one or a paused campaign's reads as unknown.
-- unscoped: bridge token lookup
-- name: GetBridgeLinkByTokenHash :one
SELECT sqlc.embed(bridge_link)
FROM bridge_link
JOIN campaign ON campaign.id = bridge_link.campaign_id
WHERE bridge_link.token_hash = @token_hash AND bridge_link.revoked_at IS NULL AND campaign.status = 'ACTIVE';

-- Pair uses the code up in the same statement that finds it, so two tabs can't both trade one code.
-- unscoped: a pairing code names its campaign
-- name: ClaimBridgePairingCode :one
UPDATE bridge_pairing_code SET closed_at = @now::timestamptz
WHERE code_hash = @code_hash AND closed_at IS NULL AND expires_at > @now::timestamptz
  AND EXISTS (SELECT 1 FROM campaign WHERE campaign.id = bridge_pairing_code.campaign_id AND campaign.status = 'ACTIVE')
RETURNING campaign_id;

-- A transaction's one check before it reads anything for Foundry.
-- name: HasActiveBridgeLink :one
SELECT EXISTS (SELECT 1 FROM bridge_link WHERE campaign_id = @campaign_id AND revoked_at IS NULL);

-- name: GetActiveBridgeLink :one
SELECT * FROM bridge_link WHERE campaign_id = @campaign_id AND revoked_at IS NULL;

-- An event batch locks its own live link first, so concurrent batches count the inbox one at a time
-- and a link revoked meanwhile writes nothing.
-- name: LockLiveBridgeLink :one
SELECT id FROM bridge_link WHERE campaign_id = @campaign_id AND id = @id AND revoked_at IS NULL FOR UPDATE;

-- name: TouchBridgeLinkContact :exec
UPDATE bridge_link SET last_contact_on = @today::date
WHERE campaign_id = @campaign_id AND id = @id AND (last_contact_on IS NULL OR last_contact_on < @today::date);

-- name: RevokeBridgeLinks :many
UPDATE bridge_link SET revoked_at = @now::timestamptz
WHERE campaign_id = @campaign_id AND revoked_at IS NULL
RETURNING id;

-- The next link carries the campaign's outbox sequence on, so it never goes back.
-- name: CampaignBridgeOutboxSeq :one
SELECT coalesce(max(outbox_seq), 0)::bigint FROM bridge_link WHERE campaign_id = @campaign_id;

-- name: CreateBridgeLink :one
INSERT INTO bridge_link (campaign_id, token_hash, origin, linked_at, outbox_seq)
VALUES (@campaign_id, @token_hash, @origin, @now::timestamptz, @outbox_seq)
RETURNING *;

-- name: SetBridgeLinkStatus :exec
UPDATE bridge_link
SET foundry_version = @foundry_version, system_id = @system_id, system_version = @system_version,
    module_version = @module_version, world_title = @world_title
WHERE campaign_id = @campaign_id AND id = @id;

-- name: CreateBridgePairingCode :one
INSERT INTO bridge_pairing_code (campaign_id, code_hash, created_at, expires_at)
VALUES (@campaign_id, @code_hash, @now::timestamptz, @expires_at)
RETURNING expires_at;

-- name: CloseOpenBridgePairingCodes :exec
UPDATE bridge_pairing_code SET closed_at = @now::timestamptz WHERE campaign_id = @campaign_id AND closed_at IS NULL;

-- name: CountBridgePairingCodesSince :one
SELECT count(*)::integer FROM bridge_pairing_code WHERE campaign_id = @campaign_id AND created_at > @since::timestamptz;

-- name: PruneBridgePairingCodes :exec
DELETE FROM bridge_pairing_code WHERE campaign_id = @campaign_id AND created_at < @before::timestamptz;

-- name: GetOpenBridgePairingCode :one
SELECT expires_at FROM bridge_pairing_code
WHERE campaign_id = @campaign_id AND closed_at IS NULL AND expires_at > @now::timestamptz;

-- Taking the next outbox number locks the active link, so a concurrent Pair waits for this change.
-- name: NextBridgeOutboxSeq :one
UPDATE bridge_link SET outbox_seq = outbox_seq + 1
WHERE campaign_id = @campaign_id AND revoked_at IS NULL
RETURNING outbox_seq;

-- A transaction's rows take consecutive numbers ending at the one returned.
-- name: ReserveBridgeOutboxSeqs :one
UPDATE bridge_link SET outbox_seq = outbox_seq + @row_count::bigint
WHERE campaign_id = @campaign_id AND revoked_at IS NULL
RETURNING outbox_seq;

-- name: DeleteUnappliedBridgeOutboxTargets :execrows
DELETE FROM bridge_outbox
WHERE campaign_id = @campaign_id AND target_key = ANY(@target_keys::text[]) AND applied_at IS NULL AND failed_at IS NULL;

-- name: DeleteUnappliedBridgeOutboxKinds :execrows
DELETE FROM bridge_outbox
WHERE campaign_id = @campaign_id AND kind::text = ANY(@kinds::text[]) AND applied_at IS NULL AND failed_at IS NULL;

-- name: CreateBridgeOutboxRows :exec
INSERT INTO bridge_outbox (campaign_id, seq, kind, target_key, payload, created_at)
SELECT @campaign_id, unnest(@seqs::bigint[]), unnest(@kinds::text[])::bridge_outbox_kind,
       unnest(@target_keys::text[]), unnest(@payloads::text[])::jsonb, @now::timestamptz;

-- name: DeleteUnappliedBridgeOutboxTarget :execrows
DELETE FROM bridge_outbox
WHERE campaign_id = @campaign_id AND target_key = @target_key AND applied_at IS NULL AND failed_at IS NULL;

-- name: CreateBridgeOutboxRow :exec
INSERT INTO bridge_outbox (campaign_id, seq, kind, target_key, payload, created_at)
VALUES (@campaign_id, @seq, @kind, @target_key, @payload, @now::timestamptz);

-- Past the cap, the oldest one-shot rows go first, but the newest min_one_shots always stay, however
-- many state rows wait. A state row is its target's only waiting row, so trimming one would lose that
-- target until it next changes (docs/design/foundry-bridge.md §4.1).
-- name: TrimBridgeOutbox :execrows
DELETE FROM bridge_outbox
WHERE bridge_outbox.campaign_id = @campaign_id AND bridge_outbox.id IN (
    SELECT oldest.id FROM bridge_outbox AS oldest
    WHERE oldest.campaign_id = @campaign_id AND oldest.applied_at IS NULL AND oldest.failed_at IS NULL
      AND oldest.kind::text = ANY(@one_shot_kinds::text[])
    ORDER BY oldest.seq
    LIMIT greatest(0, (
        SELECT count(*) FROM bridge_outbox AS one_shot
        WHERE one_shot.campaign_id = @campaign_id AND one_shot.applied_at IS NULL AND one_shot.failed_at IS NULL
          AND one_shot.kind::text = ANY(@one_shot_kinds::text[])
    ) - greatest(sqlc.arg(min_one_shots)::integer, sqlc.arg(keep)::integer - (
        SELECT count(*) FROM bridge_outbox AS state
        WHERE state.campaign_id = @campaign_id AND state.applied_at IS NULL AND state.failed_at IS NULL
          AND NOT (state.kind::text = ANY(@one_shot_kinds::text[]))
    )))
);

-- name: ListBridgeOutbox :many
SELECT seq, kind, target_key, payload FROM bridge_outbox
WHERE campaign_id = @campaign_id AND seq > @after::bigint AND applied_at IS NULL AND failed_at IS NULL
ORDER BY seq
LIMIT @row_limit::integer;

-- name: NewestUnappliedBridgeSeq :one
SELECT coalesce(max(seq), 0)::bigint FROM bridge_outbox
WHERE campaign_id = @campaign_id AND applied_at IS NULL AND failed_at IS NULL;

-- name: CountUnappliedBridgeOutbox :one
SELECT count(*)::integer FROM bridge_outbox
WHERE campaign_id = @campaign_id AND applied_at IS NULL AND failed_at IS NULL;

-- name: FailBridgeOutboxRow :execrows
UPDATE bridge_outbox SET failed_at = @now::timestamptz, failure_reason = @failure_reason
WHERE campaign_id = @campaign_id AND seq = @seq AND applied_at IS NULL AND failed_at IS NULL;

-- name: ApplyBridgeOutboxThrough :execrows
UPDATE bridge_outbox SET applied_at = @now::timestamptz
WHERE campaign_id = @campaign_id AND seq <= @through::bigint AND applied_at IS NULL AND failed_at IS NULL;

-- name: PruneBridgeOutbox :execrows
DELETE FROM bridge_outbox
WHERE campaign_id = @campaign_id AND (applied_at < @before::timestamptz OR failed_at < @before::timestamptz);

-- name: ClearFailedBridgeOutbox :execrows
DELETE FROM bridge_outbox WHERE campaign_id = @campaign_id AND failed_at IS NOT NULL;

-- name: ListFailedBridgeOutbox :many
SELECT seq, kind, failure_reason, failed_at FROM bridge_outbox
WHERE campaign_id = @campaign_id AND failed_at IS NOT NULL
ORDER BY seq DESC
LIMIT @row_limit::integer;

-- A repeat of a module's event ID inserts nothing.
-- name: CreateBridgeEvent :execrows
INSERT INTO bridge_event (campaign_id, module_event_id, kind, status, occurred_at, received_at, days)
VALUES (@campaign_id, @module_event_id, @kind, @status, @occurred_at, @now::timestamptz, sqlc.narg(days))
ON CONFLICT (campaign_id, module_event_id) DO NOTHING;

-- Pending items wait for the DM however old they are; decided ones are kept 30 days past the decision
-- only to ignore repeats. A STATUS row is decided on arrival, so it has no decided_at.
-- name: PruneBridgeEvents :exec
DELETE FROM bridge_event
WHERE campaign_id = @campaign_id AND status <> 'PENDING' AND coalesce(decided_at, received_at) < @before::timestamptz;

-- name: TrimBridgeEvents :exec
DELETE FROM bridge_event
WHERE bridge_event.campaign_id = @campaign_id AND bridge_event.id IN (
    SELECT decided.id FROM bridge_event AS decided
    WHERE decided.campaign_id = @campaign_id AND decided.status <> 'PENDING'
    ORDER BY coalesce(decided.decided_at, decided.received_at) DESC, decided.id DESC
    OFFSET @keep::integer
);

-- name: CountPendingBridgeEvents :one
SELECT count(*)::integer FROM bridge_event WHERE campaign_id = @campaign_id AND status = 'PENDING';

-- name: ListPendingBridgeEvents :many
SELECT id, kind, received_at, days FROM bridge_event
WHERE campaign_id = @campaign_id AND status = 'PENDING'
ORDER BY received_at, id
LIMIT @row_limit::integer;

-- name: GetBridgeEventStatus :one
SELECT status FROM bridge_event WHERE campaign_id = @campaign_id AND id = @id;

-- name: DecideBridgeEvent :execrows
UPDATE bridge_event SET status = @status, decided_at = @now::timestamptz
WHERE campaign_id = @campaign_id AND id = @id AND kind = 'TIME_ADVANCED' AND status = 'PENDING';

-- Undoing the Advance that accepted an item puts it back in the inbox; a pruned item stays gone.
-- name: ReopenAcceptedBridgeEvent :execrows
UPDATE bridge_event SET status = 'PENDING', decided_at = NULL
WHERE campaign_id = @campaign_id AND id = @id AND kind = 'TIME_ADVANCED' AND status = 'ACCEPTED';
