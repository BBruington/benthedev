package seed

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// samplePoll is one poll and its votes, cast in order.
type samplePoll struct {
	input core.CreatePollInput
	votes []sampleVote
	close bool
}

// sampleVote is one member's picks, by option index.
type sampleVote struct {
	discordUserID string
	picks         []int
}

// seedSamplePolls plays the polls through core in order: each is opened, voted on and maybe closed.
func seedSamplePolls(requestContext context.Context, service *core.Service, dm core.Viewer, enter enterAs, polls []samplePoll) error {
	for _, sample := range polls {
		poll, err := service.CreatePoll(requestContext, dm, sample.input)
		if err != nil {
			return fmt.Errorf("poll %s: %w", sample.input.Question, err)
		}
		for _, vote := range sample.votes {
			voter, err := enter(vote.discordUserID)
			if err != nil {
				return err
			}
			optionIDs := make([]uuid.UUID, 0, len(vote.picks))
			for _, pick := range vote.picks {
				optionIDs = append(optionIDs, poll.Options[pick].ID)
			}
			if _, err := service.VotePoll(requestContext, voter, poll.ID, optionIDs); err != nil {
				return fmt.Errorf("%s's vote on %s: %w", vote.discordUserID, sample.input.Question, err)
			}
		}
		if sample.close {
			if _, err := service.ClosePoll(requestContext, dm, poll.ID); err != nil {
				return fmt.Errorf("close %s: %w", sample.input.Question, err)
			}
		}
	}
	return nil
}

// Sample Colony's polls: the boat's name was settled (so its POLL_CLOSED entry is on the timeline),
// the festivals vote closes in three days, and the watchtower vote, opened last, waits on Ana (Thorin),
// the player dev sign-in picks, so the board shows Thorin its vote card.
var sampleColonyPolls = []samplePoll{
	{
		input: core.CreatePollInput{
			Question:      "What should we name the new fishing boat?",
			Options:       []string{"Second Dawn", "Saltwind", "Sea Biscuit"},
			DurationHours: 24,
		},
		votes: []sampleVote{{"seed:ana", []int{1}}, {"seed:bram", []int{1}}, {"seed:cass", []int{2}}, {"seed:dov", []int{0}}},
		close: true,
	},
	{
		input: core.CreatePollInput{
			Question:      "Which festivals should we hold this season?",
			Options:       []string{"Harvest Moon feast", "Founders' Day games", "Lantern night on the docks"},
			AllowMultiple: true,
			DurationHours: 72,
		},
		votes: []sampleVote{{"seed:ana", []int{0, 2}}, {"seed:hollis", []int{1}}, {"seed:bram", []int{0}}},
	},
	{
		input: core.CreatePollInput{
			Question:      "Where should we raise the watchtower?",
			Options:       []string{"The north ridge", "Beside the harbor", "On the old shrine hill"},
			DurationHours: 168,
		},
		votes: []sampleVote{{"seed:bram", []int{1}}, {"seed:cass", []int{0}}, {"seed:dov", []int{1}}},
	},
}

// Second Sample's one open poll, which Tamsin has answered and Brisa hasn't.
var secondSamplePolls = []samplePoll{
	{
		input: core.CreatePollInput{
			Question:      "Which tunnel should the miners clear first?",
			Options:       []string{"The east drift", "The flooded shaft"},
			DurationHours: 168,
		},
		votes: []sampleVote{{"seed:iko", []int{0}}},
	},
}
