package core

import "github.com/BBruington/colony-planner/internal/store"

// PollVoting is the Postgres poll_voting enum: where a poll's players vote, fixed when it is created.
type PollVoting = store.PollVoting

const (
	PollVotingSite    = store.PollVotingSITE
	PollVotingDiscord = store.PollVotingDISCORD
)

// AllPollVotingValues lists every voting place, for the API's enum schema.
func AllPollVotingValues() []PollVoting {
	return store.AllPollVotingValues()
}
