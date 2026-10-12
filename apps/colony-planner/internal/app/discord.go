package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

// registerDiscord holds the DM's Discord server card: a one-time /link code, unlinking, and the linked
// server's text channels for the channel picker.
func (handlers *handlers) registerDiscord(api huma.API) {
	Register(api, AccessDm, huma.Operation{
		OperationID: "createLinkCode",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/discord/link-code",
		Summary:     "A 15-minute, single-use code for /link in the campaign's Discord server; replaces any older one",
		Tags:        []string{"discord"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.LinkCode], error) {
		code, err := handlers.core.CreateLinkCode(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.LinkCode]{Body: code}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "unlinkDiscordServer",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/discord/unlink",
		Summary:       "Unlink the campaign's Discord server and clear its channels",
		Tags:          []string{"discord"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, _ *campaignPath) (*struct{}, error) {
		if err := handlers.core.UnlinkGuild(requestContext, viewerFrom(requestContext)); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "listDiscordChannels",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/discord/channels",
		Summary:     "The linked server's text channels, for the channel picker; empty when none is linked or Beny Bot is off",
		Tags:        []string{"discord"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[[]core.DiscordChannel], error) {
		channels, err := handlers.core.DiscordChannelOptions(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[[]core.DiscordChannel]{Body: channels}, nil
	})
}
