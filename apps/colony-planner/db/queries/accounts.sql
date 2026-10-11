-- An account is a Discord identity; it belongs to no campaign, so these queries take none.

-- name: GetAccount :one
SELECT * FROM account WHERE id = $1;

-- name: GetAccountByDiscordUserID :one
SELECT * FROM account WHERE discord_user_id = $1;

-- Dev sign-in lists every account, oldest first.
-- name: ListAccounts :many
SELECT * FROM account ORDER BY created_at, id;

-- name: CountAccounts :one
SELECT count(*) FROM account;

-- A Discord sign-in creates the account or refreshes what Discord says about it. It makes nobody a
-- member of anything: joining is by invite link.
-- name: UpsertAccountFromDiscord :one
INSERT INTO account (discord_user_id, discord_username, display_name, avatar_url, last_seen_at)
VALUES (@discord_user_id, @discord_username, @display_name, sqlc.narg(avatar_url), now())
ON CONFLICT (discord_user_id) DO UPDATE
SET discord_username = EXCLUDED.discord_username,
    display_name = EXCLUDED.display_name,
    avatar_url = EXCLUDED.avatar_url,
    last_seen_at = now()
RETURNING *;

-- The seed backdates its sample accounts.
-- name: CreateAccount :one
INSERT INTO account (discord_user_id, discord_username, display_name, avatar_url, created_at, last_seen_at)
VALUES (
    @discord_user_id, @discord_username, @display_name, sqlc.narg(avatar_url),
    COALESCE(sqlc.narg(created_at)::timestamptz, now()), sqlc.narg(last_seen_at)
)
RETURNING *;

-- name: TouchAccountLastSeen :exec
UPDATE account
SET last_seen_at = now()
WHERE id = $1 AND (last_seen_at IS NULL OR last_seen_at < now() - interval '1 day');
