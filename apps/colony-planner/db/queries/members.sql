-- name: GetMember :one
SELECT * FROM member WHERE id = $1;

-- name: GetMemberForUpdate :one
SELECT * FROM member WHERE id = $1 FOR UPDATE;

-- name: GetMemberByDiscordUserID :one
SELECT * FROM member WHERE discord_user_id = $1;

-- name: ListMembers :many
SELECT * FROM member ORDER BY created_at, id;

-- name: CountMembers :one
SELECT count(*) FROM member;

-- name: CountPendingMembers :one
SELECT count(*) FROM member WHERE status = 'PENDING';

-- Serializes first sign-ins for the rest of the transaction, so two people signing up at once can't
-- both slip under the pending cap.
-- name: LockMemberSignIns :exec
SELECT pg_advisory_xact_lock(hashtext('colony-planner:member-sign-in'));

-- name: CreateMember :one
INSERT INTO member (
    discord_user_id, discord_username, display_name, avatar_url, character_name, status,
    created_at, approved_at, last_seen_at
) VALUES (
    @discord_user_id, @discord_username, @display_name, sqlc.narg(avatar_url), sqlc.narg(character_name),
    @status, COALESCE(sqlc.narg(created_at)::timestamptz, now()), sqlc.narg(approved_at), sqlc.narg(last_seen_at)
)
RETURNING *;

-- Refreshes what Discord says about a returning member. Their status is kept, except that the DM is
-- always ACTIVE.
-- name: UpdateMemberFromDiscord :one
UPDATE member
SET discord_username = @discord_username,
    display_name = @display_name,
    avatar_url = sqlc.narg(avatar_url),
    status = CASE WHEN @make_active::boolean THEN 'ACTIVE'::member_status ELSE status END,
    approved_at = CASE WHEN @make_active::boolean THEN COALESCE(approved_at, now()) ELSE approved_at END,
    last_seen_at = now()
WHERE id = @id
RETURNING *;

-- name: SetMemberStatus :one
UPDATE member
SET status = @status,
    approved_at = CASE WHEN @status = 'ACTIVE'::member_status THEN now() ELSE approved_at END
WHERE id = @id
RETURNING *;

-- name: SetMemberCharacterName :one
UPDATE member SET character_name = sqlc.narg(character_name) WHERE id = @id RETURNING *;

-- name: TouchMemberLastSeen :exec
UPDATE member
SET last_seen_at = now()
WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < now() - interval '1 day');
