package core

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/BBruington/colony-planner/internal/game"
	"github.com/BBruington/colony-planner/internal/store"
)

// closedPollLimit is how many closed polls /polls lists, newest first.
const closedPollLimit = 50

var (
	ErrPollClosed      = Conflict("This poll is closed.")
	ErrOpenPollLimit   = Conflict("There are already 10 open polls. Close one before starting another.")
	ErrDmDoesNotVote   = Forbidden("The DM runs polls and doesn't vote.")
	ErrNoSuchPoll      = NotFound("That poll doesn't exist.")
	errForeignPollVote = Invalid("That option isn't on this poll.", FieldError{Field: "optionIds", Message: "That option isn't on this poll."})
	// ErrPollAlreadyPosted tells Beny Bot another post won the poll's message; it deletes its own.
	ErrPollAlreadyPosted = Conflict("That poll already has a Discord message.")
	ErrVoteInDiscord     = Conflict("Vote on this poll in Discord.")
	// ErrPollServerChanged tells Beny Bot the campaign's server changed while it posted; it deletes its post.
	ErrPollServerChanged = Conflict("The campaign's Discord server changed.")
	// ErrPollVotesInconsistent tells Beny Bot its answer reads raced a vote change; it reads again.
	ErrPollVotesInconsistent = Conflict("Discord's votes changed while they were read.")
)

// PollNotifier is Beny Bot's side of polls. Its methods run after commit and never block or fail the
// operation, the same contract as SessionPlanner.
type PollNotifier interface {
	// PollChanged runs after create and close commit: Beny Bot posts an open DISCORD poll that has no
	// message yet, ends a posted one closed early, and re-plans the deadline timer.
	PollChanged(campaignID uuid.UUID, pollID uuid.UUID)
	// PollDeleted runs after delete commits with the message the poll had (empty when site-only).
	PollDeleted(campaignID uuid.UUID, channelID string, messageID string)
}

// NoopPollNotifier is the PollNotifier while Beny Bot is off.
type NoopPollNotifier struct{}

func (NoopPollNotifier) PollChanged(uuid.UUID, uuid.UUID)      {}
func (NoopPollNotifier) PollDeleted(uuid.UUID, string, string) {}

// SetPollNotifier attaches Beny Bot once the bot exists, like SetRequestNotifier.
func (service *Service) SetPollNotifier(pollNotifier PollNotifier) {
	if pollNotifier == nil {
		pollNotifier = NoopPollNotifier{}
	}
	service.pollNotifier = pollNotifier
}

type Poll struct {
	ID            uuid.UUID    `json:"id" format:"uuid"`
	Question      string       `json:"question"`
	AllowMultiple bool         `json:"allowMultiple" doc:"True when a player may pick several options"`
	Voting        PollVoting   `json:"voting" doc:"Where players vote, fixed when the poll was created"`
	ClosesAt      time.Time    `json:"closesAt" doc:"When the poll closes by itself"`
	ClosedAt      *time.Time   `json:"closedAt" nullable:"true" doc:"When the poll closed; null while it is open"`
	Closed        bool         `json:"closed"`
	CreatedAt     time.Time    `json:"createdAt"`
	Options       []PollOption `json:"options" doc:"In the order the DM wrote them"`
	VoterCount    int64        `json:"voterCount" doc:"How many have voted: players, plus anyone else counted in Discord"`
	MyOptionIDs   []uuid.UUID  `json:"myOptionIds" format:"uuid" doc:"The viewer's own current vote; empty for the DM"`
	DiscordURL    *string      `json:"discordUrl" nullable:"true" doc:"The Discord poll's message; null for a site poll and until Beny Bot posts it"`
	// CloseRecorded is false for a poll past its deadline whose close (and POLL_CLOSED entry) isn't stored yet.
	CloseRecorded bool `json:"-"`
}

type PollOption struct {
	ID       uuid.UUID   `json:"id" format:"uuid"`
	Label    string      `json:"label"`
	Position int         `json:"position"`
	Count    int64       `json:"count" doc:"Every vote: the named players plus otherVotes"`
	Voters   []PollVoter `json:"voters" doc:"Players, in the order they voted"`
	// OtherVotes counts Discord votes from the DM and anyone who isn't a player here, never named.
	OtherVotes int64 `json:"otherVotes" doc:"Discord votes from anyone who isn't a player of this campaign"`
}

type PollVoter struct {
	Name string `json:"name"`
}

