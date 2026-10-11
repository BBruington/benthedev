package app

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

type timelineRangeQuery struct {
	FromDay int64 `query:"fromDay" required:"true" doc:"First day shown, inclusive"`
	ToDay   int64 `query:"toDay" required:"true" doc:"Last day shown, inclusive; at most 365 days after fromDay"`
}

func (handlers *handlers) registerTimeline(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listTimeline",
		Method:      http.MethodGet,
		Path:        "/api/timeline",
		Summary:     "Timeline entries for a range of days, in the viewer's projection",
		Tags:        []string{"timeline"},
	}, func(requestContext context.Context, input *timelineRangeQuery) (*bodyOutput[core.TimelinePage], error) {
		page, err := handlers.core.ListTimeline(requestContext, viewerFrom(requestContext), core.TimelineRangeInput{
			FromDay: input.FromDay,
			ToDay:   input.ToDay,
		})
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.TimelinePage]{Body: page}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "listRecentTimeline",
		Method:      http.MethodGet,
		Path:        "/api/timeline/recent",
		Summary:     "The newest timeline entries in write order, for the board and live toasts",
		Tags:        []string{"timeline"},
	}, func(requestContext context.Context, _ *struct{}) (*bodyOutput[core.RecentTimeline], error) {
		recent, err := handlers.core.RecentTimeline(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.RecentTimeline]{Body: recent}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createTimelineEntry",
		Method:        http.MethodPost,
		Path:          "/api/timeline",
		Summary:       "Add a custom timeline entry on any day",
		Tags:          []string{"timeline"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *bodyInput[core.TimelineEntryInput]) (*bodyOutput[core.TimelineEntry], error) {
		entry, err := handlers.core.CreateCustomEntry(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.TimelineEntry]{Body: entry}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateTimelineEntry",
		Method:      http.MethodPut,
		Path:        "/api/timeline/{timelineEntryId}",
		Summary:     "Replace a timeline entry's text and visibility (and a custom entry's day)",
		Tags:        []string{"timeline"},
	}, func(requestContext context.Context, input *timelineEntryBodyInput[core.TimelineEntryInput]) (*bodyOutput[core.TimelineEntry], error) {
		entry, err := handlers.core.UpdateTimelineEntry(requestContext, viewerFrom(requestContext), input.TimelineEntryID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.TimelineEntry]{Body: entry}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deleteTimelineEntry",
		Method:        http.MethodDelete,
		Path:          "/api/timeline/{timelineEntryId}",
		Summary:       "Delete a custom timeline entry",
		Tags:          []string{"timeline"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *timelineEntryPath) (*struct{}, error) {
		if err := handlers.core.DeleteCustomEntry(requestContext, viewerFrom(requestContext), input.TimelineEntryID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})
}
