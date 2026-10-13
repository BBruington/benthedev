package app

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// NextSession is the board's next scheduled date.
type NextSession struct {
	Session *core.Session `json:"session" doc:"The next scheduled date that hasn't started; null when none is planned"`
}

// LatestRecap is the board's last recap snippet.
type LatestRecap struct {
	Session *core.Session `json:"session" doc:"The newest played session with a recap; null when none has one"`
}

// calendarFileParameter is the ICS feed's last path segment, `{token}.ics`: a ServeMux wildcard spans
// a whole segment, so the suffix is checked by the handler.
const calendarFileParameter = "calendarFile"

func (handlers *handlers) registerGameSessions(api huma.API) {
	Register(api, AccessMember, huma.Operation{
		OperationID: "listSessionSeries",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/session-series",
		Summary:     "Every session series and one-off session, in the table's timezone",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[[]core.SessionSeries], error) {
		series, err := handlers.core.ListSessionSeries(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[[]core.SessionSeries]{Body: series}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createSessionSeries",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/session-series",
		Summary:       "Add a weekly or every-other-week series, or a one-off session",
		Tags:          []string{"sessions"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, input *campaignBodyInput[core.SessionSeriesInput]) (*bodyOutput[core.SessionSeries], error) {
		series, err := handlers.core.CreateSessionSeries(requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.SessionSeries]{Body: series}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "updateSessionSeries",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/session-series/{seriesId}",
		Summary:     "Replace a series; cancelled dates stay cancelled because they are kept by local date",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, input *seriesBodyInput[core.SessionSeriesInput]) (*bodyOutput[core.SessionSeries], error) {
		series, err := handlers.core.UpdateSessionSeries(requestContext, viewerFrom(requestContext), input.SeriesID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.SessionSeries]{Body: series}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "deleteSessionSeries",
		Method:        http.MethodDelete,
		Path:          "/api/campaigns/{campaignId}/session-series/{seriesId}",
		Summary:       "Delete a series; its played sessions stay in the history",
		Tags:          []string{"sessions"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *seriesPath) (*struct{}, error) {
		if err := handlers.core.DeleteSessionSeries(requestContext, viewerFrom(requestContext), input.SeriesID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	registerSessionList(handlers, api, "listUpcomingSessions", "/api/campaigns/{campaignId}/sessions/upcoming",
		"Every series' dates from a game in progress through the next 12 weeks, oldest first, cancelled ones included",
		(*core.Service).ListUpcomingSessions)
	registerSessionList(handlers, api, "listPlayedSessions", "/api/campaigns/{campaignId}/sessions/played",
		"Played sessions, newest first", (*core.Service).ListPlayedSessions)

	Register(api, AccessMember, huma.Operation{
		OperationID: "getNextSession",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/sessions/next",
		Summary:     "The next scheduled date, for the board",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[NextSession], error) {
		session, err := handlers.core.NextSession(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[NextSession]{Body: NextSession{Session: session}}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getLatestRecap",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/sessions/latest-recap",
		Summary:     "The newest played session with a recap, for the board's snippet",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[LatestRecap], error) {
		session, err := handlers.core.LatestRecap(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[LatestRecap]{Body: LatestRecap{Session: session}}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "listSessionBands",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/sessions/bands",
		Summary:     "Played sessions whose colony days meet a range of timeline days, for the timeline's bands",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, input *timelineRangeQuery) (*bodyOutput[[]core.Session], error) {
		sessions, err := handlers.core.ListSessionBands(requestContext, viewerFrom(requestContext), core.TimelineRangeInput{
			FromDay: input.FromDay,
			ToDay:   input.ToDay,
		})
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[[]core.Session]{Body: sessions}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getSession",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/sessions/{sessionId}",
		Summary:     "One stored session date: its status, recap and colony days",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, input *sessionPath) (*bodyOutput[core.Session], error) {
		session, err := handlers.core.GetSession(requestContext, viewerFrom(requestContext), input.SessionID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Session]{Body: session}, nil
	})

	registerSessionDate(handlers, api, "cancelSessionDate", "/api/campaigns/{campaignId}/sessions/cancel",
		"Cancel one date of a series", (*core.Service).CancelSessionDate)
	registerSessionDate(handlers, api, "restoreSessionDate", "/api/campaigns/{campaignId}/sessions/restore",
		"Put a cancelled date back on the schedule", (*core.Service).RestoreSessionDate)
	registerSessionDate(handlers, api, "markSessionPlayed", "/api/campaigns/{campaignId}/sessions/mark-played",
		"Mark a date played, stamping the colony's current day as its first", (*core.Service).MarkSessionPlayed)

	Register(api, AccessDm, huma.Operation{
		OperationID: "endSession",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/sessions/{sessionId}/end",
		Summary:     "End a played session, stamping the colony's current day as its last",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, input *sessionPath) (*bodyOutput[core.Session], error) {
		session, err := handlers.core.EndSession(requestContext, viewerFrom(requestContext), input.SessionID)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Session]{Body: session}, nil
	})

	registerSessionEdit(handlers, api, "setSessionDays", "/api/campaigns/{campaignId}/sessions/{sessionId}/days",
		"Correct a played session's colony days", (*core.Service).SetSessionDays)
	registerSessionEdit(handlers, api, "setSessionRecap", "/api/campaigns/{campaignId}/sessions/{sessionId}/recap",
		"Write a session's recap, shown to everyone", (*core.Service).SetSessionRecap)
	registerSessionEdit(handlers, api, "setSessionTitle", "/api/campaigns/{campaignId}/sessions/{sessionId}/title",
		"Rename a played session (upcoming dates take their series' title)", (*core.Service).SetSessionTitle)

	Register(api, AccessDm, huma.Operation{
		OperationID: "setNpcFirstSession",
		Method:      http.MethodPut,
		Path:        "/api/campaigns/{campaignId}/npcs/{npcId}/first-session",
		Summary:     "Set the session where the party first met an NPC, or clear it with null",
		Tags:        []string{"npcs"},
	}, func(requestContext context.Context, input *npcBodyInput[core.NpcFirstSessionInput]) (*bodyOutput[core.Npc], error) {
		npc, err := handlers.core.SetNpcFirstSession(requestContext, viewerFrom(requestContext), input.NpcID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Npc]{Body: npc}, nil
	})

	Register(api, AccessMember, huma.Operation{
		OperationID: "getCalendarLink",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/calendar",
		Summary:     "The campaign's ICS subscription address; every member may subscribe",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.CalendarLink], error) {
		link, err := handlers.core.GetCalendarLink(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CalendarLink]{Body: link}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID: "regenerateCalendarToken",
		Method:      http.MethodPost,
		Path:        "/api/campaigns/{campaignId}/calendar/regenerate",
		Summary:     "Replace the ICS token; every old subscription stops working at once",
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.CalendarLink], error) {
		link, err := handlers.core.RegenerateCalendarToken(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.CalendarLink]{Body: link}, nil
	})
}

func registerSessionList(handlers *handlers, api huma.API, operationID, path, summary string,
	run func(*core.Service, context.Context, core.Viewer) ([]core.Session, error),
) {
	Register(api, AccessMember, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodGet,
		Path:        path,
		Summary:     summary,
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[[]core.Session], error) {
		sessions, err := run(handlers.core, requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[[]core.Session]{Body: sessions}, nil
	})
}

// registerSessionDate registers one POST on a date named by its series and local date, which may have
// no stored row yet.
func registerSessionDate(handlers *handlers, api huma.API, operationID, path, summary string,
	run func(*core.Service, context.Context, core.Viewer, core.SessionDateInput) (core.Session, error),
) {
	Register(api, AccessDm, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodPost,
		Path:        path,
		Summary:     summary,
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, input *campaignBodyInput[core.SessionDateInput]) (*bodyOutput[core.Session], error) {
		session, err := run(handlers.core, requestContext, viewerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Session]{Body: session}, nil
	})
}

func registerSessionEdit[Input any](handlers *handlers, api huma.API, operationID, path, summary string,
	run func(*core.Service, context.Context, core.Viewer, uuid.UUID, Input) (core.Session, error),
) {
	Register(api, AccessDm, huma.Operation{
		OperationID: operationID,
		Method:      http.MethodPut,
		Path:        path,
		Summary:     summary,
		Tags:        []string{"sessions"},
	}, func(requestContext context.Context, input *sessionBodyInput[Input]) (*bodyOutput[core.Session], error) {
		session, err := run(handlers.core, requestContext, viewerFrom(requestContext), input.SessionID, input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.Session]{Body: session}, nil
	})
}

// calendarFeed is GET /calendar/{token}.ics (token level). The token is the only credential a calendar
// app sends, so a wrong, regenerated or paused campaign's token reads exactly like a route that
// doesn't exist, and the feed never says whether a campaign is behind it.
func (handlers *handlers) calendarFeed() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		token, isCalendar := strings.CutSuffix(request.PathValue(calendarFileParameter), ".ics")
		if !isCalendar {
			writeMissingRoute(writer)
			return
		}
		feed, err := handlers.core.CalendarFeed(request.Context(), token)
		if err != nil {
			if operationError, typed := core.AsError(err); typed && operationError.Kind == core.KindNotFound {
				writeMissingRoute(writer)
				return
			}
			handlers.logger.ErrorContext(request.Context(), "calendar feed failed", slog.Any("error", err))
			auth.WriteProblem(writer, http.StatusInternalServerError, "Something went wrong.")
			return
		}
		writer.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		// Calendar apps poll; a kept copy would outlive a regenerated token.
		writer.Header().Set("Cache-Control", "no-store")
		_, _ = writer.Write([]byte(feed))
	})
}