type PollList struct {
	Open   []Poll `json:"open" doc:"Newest first"`
	Closed []Poll `json:"closed" doc:"The most recently closed, at most 50"`
	// NewPollVoting tells the DM where players would vote on a poll created now.
	NewPollVoting PollVoting `json:"newPollVoting" doc:"Where players would vote on a poll created now"`
}

type CreatePollInput struct {
	Question      string   `json:"question" maxLength:"1000"`
	Options       []string `json:"options" maxItems:"20"`
	AllowMultiple bool     `json:"allowMultiple"`
	DurationHours int      `json:"durationHours" doc:"How many hours the poll stays open, 1 to 768"`
}

type PollDeadline struct {
	PollID   uuid.UUID
	ClosesAt time.Time
}

// DiscordPoll is a DISCORD poll's player projection and the Discord poll that shows it. The IDs are
// empty and AnswerIDs zero until Beny Bot posts it.
type DiscordPoll struct {
	Poll      Poll
	GuildID   string
	ChannelID string
	MessageID string
	// AnswerIDs are Discord's answer IDs, one per option in position order.
	AnswerIDs []int
}

// RecordPollMessageInput is the Discord poll Beny Bot posted. AnswerIDs follow the options' position order.
type RecordPollMessageInput struct {
	GuildID   string
	ChannelID string
	MessageID string
	Expiry    time.Time
	AnswerIDs []int
}

func (service *Service) ListPolls(requestContext context.Context, viewer Viewer) (PollList, error) {
	if err := requireActive(viewer); err != nil {
		return PollList{}, err
	}
	now := service.now()
	openRows, err := service.queries.ListOpenPolls(requestContext, store.ListOpenPollsParams{CampaignID: viewer.campaignID, Now: now})
	if err != nil {
		return PollList{}, err
	}
	closedRows, err := service.queries.ListClosedPolls(requestContext, store.ListClosedPollsParams{CampaignID: viewer.campaignID, Now: now, RowLimit: closedPollLimit})
	if err != nil {
		return PollList{}, err
	}
	// A poll closed between the two reads is in both; it is closed.
	openRows = slices.DeleteFunc(openRows, func(open store.Poll) bool {
		return slices.ContainsFunc(closedRows, func(closed store.Poll) bool { return closed.ID == open.ID })
	})
	polls, err := loadPolls(requestContext, service.queries, viewer, append(openRows, closedRows...), now)
	if err != nil {
		return PollList{}, err
	}
	voting, err := newPollVoting(requestContext, service.queries, viewer.campaignID)
	if err != nil {
		return PollList{}, err
	}
	return PollList{Open: polls[:len(openRows)], Closed: polls[len(openRows):], NewPollVoting: voting}, nil
}

func (service *Service) CreatePoll(requestContext context.Context, viewer Viewer, input CreatePollInput) (Poll, error) {
	if err := requireDm(viewer); err != nil {
		return Poll{}, err
	}
	question, fieldError := cleanText("question", input.Question, game.MaxPollQuestionLength)
	if fieldError != nil {
		return Poll{}, Invalid(fieldError.Message, *fieldError)
	}
	if question == "" {
		return Poll{}, Invalid("Write the question.", FieldError{Field: "question", Message: "Write the question."})
	}
	labels := make([]string, 0, len(input.Options))
	for _, option := range input.Options {
		label, fieldError := cleanText("options", option, game.MaxPollOptionLength)
		if fieldError != nil {
			return Poll{}, Invalid(fieldError.Message, *fieldError)
		}
		if label == "" {
			return Poll{}, Invalid("Fill in every option.", FieldError{Field: "options", Message: "Fill in every option."})
		}
		labels = append(labels, label)
	}
	if err := game.CheckPollOptions(labels); err != nil {
		return Poll{}, gameError(err, nil)
	}
	if err := game.CheckPollDuration(input.DurationHours); err != nil {
		return Poll{}, gameError(err, nil)
	}
	now := service.now()
	if err := service.closeOverduePolls(requestContext, viewer); err != nil {
		return Poll{}, err
	}
	var created Poll
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		// Locked first, so concurrent creates can't each pass the count and go over the limit.
		if _, err := queries.GetColonyForUpdate(requestContext, viewer.campaignID); err != nil {
			return campaignGone(err)
		}
		openPolls, err := queries.CountOpenPolls(requestContext, store.CountOpenPollsParams{CampaignID: viewer.campaignID, Now: now})
		if err != nil {
			return err
		}
		if game.OpenPollLimitReached(openPolls) {
			return ErrOpenPollLimit
		}
		voting, err := newPollVoting(requestContext, queries, viewer.campaignID)
		if err != nil {
			return err
		}
		row, err := queries.CreatePoll(requestContext, store.CreatePollParams{
			CampaignID:    viewer.campaignID,
			Question:      question,
			AllowMultiple: input.AllowMultiple,
			Voting:        voting,
			ClosesAt:      now.Add(time.Duration(input.DurationHours) * time.Hour).UTC(),
		})
		if err != nil {
			return campaignGone(err)
		}
		for position, label := range labels {
			if err := queries.CreatePollOption(requestContext, store.CreatePollOptionParams{
				CampaignID: viewer.campaignID,
				PollID:     row.ID,
				Position:   int32(position),
				Label:      label,
			}); err != nil {
				return err
			}
		}
		polls, err := loadPolls(requestContext, queries, viewer, []store.Poll{row}, now)
		if err != nil {
			return err
		}
		created = polls[0]
		return nil
	})
	if err != nil {
		return Poll{}, err
	}
	service.pollChanged(viewer.campaignID, created.ID)
	return created, nil
}

