-- Polls (spec §3.5): the DM's questions, their options and each player's vote. A poll is closed once
-- closed_at is set or closes_at has passed, so every read that splits open from closed takes @now.
-- Discord votes from anyone who isn't an active player are kept only as counts.

-- name: ListOpenPolls :many
SELECT * FROM poll
WHERE campaign_id = @campaign_id AND closed_at IS NULL AND closes_at > @now::timestamptz
ORDER BY created_at DESC, id DESC;

-- An overdue poll nothing has closed yet sorts by its deadline, the moment it closed.
-- name: ListClosedPolls :many
SELECT * FROM poll
WHERE campaign_id = @campaign_id AND (closed_at IS NOT NULL OR closes_at <= @now::timestamptz)
ORDER BY COALESCE(closed_at, closes_at) DESC, created_at DESC, id DESC
LIMIT @row_limit;

-- name: CountOpenPolls :one
SELECT count(*) FROM poll
WHERE campaign_id = @campaign_id AND closed_at IS NULL AND closes_at > @now::timestamptz;

-- name: GetPoll :one
SELECT * FROM poll WHERE campaign_id = @campaign_id AND id = @id;

-- Votes and closes both take this lock, so one player's two clicks can't interleave and a vote sees a close.
-- name: GetPollForUpdate :one
SELECT * FROM poll WHERE campaign_id = @campaign_id AND id = @id FOR NO KEY UPDATE;

-- name: CreatePoll :one
INSERT INTO poll (campaign_id, question, allow_multiple, voting, closes_at)
VALUES (@campaign_id, @question, @allow_multiple, @voting, @closes_at)
RETURNING *;

-- name: CreatePollOption :exec
INSERT INTO poll_option (campaign_id, poll_id, position, label)
VALUES (@campaign_id, @poll_id, @position, @label);

-- name: ListPollOptions :many
SELECT * FROM poll_option
WHERE campaign_id = @campaign_id AND poll_id = ANY(@poll_ids::uuid[])
ORDER BY poll_id, position;

-- Voters in the order they first voted, each named as on /players; one sync's voters tie, so their names order them.
-- name: ListPollVotes :many
SELECT poll_vote.poll_id, poll_vote.option_id, poll_vote.voter,
       voter.character_name AS voter_character_name,
       voter_account.display_name AS voter_display_name
FROM poll_vote
JOIN campaign_member AS voter ON voter.campaign_id = poll_vote.campaign_id AND voter.id = poll_vote.voter
JOIN account AS voter_account ON voter_account.id = voter.account_id
WHERE poll_vote.campaign_id = @campaign_id AND poll_vote.poll_id = ANY(@poll_ids::uuid[])
ORDER BY poll_vote.poll_id, poll_vote.created_at,
  COALESCE(NULLIF(voter.character_name, ''), voter_account.display_name), poll_vote.voter, poll_vote.option_id;

-- name: CountPollVotesByOption :many
SELECT poll_option.label, (count(poll_vote.voter) + poll_option.discord_other_votes)::bigint AS votes
FROM poll_option
LEFT JOIN poll_vote ON poll_vote.campaign_id = poll_option.campaign_id
  AND poll_vote.poll_id = poll_option.poll_id AND poll_vote.option_id = poll_option.id
WHERE poll_option.campaign_id = @campaign_id AND poll_option.poll_id = @poll_id
GROUP BY poll_option.id, poll_option.label, poll_option.position, poll_option.discord_other_votes
ORDER BY poll_option.position;

-- Options still in the vote keep their row, so voters stay in the order they first picked them.
-- name: DeleteMemberPollVotes :exec
DELETE FROM poll_vote
WHERE campaign_id = @campaign_id AND poll_id = @poll_id AND voter = @voter
  AND option_id <> ALL(@keep_option_ids::uuid[]);

-- The vote's option key makes an option of another poll fail here too; core checks first to name it.
-- name: InsertMemberPollVotes :exec
INSERT INTO poll_vote (campaign_id, poll_id, option_id, voter)
SELECT @campaign_id, @poll_id, option_id, @voter
FROM unnest(@option_ids::uuid[]) AS option_id
ON CONFLICT DO NOTHING;

-- name: CountPollOptionsAmong :one
SELECT count(*) FROM poll_option
WHERE campaign_id = @campaign_id AND poll_id = @poll_id AND id = ANY(@option_ids::uuid[]);

-- name: ClosePoll :one
UPDATE poll SET closed_at = @closed_at::timestamptz
WHERE campaign_id = @campaign_id AND id = @id AND closed_at IS NULL
RETURNING *;

