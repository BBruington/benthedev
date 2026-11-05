package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerPolls(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listPolls",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/polls",
		Summary:     "Open polls, newest first, and the 50 most recently closed",
		Tags:        []string{"polls"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.PollList], error) {
		list, err := handlers.core.ListPolls(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.PollList]{Body: list}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createPoll",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/polls",
		Summary:       "Ask the party a question; with an announcements channel on the linked server, players vote in a Discord poll Beny Bot posts",
		Tags:          []string{"polls"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.CreatePollInput]) (*bodyOutput[core.Poll], error) {
		poll, err := handlers.core.CreatePoll(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Poll]{Body: poll}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "votePoll",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/polls/{pollId}/vote",
		Summary:     "Replace the caller's vote on a site poll (active players only); an empty list withdraws it",
		Tags:        []string{"polls"},
	}, func(requestContext context.Context, input *pollBodyInput[PollVoteInput]) (*bodyOutput[core.Poll], error) {
		poll, err := handlers.core.VotePoll(requestContext, viewerFrom(requestContext), input.PollID, input.Body.OptionIDs)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Poll]{Body: poll}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "closePoll",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/polls/{pollId}/close",
		Summary:     "Close a poll now and post its final counts to the timeline; a closed poll stays closed",
		Tags:        []string{"polls"},
	}, func(requestContext context.Context, input *pollPath) (*bodyOutput[core.Poll], error) {
		poll, err := handlers.core.ClosePoll(requestContext, viewerFrom(requestContext), input.PollID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Poll]{Body: poll}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deletePoll",
		Method:        http.MethodDelete,
		Path:          "/api/campaigns/{campaignId}/polls/{pollId}",
		Summary:       "Delete a poll and its votes; its closing timeline entry stays",
		Tags:          []string{"polls"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *pollPath) (*struct{}, error) {
		if err := handlers.core.DeletePoll(requestContext, viewerFrom(requestContext), input.PollID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})
}