// VotePoll replaces the viewer's vote on a SITE poll with optionIDs; an empty list withdraws it.
func (service *Service) VotePoll(requestContext context.Context, viewer Viewer, pollID uuid.UUID, optionIDs []uuid.UUID) (Poll, error) {
	if err := requireVoter(viewer); err != nil {
		return Poll{}, err
	}
	if err := service.closeOverduePolls(requestContext, viewer); err != nil {
		return Poll{}, err
	}
	choice := uniqueIDs(optionIDs)
	var voted Poll
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		row, err := lockOpenPoll(requestContext, queries, viewer.campaignID, pollID, service.now)
		if err != nil {
			return err
		}
		if row.Voting == PollVotingDiscord {
			return ErrVoteInDiscord
		}
		if err := game.CheckPollVote(row.AllowMultiple, len(choice)); err != nil {
			return gameError(err, nil)
		}
		found, err := queries.CountPollOptionsAmong(requestContext, store.CountPollOptionsAmongParams{CampaignID: viewer.campaignID, PollID: pollID, OptionIds: choice})
		if err != nil {
			return err
		}
		if found != int64(len(choice)) {
			return errForeignPollVote
		}
		if err := replaceVote(requestContext, queries, viewer, pollID, choice); err != nil {
			return err
		}
		polls, err := loadPolls(requestContext, queries, viewer, []store.Poll{row}, service.now())
		if err != nil {
			return err
		}
		voted = polls[0]
		return nil
	})
	if err != nil {
		return Poll{}, err
	}
	service.broker.Publish(viewer.campaignID, TopicPolls)
	return voted, nil
}

// ClosePoll closes a poll now; closing a closed poll is a no-op.
func (service *Service) ClosePoll(requestContext context.Context, viewer Viewer, pollID uuid.UUID) (Poll, error) {
	if err := requireDm(viewer); err != nil {
		return Poll{}, err
	}
	if err := service.closeOverduePolls(requestContext, viewer); err != nil {
		return Poll{}, err
	}
	if _, err := service.closePoll(requestContext, viewer.campaignID, pollID, false); err != nil {
		return Poll{}, err
	}
	row, err := service.queries.GetPoll(requestContext, store.GetPollParams{CampaignID: viewer.campaignID, ID: pollID})
	if err != nil {
		return Poll{}, pollGone(err)
	}
	polls, err := loadPolls(requestContext, service.queries, viewer, []store.Poll{row}, service.now())
	if err != nil {
		return Poll{}, err
	}
	return polls[0], nil
}

// DeletePoll removes a poll, its options and its votes; its POLL_CLOSED entry stays on the timeline.
func (service *Service) DeletePoll(requestContext context.Context, viewer Viewer, pollID uuid.UUID) error {
	if err := requireDm(viewer); err != nil {
		return err
	}
	if err := service.closeOverduePolls(requestContext, viewer); err != nil {
		return err
	}
	message, err := service.queries.DeletePoll(requestContext, store.DeletePollParams{CampaignID: viewer.campaignID, ID: pollID})
	if err != nil {
		return pollGone(err)
	}
	service.broker.Publish(viewer.campaignID, TopicPolls)
	service.pollNotifier.PollDeleted(viewer.campaignID, stringValue(message.DiscordChannelID), stringValue(message.DiscordMessageID))
	return nil
}

