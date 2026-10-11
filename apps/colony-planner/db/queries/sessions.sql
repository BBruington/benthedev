-- name: CreateSession :one
INSERT INTO web_session (token_hash, member_id, expires_at)
VALUES (@token_hash, @member_id, @expires_at)
RETURNING *;

-- name: GetSessionWithMember :one
SELECT sqlc.embed(web_session), sqlc.embed(member)
FROM web_session
JOIN member ON member.id = web_session.member_id
WHERE web_session.token_hash = $1 AND web_session.expires_at > now();

-- Slides the expiry forward; the middleware calls it at most once a day per session.
-- name: TouchSession :exec
UPDATE web_session SET last_seen_at = now(), expires_at = @expires_at WHERE token_hash = @token_hash;

-- name: SetSessionViewAsPlayer :execrows
UPDATE web_session SET view_as_player = @view_as_player WHERE token_hash = @token_hash;

-- name: DeleteSession :exec
DELETE FROM web_session WHERE token_hash = $1;

-- name: DeleteMemberSessions :exec
DELETE FROM web_session WHERE member_id = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM web_session WHERE expires_at <= now();
