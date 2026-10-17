package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerQuests(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listQuests",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/quests",
		Summary:     "Every quest in the viewer's projection; players get only visible ones",
		Tags:        []string{"quests"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.QuestList], error) {
		quests, err := handlers.core.ListQuests(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.QuestList]{Body: quests}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createQuest",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/quests",
		Summary:       "Add a quest, visible or hidden, posting it to the timeline when it starts visible",
		Tags:          []string{"quests"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.NewQuestInput]) (*bodyOutput[core.Quest], error) {
		quest, err := handlers.core.CreateQuest(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Quest]{Body: quest}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getQuest",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/quests/{questId}",
		Summary:     "One quest; a hidden one is not found for players",
		Tags:        []string{"quests"},
	}, func(requestContext context.Context, input *questPath) (*bodyOutput[core.Quest], error) {
		quest, err := handlers.core.GetQuest(requestContext, viewerFrom(requestContext), input.QuestID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Quest]{Body: quest}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateQuest",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/quests/{questId}",
		Summary:     "Replace a quest's record (status changes through complete, fail and reopen, visibility through setQuestRevealed)",
		Tags:        []string{"quests"},
	}, func(requestContext context.Context, input *questBodyInput[core.QuestInput]) (*bodyOutput[core.Quest], error) {
		quest, err := handlers.core.UpdateQuest(requestContext, viewerFrom(requestContext), input.QuestID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Quest]{Body: quest}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setQuestRevealed",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/quests/{questId}/revealed",
		Summary:     "Show a quest to the players, posting it to the timeline unless asked not to, or hide it again",
		Tags:        []string{"quests"},
	}, func(requestContext context.Context, input *questBodyInput[core.QuestRevealInput]) (*bodyOutput[core.Quest], error) {
		quest, err := handlers.core.SetQuestRevealed(requestContext, viewerFrom(requestContext), input.QuestID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Quest]{Body: quest}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deleteQuest",
		Method:        http.MethodDelete,
		Path:          "/api/campaigns/{campaignId}/quests/{questId}",
		Summary:       "Delete a quest; its timeline entries keep their text",
		Tags:          []string{"quests"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *questPath) (*struct{}, error) {
		if err := handlers.core.DeleteQuest(requestContext, viewerFrom(requestContext), input.QuestID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	registerQuestPreview(handlers, api, "previewCompleteQuest", "/api/campaigns/{campaignId}/quests/{questId}/complete-preview",
		"What completing a quest would apply, without doing it", (*core.Service).PreviewCompleteQuest)
	registerQuestPreview(handlers, api, "previewFailQuest", "/api/campaigns/{campaignId}/quests/{questId}/fail-preview",
		"What failing a quest would cost, without doing it", (*core.Service).PreviewFailQuest)
	registerQuestResolution(handlers, api, "completeQuest", "/api/campaigns/{campaignId}/quests/{questId}/complete",
		"Complete an active quest and pay its reward", (*core.Service).CompleteQuest)
	registerQuestResolution(handlers, api, "failQuest", "/api/campaigns/{campaignId}/quests/{questId}/fail",
		"Fail an active quest and apply its failure cost", (*core.Service).FailQuest)
	registerQuestResolution(handlers, api, "reopenQuest", "/api/campaigns/{campaignId}/quests/{questId}/reopen",
		"Reopen a failed quest, giving nothing back", (*core.Service).ReopenQuest)
}

func registerQuestPreview(handlers *handlers, api huma.API, operationID, path, summary string,
	run func(*core.Service, context.Context, core.Viewer, uuid.UUID) (core.QuestResolutionPreview, error),
) {
	Register(api, AccessDm, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodGet,
		Path:        path,
		Summary:     summary,
		Tags:        []string{"quests"},
	}, func(requestContext context.Context, input *questPath) (*bodyOutput[core.QuestResolutionPreview], error) {
		preview, err := run(handlers.core, requestContext, viewerFrom(requestContext), input.QuestID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.QuestResolutionPreview]{Body: preview}, nil
	})
}

// registerQuestResolution registers one bodiless POST colony mutation on a quest. The quest's status
// guards it, so it takes no expectedVersion: a second click answers 409 "Already resolved".
func registerQuestResolution(handlers *handlers, api huma.API, operationID, path, summary string,
	run func(*core.Service, context.Context, core.Viewer, uuid.UUID) (core.ColonyChange, error),
) {
	Register(api, AccessDm, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodPost,
		Path:        path,
		Summary:     summary,
		Tags:        []string{"quests"},
	}, func(requestContext context.Context, input *questPath) (*bodyOutput[core.ColonyChange], error) {
		change, err := run(handlers.core, requestContext, viewerFrom(requestContext), input.QuestID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})
}