// PollDeadlines lists a campaign's open polls that close by themselves, for Beny Bot's planner. A
// campaign that is gone or suspended has none.
func (service *Service) PollDeadlines(requestContext context.Context, campaignID uuid.UUID) ([]PollDeadline, error) {
	deadlines := []PollDeadline{}
	active, err := service.campaignActive(requestContext, campaignID)
	if err != nil || !active {
		return deadlines, err
	}
	rows, err := service.queries.ListPollDeadlines(requestContext, campaignID)
	if err != nil {
		return deadlines, err
	}
	for _, row := range rows {
		deadlines = append(deadlines, PollDeadline{PollID: row.ID, ClosesAt: row.ClosesAt.UTC()})
	}
	return deadlines, nil
}

// PollDiscordMessage is the Discord poll a DISCORD poll was posted as.
type PollDiscordMessage struct {
	PollID    uuid.UUID
	ChannelID string
	MessageID string
}

// DiscordPollsToEnd lists a campaign's DISCORD polls the DM closed early whose Discord poll may still
// run, so Beny Bot can end them after a restart. A campaign that is gone or suspended has none.
func (service *Service) DiscordPollsToEnd(requestContext context.Context, campaignID uuid.UUID) ([]PollDiscordMessage, error) {
	messages := []PollDiscordMessage{}
	active, err := service.campaignActive(requestContext, campaignID)
	if err != nil || !active {
		return messages, err
	}
	rows, err := service.queries.ListDiscordPollsToEnd(requestContext, store.ListDiscordPollsToEndParams{CampaignID: campaignID, Now: service.now()})
	if err != nil {
		return messages, err
	}
	for _, row := range rows {
		messages = append(messages, PollDiscordMessage{PollID: row.ID, ChannelID: row.DiscordChannelID, MessageID: row.DiscordMessageID})
	}
	return messages, nil
}

// PollDeadlineClose is what the planner's deadline timer did: Closed when this call closed the poll,
// and the poll's current close time, which a record of Discord's expiry may have moved.
type PollDeadlineClose struct {
	Closed   bool
	ClosesAt time.Time
}