-- Core applies the close rule (a posted Discord poll waits out its grace), so this lists every candidate.
-- name: ListOverduePolls :many
SELECT id, (discord_message_id IS NOT NULL)::boolean AS posted, closes_at FROM poll
WHERE campaign_id = @campaign_id AND closed_at IS NULL AND closes_at <= @now::timestamptz
ORDER BY closes_at, id;

-- name: ListPollDeadlines :many
SELECT id, closes_at FROM poll
WHERE campaign_id = @campaign_id AND closed_at IS NULL
ORDER BY closes_at, id;

-- Only the first post on an open poll is recorded, so a second process's post during a deploy overlap
-- and a post that lost to a close are refused. So is a post in a server the campaign is no longer linked
-- to: unlink deletes the link row without touching poll, so the share lock makes an unlink in flight
-- finish first and the check then sees it.
-- name: RecordPollMessage :execrows
UPDATE poll SET discord_guild_id = @discord_guild_id, discord_channel_id = @discord_channel_id,
  discord_message_id = @discord_message_id, closes_at = @closes_at
WHERE poll.campaign_id = @campaign_id AND poll.id = @id AND poll.voting = 'DISCORD'
  AND poll.discord_message_id IS NULL AND poll.closed_at IS NULL
  AND EXISTS (
    SELECT 1 FROM discord_guild_link
    WHERE discord_guild_link.campaign_id = @campaign_id AND discord_guild_link.guild_id = @discord_guild_id
    FOR KEY SHARE
  );

-- A DISCORD poll the DM closed before its Discord poll expires, which Beny Bot still has to end.
-- name: ListDiscordPollsToEnd :many
SELECT id, discord_channel_id::text AS discord_channel_id, discord_message_id::text AS discord_message_id FROM poll
WHERE campaign_id = @campaign_id AND voting = 'DISCORD' AND closed_at IS NOT NULL
  AND discord_channel_id IS NOT NULL AND discord_message_id IS NOT NULL AND closes_at > @now::timestamptz
ORDER BY closes_at, id;

-- answer_ids is in option position order.
-- name: SetPollOptionAnswerIDs :exec
UPDATE poll_option SET discord_answer_id = (@answer_ids::integer[])[position + 1]
WHERE campaign_id = @campaign_id AND poll_id = @poll_id;

-- The campaign's active players among a Discord poll's voters; everyone else is counted, never stored.
-- name: ListPollVotingPlayers :many
SELECT campaign_member.id, account.discord_user_id
FROM campaign_member
JOIN account ON account.id = campaign_member.account_id
WHERE campaign_member.campaign_id = @campaign_id AND campaign_member.status = 'ACTIVE'
  AND campaign_member.role = 'PLAYER' AND account.discord_user_id = ANY(@discord_user_ids::text[]);

-- voters and option_ids are paired by index. A row still in the snapshot keeps its created_at, so
-- voters stay in the order they first picked.
-- name: DeletePollVotesOutside :execrows
DELETE FROM poll_vote
WHERE campaign_id = @campaign_id AND poll_id = @poll_id
  AND NOT EXISTS (
    SELECT 1 FROM generate_series(1, cardinality(@voters::uuid[])) AS slot
    WHERE (@voters::uuid[])[slot] = poll_vote.voter AND (@option_ids::uuid[])[slot] = poll_vote.option_id
  );

-- name: InsertPollVotes :execrows
INSERT INTO poll_vote (campaign_id, poll_id, voter, option_id)
SELECT @campaign_id, @poll_id, (@voters::uuid[])[slot], (@option_ids::uuid[])[slot]
FROM generate_series(1, cardinality(@voters::uuid[])) AS slot
ON CONFLICT DO NOTHING;

-- votes is in option position order.
-- name: SetPollOptionOtherVotes :execrows
UPDATE poll_option SET discord_other_votes = (@votes::integer[])[position + 1]
WHERE campaign_id = @campaign_id AND poll_id = @poll_id
  AND discord_other_votes <> (@votes::integer[])[position + 1];

-- name: SetPollOtherVoters :execrows
UPDATE poll SET discord_other_voters = @discord_other_voters
WHERE campaign_id = @campaign_id AND id = @id AND discord_other_voters <> @discord_other_voters;

-- name: DeletePoll :one
DELETE FROM poll WHERE campaign_id = @campaign_id AND id = @id
RETURNING discord_guild_id, discord_channel_id, discord_message_id;
