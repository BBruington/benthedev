package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/core"
)

// npcListQuery is the roster's search and filters; every empty value is off. huma takes no pointer
// query parameters, so an absent place is the nil UUID and the DM's Hidden filter is a string.
type npcListQuery struct {
	CampaignID    uuid.UUID        `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	Search        string           `query:"search" doc:"Players match display names and aliases; the DM also matches true and known names"`
	Status        core.NpcStatus   `query:"status"`
	Health        core.NpcHealth   `query:"health"`
	Relation      core.NpcRelation `query:"relation"`
	Label         string           `query:"label"`
	RoleKey       string           `query:"roleKey" doc:"Named colonists in this role"`
	PlaceID       uuid.UUID        `query:"placeId" format:"uuid" doc:"NPCs whose current place this is"`
	ColonistsOnly bool             `query:"colonistsOnly"`
	Revealed      string           `query:"revealed" enum:"true,false" doc:"The DM's Hidden filter; players only ever get revealed NPCs"`
}

func (query *npcListQuery) filter() core.NpcFilter {
	filter := core.NpcFilter{
		Search:        query.Search,
		Status:        query.Status,
		Health:        query.Health,
		Relation:      query.Relation,
		Label:         query.Label,
		RoleKey:       query.RoleKey,
		ColonistsOnly: query.ColonistsOnly,
	}
	if query.PlaceID != uuid.Nil {
		placeID := query.PlaceID
		filter.PlaceID = &placeID
	}
	if query.Revealed != "" {
		revealed := query.Revealed == "true"
		filter.Revealed = &revealed
	}
	return filter
}

func (handlers *handlers) registerNpcs(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listNpcs",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/npcs",
		Summary:     "The NPC roster, searched and filtered, in the viewer's projection",
		Tags:        []string{"npcs"},
	}, func(requestContext context.Context, input *npcListQuery) (*bodyOutput[core.NpcList], error) {
		npcs, err := handlers.core.ListNpcs(requestContext, viewerFrom(requestContext), input.filter())
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.NpcList]{Body: npcs}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createNpc",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/npcs",
		Summary:       "Add an NPC",
		Tags:          []string{"npcs"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.NpcInput]) (*bodyOutput[core.Npc], error) {
		npc, err := handlers.core.CreateNpc(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Npc]{Body: npc}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getNpc",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/npcs/{npcId}",
		Summary:     "One NPC; a hidden one is not found for players",
		Tags:        []string{"npcs"},
	}, func(requestContext context.Context, input *npcPath) (*bodyOutput[core.Npc], error) {
		npc, err := handlers.core.GetNpc(requestContext, viewerFrom(requestContext), input.NpcID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Npc]{Body: npc}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateNpc",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/npcs/{npcId}",
		Summary:     "Replace an NPC's record (the colony role changes through the colony operations)",
		Tags:        []string{"npcs"},
	}, func(requestContext context.Context, input *npcBodyInput[core.NpcInput]) (*bodyOutput[core.Npc], error) {
		npc, err := handlers.core.UpdateNpc(requestContext, viewerFrom(requestContext), input.NpcID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Npc]{Body: npc}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "setNpcRevealed",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/npcs/{npcId}/revealed",
		Summary:     "Reveal an NPC to the players, or hide them again",
		Tags:        []string{"npcs"},
	}, func(requestContext context.Context, input *npcBodyInput[RevealInput]) (*bodyOutput[core.Npc], error) {
		npc, err := handlers.core.SetNpcRevealed(requestContext, viewerFrom(requestContext), input.NpcID, input.Body.Revealed)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Npc]{Body: npc}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deleteNpc",
		Method:        http.MethodDelete,
		Path:          "/api/campaigns/{campaignId}/npcs/{npcId}",
		Summary:       "Delete an NPC who isn't a colonist",
		Tags:          []string{"npcs"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *npcPath) (*struct{}, error) {
		if err := handlers.core.DeleteNpc(requestContext, viewerFrom(requestContext), input.NpcID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	registerNpcColonyChange(handlers, api, "joinColony", "/api/campaigns/{campaignId}/npcs/{npcId}/join-colony",
		"Make an NPC a named colonist, as a new colonist or by naming one of a role's unnamed colonists", (*core.Service).JoinColony)
	registerNpcColonyChange(handlers, api, "changeColonyRole", "/api/campaigns/{campaignId}/npcs/{npcId}/change-role",
		"Move a named colonist to another role", (*core.Service).ChangeColonyRole)
	registerNpcColonyChange(handlers, api, "leaveColony", "/api/campaigns/{campaignId}/npcs/{npcId}/leave-colony",
		"Take a named colonist out of the colony, or back among the role's unnamed colonists", (*core.Service).LeaveColony)
}

// registerNpcColonyChange registers one POST colony mutation on a named colonist that answers with a
// ColonyChange.
func registerNpcColonyChange[Input any](handlers *handlers, api huma.API, operationID, path, summary string,
	run func(*core.Service, context.Context, core.Viewer, uuid.UUID, Input) (core.ColonyChange, error),
) {
	Register(api, AccessDm, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodPost,
		Path:        path,
		Summary:     summary,
		Tags:        []string{"people"},
	}, func(requestContext context.Context, input *npcBodyInput[Input]) (*bodyOutput[core.ColonyChange], error) {
		change, err := run(handlers.core, requestContext, viewerFrom(requestContext), input.NpcID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.ColonyChange]{Body: change}, nil
	})
}