// ClosePollAtDeadline is Beny Bot's planner closing a poll whose closes_at has passed. It does nothing
// early, twice, for a missing poll or in a suspended campaign.
func (service *Service) ClosePollAtDeadline(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID) (PollDeadlineClose, error) {
	active, err := service.campaignActive(requestContext, campaignID)
	if err != nil || !active {
		return PollDeadlineClose{}, err
	}
	closed, err := service.closePoll(requestContext, campaignID, pollID, true)
	if errors.Is(err, ErrNoSuchPoll) {
		return PollDeadlineClose{}, nil
	}
	if err != nil {
		return PollDeadlineClose{}, err
	}
	row, err := service.queries.GetPoll(requestContext, store.GetPollParams{CampaignID: campaignID, ID: pollID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PollDeadlineClose{Closed: closed}, nil
	}
	if err != nil {
		return PollDeadlineClose{}, err
	}
	return PollDeadlineClose{Closed: closed, ClosesAt: row.ClosesAt}, nil
}

// PollForDiscord is a DISCORD poll's player projection and its Discord poll, for Beny Bot to post,
// sync or end. A SITE poll is ErrNoSuchPoll here, since it never goes to Discord.
func (service *Service) PollForDiscord(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID) (DiscordPoll, error) {
	active, err := service.campaignActive(requestContext, campaignID)
	if err != nil {
		return DiscordPoll{}, err
	}
	if !active {
		return DiscordPoll{}, ErrNoSuchCampaign
	}
	row, err := service.queries.GetPoll(requestContext, store.GetPollParams{CampaignID: campaignID, ID: pollID})
	if err != nil {
		return DiscordPoll{}, pollGone(err)
	}
	if row.Voting != PollVotingDiscord {
		return DiscordPoll{}, ErrNoSuchPoll
	}
	polls, err := loadPolls(requestContext, service.queries, Viewer{campaignID: campaignID}, []store.Poll{row}, service.now())
	if err != nil {
		return DiscordPoll{}, err
	}
	optionRows, err := service.queries.ListPollOptions(requestContext, store.ListPollOptionsParams{CampaignID: campaignID, PollIds: []uuid.UUID{pollID}})
	if err != nil {
		return DiscordPoll{}, err
	}
	answerIDs := make([]int, 0, len(optionRows))
	for _, option := range optionRows {
		answerID := 0
		if option.DiscordAnswerID != nil {
			answerID = int(*option.DiscordAnswerID)
		}
		answerIDs = append(answerIDs, answerID)
	}
	return DiscordPoll{
		Poll:      polls[0],
		GuildID:   stringValue(row.DiscordGuildID),
		ChannelID: stringValue(row.DiscordChannelID),
		MessageID: stringValue(row.DiscordMessageID),
		AnswerIDs: answerIDs,
	}, nil
}

// RecordPollMessage keeps the Discord poll Beny Bot posted, its answer IDs, and Discord's expiry as the
// poll's close time. Only the first post on an open poll, in the server the campaign is still linked to,
// is kept; a later one gets ErrPollAlreadyPosted, one after a close ErrPollClosed and one in a server no
// longer linked ErrPollServerChanged, so Beny Bot deletes its post each time. Recording the message the
// poll already holds succeeds and changes nothing: two processes reusing one nonce share one message.
func (service *Service) RecordPollMessage(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID, input RecordPollMessageInput) error {
	for _, discordID := range []string{input.GuildID, input.ChannelID, input.MessageID} {
		if !discordSnowflakePattern.MatchString(discordID) {
			return Invalid("That isn't a Discord message.")
		}
	}
	alreadyRecorded := false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		updated, err := queries.RecordPollMessage(requestContext, store.RecordPollMessageParams{
			CampaignID:       campaignID,
			ID:               pollID,
			DiscordGuildID:   &input.GuildID,
			DiscordChannelID: &input.ChannelID,
			DiscordMessageID: &input.MessageID,
			ClosesAt:         input.Expiry.UTC(),
		})
		if err != nil {
			return err
		}
		if updated == 0 {
			row, err := queries.GetPoll(requestContext, store.GetPollParams{CampaignID: campaignID, ID: pollID})
			if err != nil {
				return pollGone(err)
			}
			if row.Voting != PollVotingDiscord {
				return ErrNoSuchPoll
			}
			if row.DiscordMessageID != nil && *row.DiscordMessageID == input.MessageID &&
				row.DiscordChannelID != nil && *row.DiscordChannelID == input.ChannelID &&
				row.DiscordGuildID != nil && *row.DiscordGuildID == input.GuildID {
				alreadyRecorded = true
				return nil
			}
			if row.ClosedAt != nil {
				return ErrPollClosed
			}
			if row.DiscordMessageID != nil {
				return ErrPollAlreadyPosted
			}
			return ErrPollServerChanged
		}
		answerIDs := make([]int32, 0, len(input.AnswerIDs))
		for _, answerID := range input.AnswerIDs {
			answerIDs = append(answerIDs, int32(answerID))
		}
		return queries.SetPollOptionAnswerIDs(requestContext, store.SetPollOptionAnswerIDsParams{CampaignID: campaignID, PollID: pollID, AnswerIds: answerIDs})
	})
	if err != nil || alreadyRecorded {
		return err
	}
	service.broker.Publish(campaignID, TopicPolls)
	return nil
}

