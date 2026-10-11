package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

func (handlers *handlers) registerMembers(api huma.API) {
	Register(api, AccessDm, huma.Operation{
		OperationID: "listMembers",
		Method:      http.MethodGet,
		Path:        "/api/members",
		Summary:     "Pending, active and removed members",
		Tags:        []string{"members"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.MemberGroups], error) {
		groups, err := handlers.core.ListMembers(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.MemberGroups]{Body: groups}, nil
	})

	memberActions := []struct {
		operationID string
		action      string
		summary     string
		run         func(*core.Service, context.Context, core.Viewer, uuid.UUID) (core.Member, error)
	}{
		{"approveMember", "approve", "Let a pending member in", (*core.Service).ApproveMember},
		{"denyMember", "deny", "Turn a pending member away", (*core.Service).DenyMember},
		{"removeMember", "remove", "Remove an active member and sign them out everywhere", (*core.Service).RemoveMember},
		{"readmitMember", "readmit", "Let a removed or denied member back in", (*core.Service).ReadmitMember},
	}
	for _, memberAction := range memberActions {
		Register(api, AccessDm, huma.Operation{
			OperationID: memberAction.operationID,
			Method:      http.MethodPost,
			Path:        "/api/members/{memberId}/" + memberAction.action,
			Summary:     memberAction.summary,
			Tags:        []string{"members"},
		}, func(requestContext context.Context, input *memberPath) (*bodyOutput[core.Member], error) {
			member, err := memberAction.run(handlers.core, requestContext, viewerFrom(requestContext), input.MemberID)
			if err != nil {
				return nil, handlers.fail(requestContext, err)
			}
			return &bodyOutput[core.Member]{Body: member}, nil
		})
	}

	Register(api, AccessDm, huma.Operation{
		OperationID: "setMemberCharacterName",
		Method:      http.MethodPut,
		Path:        "/api/members/{memberId}/character-name",
		Summary:     "Set a member's character name",
		Tags:        []string{"members"},
	}, func(requestContext context.Context, input *memberBodyInput[core.CharacterNameInput]) (*bodyOutput[core.Member], error) {
		member, err := handlers.core.SetCharacterName(requestContext, viewerFrom(requestContext), input.MemberID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Member]{Body: member}, nil
	})
}
