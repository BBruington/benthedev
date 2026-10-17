-- Request reads take dm_projection (viewer.SeesDmFields()). Players see every party request, except one
-- whose patron or action is hidden now, or was hidden when it was made: a hidden row never reaches them,
-- not even through the snapshot of its name (spec §5.9). Archiving hides neither, so history stays
-- intact. The decision note is filtered per viewer in core (only the requester and the DM read it).

-- name: ListRequests :many
SELECT sqlc.embed(request),
       requester.character_name AS requester_character_name,
       requester_account.display_name AS requester_display_name,
       beneficiary.character_name AS on_behalf_of_character_name,
       beneficiary_account.display_name AS on_behalf_of_display_name
FROM request
JOIN campaign_member AS requester ON requester.campaign_id = request.campaign_id AND requester.id = request.requested_by
JOIN account AS requester_account ON requester_account.id = requester.account_id
LEFT JOIN campaign_member AS beneficiary ON beneficiary.campaign_id = request.campaign_id AND beneficiary.id = request.on_behalf_of
LEFT JOIN account AS beneficiary_account ON beneficiary_account.id = beneficiary.account_id
WHERE request.campaign_id = @campaign_id
  AND (sqlc.narg(status)::request_status IS NULL OR request.status = sqlc.narg(status))
  AND (sqlc.narg(requested_by)::uuid IS NULL OR request.requested_by = sqlc.narg(requested_by))
  AND (sqlc.narg(patron_id)::uuid IS NULL OR request.patron_id = sqlc.narg(patron_id))
  AND (@dm_projection::boolean OR (request.made_while_revealed
       AND (request.patron_id IS NULL OR EXISTS (SELECT 1 FROM patron WHERE patron.campaign_id = @campaign_id AND patron.id = request.patron_id AND patron.revealed))
       AND (request.patron_action_id IS NULL OR EXISTS (SELECT 1 FROM patron_action WHERE patron_action.campaign_id = @campaign_id
            AND patron_action.id = request.patron_action_id AND patron_action.revealed))))
ORDER BY request.status <> 'PENDING', request.created_at DESC, request.id
LIMIT @result_limit;

-- name: GetRequestView :one
SELECT sqlc.embed(request),
       requester.character_name AS requester_character_name,
       requester_account.display_name AS requester_display_name,
       beneficiary.character_name AS on_behalf_of_character_name,
       beneficiary_account.display_name AS on_behalf_of_display_name
FROM request
JOIN campaign_member AS requester ON requester.campaign_id = request.campaign_id AND requester.id = request.requested_by
JOIN account AS requester_account ON requester_account.id = requester.account_id
LEFT JOIN campaign_member AS beneficiary ON beneficiary.campaign_id = request.campaign_id AND beneficiary.id = request.on_behalf_of
LEFT JOIN account AS beneficiary_account ON beneficiary_account.id = beneficiary.account_id
WHERE request.campaign_id = @campaign_id
  AND request.id = @id
  AND (@dm_projection::boolean OR (request.made_while_revealed
       AND (request.patron_id IS NULL OR EXISTS (SELECT 1 FROM patron WHERE patron.campaign_id = @campaign_id AND patron.id = request.patron_id AND patron.revealed))
       AND (request.patron_action_id IS NULL OR EXISTS (SELECT 1 FROM patron_action WHERE patron_action.campaign_id = @campaign_id
            AND patron_action.id = request.patron_action_id AND patron_action.revealed))));

-- DM reads, decisions and Beny Bot's buttons: a request ID from a body or a button is loaded with the
-- viewer's campaign, so another campaign's request is simply not found.
-- name: GetRequest :one
SELECT * FROM request WHERE campaign_id = @campaign_id AND id = @id;

-- name: CreateRequest :one
INSERT INTO request (
    campaign_id, kind, title, patron_id, patron_name, patron_action_id, cost, effects, requested_by,
    on_behalf_of, requested_day, note, made_while_revealed
) VALUES (
    @campaign_id, @kind, @title, @patron_id, @patron_name, @patron_action_id, @cost, @effects, @requested_by,
    sqlc.narg(on_behalf_of), @requested_day, @note, @made_while_revealed
)
RETURNING *;

-- The status guard of every decision (spec §5.1): no row means someone decided first.
-- name: ApproveRequest :one
UPDATE request
SET status = 'APPROVED', applied_effects = sqlc.narg(applied_effects), decided_by = @decided_by, decided_at = now(),
    decision_note = @decision_note, event_group_id = @event_group_id
WHERE campaign_id = @campaign_id AND id = @id AND status = 'PENDING'
RETURNING *;

-- name: DeclineRequest :one
UPDATE request
SET status = 'DECLINED', decided_by = @decided_by, decided_at = now(), decision_note = @decision_note
WHERE campaign_id = @campaign_id AND id = @id AND status = 'PENDING'
RETURNING *;

-- name: WithdrawRequest :one
UPDATE request
SET status = 'WITHDRAWN', decided_at = now()
WHERE campaign_id = @campaign_id AND id = @id AND requested_by = @requested_by AND status = 'PENDING'
RETURNING *;

-- Undo of an approval: the request waits again, as it did before.
-- name: ReopenApprovedRequest :exec
UPDATE request
SET status = 'PENDING', applied_effects = NULL, decided_by = NULL, decided_at = NULL, decision_note = '', event_group_id = NULL
WHERE campaign_id = @campaign_id AND id = @id AND status = 'APPROVED';

-- Undo of "Use action now": the request did not exist before the change.
-- name: DeleteApprovedRequest :exec
DELETE FROM request WHERE campaign_id = @campaign_id AND id = @id AND status = 'APPROVED';

-- Beny Bot stores the message it posted, so later decisions can edit it.
-- The status after the ID is stored: a decision that committed first saw no message to edit.
-- name: SetRequestDiscordMessage :one
UPDATE request SET discord_message_id = @discord_message_id WHERE campaign_id = @campaign_id AND id = @id
RETURNING status;