// SyncDiscordPollVotes replaces a posted DISCORD poll's votes with votes, Discord's complete voter list
// for each answer ID. Players become named votes; anyone else (the DM included) is only counted, and
// their Discord IDs are never stored. A closed poll keeps the counts its close recorded. A single-answer
// poll's snapshot with one voter on two answers is ErrPollVotesInconsistent and writes nothing.
func (service *Service) SyncDiscordPollVotes(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID, votes map[int][]string) error {
	active, err := service.campaignActive(requestContext, campaignID)
	if err != nil {
		return err
	}
	if !active {
		return ErrNoSuchCampaign
	}
	changed := false
	err = service.inTransaction(requestContext, func(queries *store.Queries) error {
		row, err := queries.GetPollForUpdate(requestContext, store.GetPollForUpdateParams{CampaignID: campaignID, ID: pollID})
		if err != nil {
			return pollGone(err)
		}
		if row.Voting != PollVotingDiscord {
			return ErrNoSuchPoll
		}
		if row.ClosedAt != nil {
			return nil
		}
		optionRows, err := queries.ListPollOptions(requestContext, store.ListPollOptionsParams{CampaignID: campaignID, PollIds: []uuid.UUID{pollID}})
		if err != nil {
			return err
		}
		optionsByAnswer := map[int]store.PollOption{}
		discordUserIDs := []string{}
		for _, option := range optionRows {
			if option.DiscordAnswerID == nil {
				continue
			}
			optionsByAnswer[int(*option.DiscordAnswerID)] = option
			discordUserIDs = append(discordUserIDs, votes[int(*option.DiscordAnswerID)]...)
		}
		if !row.AllowMultiple && votedTwice(optionsByAnswer, votes) {
			return ErrPollVotesInconsistent
		}
		players, err := queries.ListPollVotingPlayers(requestContext, store.ListPollVotingPlayersParams{CampaignID: campaignID, DiscordUserIds: discordUserIDs})
		if err != nil {
			return err
		}
		memberByDiscordUser := make(map[string]uuid.UUID, len(players))
		for _, player := range players {
			memberByDiscordUser[player.DiscordUserID] = player.ID
		}
		voters := []uuid.UUID{}
		optionIDs := []uuid.UUID{}
		otherVotes := make([]int32, len(optionRows))
		otherVoters := map[string]struct{}{}
		for answerID, option := range optionsByAnswer {
			seen := map[string]struct{}{}
			for _, discordUserID := range votes[answerID] {
				if _, duplicate := seen[discordUserID]; duplicate {
					continue
				}
				seen[discordUserID] = struct{}{}
				if memberID, isPlayer := memberByDiscordUser[discordUserID]; isPlayer {
					voters = append(voters, memberID)
					optionIDs = append(optionIDs, option.ID)
					continue
				}
				otherVotes[option.Position]++
				otherVoters[discordUserID] = struct{}{}
			}
		}
		removed, err := queries.DeletePollVotesOutside(requestContext, store.DeletePollVotesOutsideParams{CampaignID: campaignID, PollID: pollID, Voters: voters, OptionIds: optionIDs})
		if err != nil {
			return err
		}
		added, err := queries.InsertPollVotes(requestContext, store.InsertPollVotesParams{CampaignID: campaignID, PollID: pollID, Voters: voters, OptionIds: optionIDs})
		if err != nil {
			return err
		}
		recountedOptions, err := queries.SetPollOptionOtherVotes(requestContext, store.SetPollOptionOtherVotesParams{CampaignID: campaignID, PollID: pollID, Votes: otherVotes})
		if err != nil {
			return err
		}
		recountedPoll, err := queries.SetPollOtherVoters(requestContext, store.SetPollOtherVotersParams{CampaignID: campaignID, ID: pollID, DiscordOtherVoters: int32(len(otherVoters))})
		if err != nil {
			return err
		}
		changed = removed+added+recountedOptions+recountedPoll > 0
		return nil
	})
	if err != nil {
		return err
	}
	if changed {
		service.broker.Publish(campaignID, TopicPolls)
	}
	return nil
}

// votedTwice reports whether one Discord user is among the voters of two of the poll's answers.
func votedTwice(optionsByAnswer map[int]store.PollOption, votes map[int][]string) bool {
	answerByVoter := map[string]int{}
	for answerID := range optionsByAnswer {
		for _, discordUserID := range votes[answerID] {
			if earlier, voted := answerByVoter[discordUserID]; voted && earlier != answerID {
				return true
			}
			answerByVoter[discordUserID] = answerID
		}
	}
	return false
}

// requireVoter admits only an ACTIVE player; the DM is refused even in view as player.
func requireVoter(viewer Viewer) error {
	if err := requireCampaign(viewer); err != nil {
		return err
	}
	if viewer.IsDM() {
		return ErrDmDoesNotVote
	}
	return requireActive(viewer)
}

func (service *Service) campaignActive(requestContext context.Context, campaignID uuid.UUID) (bool, error) {
	campaign, err := service.queries.GetCampaign(requestContext, campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return campaign.Status == CampaignStatusActive, nil
}

func (service *Service) pollChanged(campaignID uuid.UUID, pollID uuid.UUID) {
	service.broker.Publish(campaignID, TopicPolls)
	service.pollNotifier.PollChanged(campaignID, pollID)
}

// newPollVoting is where players vote on a poll created now: in Discord when the campaign's server is
// linked and has an announcements channel, the channel Beny Bot posts polls in, else on the site.
func newPollVoting(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID) (PollVoting, error) {
	link, err := optionalGuildLink(queries.GetGuildLink(requestContext, campaignID))
	if err != nil || link == nil {
		return PollVotingSite, err
	}
	settings, err := queries.GetSettings(requestContext, campaignID)
	if err != nil {
		return PollVotingSite, campaignGone(err)
	}
	if stringValue(settings.DiscordAnnounceChannelID) == "" {
		return PollVotingSite, nil
	}
	return PollVotingDiscord, nil
}

// lockOpenPoll locks a poll for a vote; a poll past its deadline is closed even before the close is
// recorded. The clock is read once the lock is held, so a vote that waited on it can't pass a deadline
// that went by meanwhile.
func lockOpenPoll(requestContext context.Context, queries *store.Queries, campaignID uuid.UUID, pollID uuid.UUID, now func() time.Time) (store.Poll, error) {
	row, err := queries.GetPollForUpdate(requestContext, store.GetPollForUpdateParams{CampaignID: campaignID, ID: pollID})
	if err != nil {
		return store.Poll{}, pollGone(err)
	}
	if game.PollClosed(row.ClosedAt, row.ClosesAt, now()) {
		return store.Poll{}, ErrPollClosed
	}
	return row, nil
}

func replaceVote(requestContext context.Context, queries *store.Queries, viewer Viewer, pollID uuid.UUID, optionIDs []uuid.UUID) error {
	keep := optionIDs
	if keep == nil {
		keep = []uuid.UUID{}
	}
	if err := queries.DeleteMemberPollVotes(requestContext, store.DeleteMemberPollVotesParams{CampaignID: viewer.campaignID, PollID: pollID, Voter: viewer.memberID, KeepOptionIds: keep}); err != nil {
		return err
	}
	if len(optionIDs) == 0 {
		return nil
	}
	return queries.InsertMemberPollVotes(requestContext, store.InsertMemberPollVotesParams{CampaignID: viewer.campaignID, PollID: pollID, Voter: viewer.memberID, OptionIds: optionIDs})
}

// closeOverduePolls records the close of every poll in the campaign whose deadline has passed (a posted
// Discord poll's grace included), so a campaign without Beny Bot still gets its POLL_CLOSED entries. Each close commits on its own, before
// the write that triggered it, so a refused write doesn't undo it.
func (service *Service) closeOverduePolls(requestContext context.Context, viewer Viewer) error {
	now := service.now()
	rows, err := service.queries.ListOverduePolls(requestContext, store.ListOverduePollsParams{CampaignID: viewer.campaignID, Now: now})
	if err != nil {
		return err
	}
	for _, row := range rows {
		if !game.PollDueForLazyClose(nil, row.ClosesAt, row.Posted, now) {
			continue
		}
		if _, err := service.closePoll(requestContext, viewer.campaignID, row.ID, true); err != nil && !errors.Is(err, ErrNoSuchPoll) {
			return err
		}
	}
	return nil
}

// closePoll records a poll's close under the colony lock with its PUBLIC POLL_CLOSED entry, leaving the
// colony's version and undo slot alone. With onlyOverdue it closes only a poll past its deadline. It
// reports whether this call closed the poll.
func (service *Service) closePoll(requestContext context.Context, campaignID uuid.UUID, pollID uuid.UUID, onlyOverdue bool) (bool, error) {
	closed := false
	err := service.inTransaction(requestContext, func(queries *store.Queries) error {
		colony, err := queries.GetColonyForUpdate(requestContext, campaignID)
		if err != nil {
			return campaignGone(err)
		}
		row, err := queries.GetPollForUpdate(requestContext, store.GetPollForUpdateParams{CampaignID: campaignID, ID: pollID})
		if err != nil {
			return pollGone(err)
		}
		now := service.now()
		overdue := game.PollOverdue(row.ClosedAt, row.ClosesAt, now)
		if row.ClosedAt != nil || (onlyOverdue && !overdue) {
			return nil
		}
		closedAt := now
		if overdue {
			closedAt = row.ClosesAt.UTC()
		}
		if _, err := queries.ClosePoll(requestContext, store.ClosePollParams{CampaignID: campaignID, ID: pollID, ClosedAt: closedAt}); err != nil {
			return err
		}
		counts, err := queries.CountPollVotesByOption(requestContext, store.CountPollVotesByOptionParams{CampaignID: campaignID, PollID: pollID})
		if err != nil {
			return err
		}
		tallies := make([]game.PollTally, 0, len(counts))
		for _, count := range counts {
			tallies = append(tallies, game.PollTally{Label: count.Label, Count: count.Votes})
		}
		closed = true
		return writeLooseEntry(requestContext, queries, Viewer{campaignID: campaignID}, plannedEntry{
			day:        int64(colony.Day),
			kind:       TimelineKindPollClosed,
			title:      game.PollClosedTitle(row.Question, tallies),
			visibility: TimelineVisibilityPublic,
		})
	})
	if err != nil {
		return false, err
	}
	if closed {
		service.broker.Publish(campaignID, TopicPolls, TopicTimeline)
		service.pollNotifier.PollChanged(campaignID, pollID)
	}
	return closed, nil
}

// loadPolls builds the DTOs for rows, in their order, with each option's voters and the viewer's vote.
func loadPolls(requestContext context.Context, queries *store.Queries, viewer Viewer, rows []store.Poll, now time.Time) ([]Poll, error) {
	polls := make([]Poll, 0, len(rows))
	if len(rows) == 0 {
		return polls, nil
	}
	pollIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		pollIDs = append(pollIDs, row.ID)
	}
	optionRows, err := queries.ListPollOptions(requestContext, store.ListPollOptionsParams{CampaignID: viewer.campaignID, PollIds: pollIDs})
	if err != nil {
		return nil, err
	}
	voteRows, err := queries.ListPollVotes(requestContext, store.ListPollVotesParams{CampaignID: viewer.campaignID, PollIds: pollIDs})
	if err != nil {
		return nil, err
	}
	options := map[uuid.UUID][]PollOption{}
	for _, row := range optionRows {
		options[row.PollID] = append(options[row.PollID], PollOption{
			ID:         row.ID,
			Label:      row.Label,
			Position:   int(row.Position),
			Count:      int64(row.DiscordOtherVotes),
			Voters:     []PollVoter{},
			OtherVotes: int64(row.DiscordOtherVotes),
		})
	}
	voters := map[uuid.UUID]map[uuid.UUID]bool{}
	mine := map[uuid.UUID][]uuid.UUID{}
	for _, vote := range voteRows {
		if voters[vote.PollID] == nil {
			voters[vote.PollID] = map[uuid.UUID]bool{}
		}
		voters[vote.PollID][vote.Voter] = true
		if viewer.memberID != uuid.Nil && vote.Voter == viewer.memberID {
			mine[vote.PollID] = append(mine[vote.PollID], vote.OptionID)
		}
		pollOptions := options[vote.PollID]
		for index := range pollOptions {
			if pollOptions[index].ID == vote.OptionID {
				pollOptions[index].Count++
				pollOptions[index].Voters = append(pollOptions[index].Voters, PollVoter{Name: memberName(vote.VoterCharacterName, vote.VoterDisplayName)})
			}
		}
	}
	for _, row := range rows {
		poll := Poll{
			ID:            row.ID,
			Question:      row.Question,
			AllowMultiple: row.AllowMultiple,
			Voting:        row.Voting,
			ClosesAt:      row.ClosesAt.UTC(),
			ClosedAt:      utcPointer(row.ClosedAt),
			Closed:        game.PollClosed(row.ClosedAt, row.ClosesAt, now),
			CreatedAt:     row.CreatedAt.UTC(),
			Options:       options[row.ID],
			VoterCount:    int64(len(voters[row.ID])) + int64(row.DiscordOtherVoters),
			MyOptionIDs:   mine[row.ID],
			DiscordURL:    discordPollURL(row),
			CloseRecorded: row.ClosedAt != nil,
		}
		if poll.ClosedAt == nil && poll.Closed {
			poll.ClosedAt = &poll.ClosesAt
		}
		if poll.Options == nil {
			poll.Options = []PollOption{}
		}
		if poll.MyOptionIDs == nil {
			poll.MyOptionIDs = []uuid.UUID{}
		}
		polls = append(polls, poll)
	}
	return polls, nil
}

func discordPollURL(row store.Poll) *string {
	if row.DiscordGuildID == nil || row.DiscordChannelID == nil || row.DiscordMessageID == nil {
		return nil
	}
	url := "https://discord.com/channels/" + *row.DiscordGuildID + "/" + *row.DiscordChannelID + "/" + *row.DiscordMessageID
	return &url
}

func pollGone(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNoSuchPoll
	}
	return err
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	unique := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if !slices.Contains(unique, id) {
			unique = append(unique, id)
		}
	}
	return unique
}

func stringValue(pointer *string) string {
	if pointer == nil {
		return ""
	}
	return *pointer
}
