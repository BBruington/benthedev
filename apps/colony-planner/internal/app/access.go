package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// AccessLevel says who may call a route. Every huma operation and every route-table entry declares
// one, and New refuses to build a server if any is missing or sits on the wrong kind of path
// (docs/design/multi-campaign.md §4.1).
type AccessLevel string

const (
	// AccessPublic routes return nothing a visitor may not see: sign-in, /healthz, the SPA shell.
	AccessPublic AccessLevel = "public"
	// AccessToken routes authorize themselves with a secret in the URL (the ICS feed) and serve only
	// data players can already see.
	AccessToken AccessLevel = "token"
	// AccessAccount admits any signed-in account: routes outside every campaign (my campaigns, invites,
	// campaign requests, sign out).
	AccessAccount AccessLevel = "account"
	// AccessOwner admits the site owner: everything under /api/admin/ and the API docs. View as player
	// has no effect here, and it confers nothing inside a campaign.
	AccessOwner AccessLevel = "owner"
	// AccessPending admits any membership in the path's campaign, whatever its status, even while the
	// campaign is suspended: GET .../me, GET .../stream and POST .../leave only.
	AccessPending AccessLevel = "pending"
	// AccessMember admits an ACTIVE membership of an ACTIVE campaign.
	AccessMember AccessLevel = "member"
	// AccessDm admits the campaign's DM, not while view as player is on, in an ACTIVE campaign.
	AccessDm AccessLevel = "dm"
)

const (
	accessExtension = "x-access"
	// campaignParameter is the path parameter every campaign-level route carries.
	campaignParameter = "campaignId"
	adminPathPrefix   = "/api/admin/"
)

var accessLevels = []AccessLevel{AccessPublic, AccessToken, AccessAccount, AccessOwner, AccessPending, AccessMember, AccessDm}

func (level AccessLevel) valid() bool {
	return slices.Contains(accessLevels, level)
}

// inCampaign reports whether the level is resolved against a campaign membership.
func (level AccessLevel) inCampaign() bool {
	return level == AccessPending || level == AccessMember || level == AccessDm
}

// Messages of the access layer. A non-member, the site owner included, reads exactly what a missing
// campaign reads.
const (
	messageSignIn    = "Sign in first."
	messageNotLetIn  = "The DM hasn't let you in."
	messageOnlyDm    = "Only the DM can do that."
	messageViewing   = "View as player is on."
	messageOnlyOwner = "Only the site owner can do that."
	messageNoLevel   = "This route declares no access level."
)

// Register is the only way to add a huma operation: it records the access level as the operation's
// x-access extension, which the access middleware enforces and the secrecy test reads.
func Register[Input, Output any](api huma.API, level AccessLevel, operation huma.Operation, handler func(context.Context, *Input) (*Output, error)) {
	if !level.valid() {
		panic(fmt.Sprintf("app.Register %s %s: unknown access level %q", operation.Method, operation.Path, level))
	}
	if operation.Extensions == nil {
		operation.Extensions = map[string]any{}
	}
	operation.Extensions[accessExtension] = string(level)
	huma.Register(api, operation, handler)
}

func operationAccess(operation *huma.Operation) AccessLevel {
	if operation == nil {
		return ""
	}
	level, _ := operation.Extensions[accessExtension].(string)
	return AccessLevel(level)
}

// accessRequest is what one access decision looks at: the caller (if signed in) and, for the three
// campaign levels, the Viewer core built for the path's campaign (if the caller has a membership).
type accessRequest struct {
	caller     core.Caller
	signedIn   bool
	viewer     core.Viewer
	isMember   bool
	accessedAs AccessLevel
}

// accessDecision is the one place access levels are interpreted. A zero status means allowed. Owner
// status is consulted only for the owner level: inside a campaign only the Viewer counts.
func accessDecision(request accessRequest) (int, string) {
	switch request.accessedAs {
	case AccessPublic, AccessToken:
		return 0, ""
	case AccessAccount, AccessOwner, AccessPending, AccessMember, AccessDm:
	default:
		return http.StatusInternalServerError, messageNoLevel
	}
	if !request.signedIn {
		return http.StatusUnauthorized, messageSignIn
	}
	switch request.accessedAs {
	case AccessAccount:
		return 0, ""
	case AccessOwner:
		if !request.caller.IsSiteOwner {
			return http.StatusForbidden, messageOnlyOwner
		}
		return 0, ""
	}
	if !request.isMember {
		return http.StatusNotFound, core.ErrNoSuchCampaign.Message
	}
	viewer := request.viewer
	switch request.accessedAs {
	case AccessPending:
		return 0, ""
	case AccessMember:
		if !viewer.IsActive() {
			return http.StatusForbidden, messageNotLetIn
		}
		if !viewer.CampaignActive() {
			return http.StatusForbidden, core.ErrCampaignSuspended.Message
		}
		return 0, ""
	default:
		if !viewer.IsActive() {
			return http.StatusForbidden, messageNotLetIn
		}
		if !viewer.CampaignActive() {
			return http.StatusForbidden, core.ErrCampaignSuspended.Message
		}
		if !viewer.IsDM() {
			return http.StatusForbidden, messageOnlyDm
		}
		if viewer.ViewAsPlayer() {
			return http.StatusForbidden, messageViewing
		}
		return 0, ""
	}
}

// accessGate resolves the caller and, for a campaign level, the path's campaign, then decides. It
// runs before huma parses any input, so an unparsable campaign ID answers the same 404 as an unknown
// one, never a 422. On success it returns the context the handler runs with (holding the Viewer).
type accessGate struct {
	core   *core.Service
	logger *slog.Logger
}

func (gate accessGate) admit(requestContext context.Context, level AccessLevel, campaignIDText string) (context.Context, int, string) {
	request := accessRequest{accessedAs: level}
	request.caller, request.signedIn = auth.CallerFromContext(requestContext)
	if request.signedIn && level.inCampaign() {
		campaignID, err := uuid.Parse(campaignIDText)
		if err == nil {
			request.viewer, err = gate.core.EnterCampaign(requestContext, request.caller, campaignID)
			switch {
			case err == nil:
				request.isMember = true
			case isNoSuchCampaign(err):
			default:
				gate.logger.ErrorContext(requestContext, "campaign membership read failed", slog.Any("error", err))
				return requestContext, http.StatusServiceUnavailable, "The server can't check your campaign right now. Try again in a moment."
			}
		}
	}
	status, message := accessDecision(request)
	if status != 0 {
		return requestContext, status, message
	}
	if request.isMember {
		requestContext = withCampaignViewer(requestContext, request.viewer)
	}
	return requestContext, 0, ""
}

func isNoSuchCampaign(err error) bool {
	operationError, typed := core.AsError(err)
	return typed && operationError.Kind == core.KindNotFound
}

func operationAccessMiddleware(api huma.API, gate accessGate) func(huma.Context, func(huma.Context)) {
	return func(humaContext huma.Context, next func(huma.Context)) {
		level := operationAccess(humaContext.Operation())
		admitted, status, message := gate.admit(humaContext.Context(), level, humaContext.Param(campaignParameter))
		if status != 0 {
			_ = huma.WriteErr(api, humaContext, status, message)
			return
		}
		next(huma.WithContext(humaContext, admitted))
	}
}

func guardRoute(route Route, gate accessGate) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		admitted, status, message := gate.admit(request.Context(), route.Access, request.PathValue(campaignParameter))
		if status != 0 {
			auth.WriteProblem(writer, status, message)
			return
		}
		route.Handler.ServeHTTP(writer, request.WithContext(admitted))
	})
}

// checkAccessDeclarations walks the OpenAPI document and the route table and refuses to start when a
// route has no valid access level, or when its level doesn't fit its path: pending, member and dm if
// and only if the path has {campaignId} and is outside /api/admin/; owner only under /api/admin/ (and
// the two docs routes); everything under /api/admin/ at owner.
func checkAccessDeclarations(api huma.API, routes []Route) error {
	var undeclared, misplaced []string
	check := func(label string, path string, level AccessLevel) {
		if !level.valid() {
			undeclared = append(undeclared, label)
			return
		}
		if problem := levelPathProblem(path, level); problem != "" {
			misplaced = append(misplaced, label+" ("+problem+")")
		}
	}
	for path, pathItem := range api.OpenAPI().Paths {
		for method, operation := range pathOperations(pathItem) {
			check(method+" "+path, path, operationAccess(operation))
		}
	}
	for _, route := range routes {
		_, path, _ := strings.Cut(route.Pattern, " ")
		if route.Handler == nil {
			undeclared = append(undeclared, route.Pattern)
			continue
		}
		check(route.Pattern, path, route.Access)
	}
	var problems []string
	if len(undeclared) > 0 {
		slices.Sort(undeclared)
		problems = append(problems, "these routes declare no access level: "+strings.Join(undeclared, ", "))
	}
	if len(misplaced) > 0 {
		slices.Sort(misplaced)
		problems = append(problems, "these routes' access levels don't fit their paths: "+strings.Join(misplaced, ", "))
	}
	if len(problems) > 0 {
		return fmt.Errorf("refusing to start: %s", strings.Join(problems, "; "))
	}
	return nil
}

// ownerRoutesOutsideAdmin are the API description and its docs page: they describe every route,
// so only the site owner reads them.
var ownerRoutesOutsideAdmin = []string{"/api/openapi.json", "/api/docs"}

func levelPathProblem(path string, level AccessLevel) string {
	underAdmin := strings.HasPrefix(path, adminPathPrefix)
	hasCampaign := strings.Contains(path, "{"+campaignParameter+"}")
	switch {
	case underAdmin && level != AccessOwner:
		return "every /api/admin/ route is owner"
	case level == AccessOwner && !underAdmin && !slices.Contains(ownerRoutesOutsideAdmin, path):
		return "owner routes live under /api/admin/"
	case level.inCampaign() && !hasCampaign:
		return "a campaign level needs a {campaignId} path parameter"
	case !underAdmin && hasCampaign && !level.inCampaign():
		return "a {campaignId} route outside /api/admin/ must be pending, member or dm"
	}
	return ""
}

func pathOperations(pathItem *huma.PathItem) map[string]*huma.Operation {
	operations := map[string]*huma.Operation{}
	for method, operation := range map[string]*huma.Operation{
		http.MethodGet:     pathItem.Get,
		http.MethodPut:     pathItem.Put,
		http.MethodPost:    pathItem.Post,
		http.MethodDelete:  pathItem.Delete,
		http.MethodOptions: pathItem.Options,
		http.MethodHead:    pathItem.Head,
		http.MethodPatch:   pathItem.Patch,
		http.MethodTrace:   pathItem.Trace,
	} {
		if operation != nil {
			operations[method] = operation
		}
	}
	return operations
}

type campaignViewerContextKey struct{}

// withCampaignViewer stores the Viewer core.EnterCampaign built for the request's {campaignId}; only
// the access gate calls it.
func withCampaignViewer(requestContext context.Context, viewer core.Viewer) context.Context {
	return context.WithValue(requestContext, campaignViewerContextKey{}, viewer)
}

func campaignViewerFromContext(requestContext context.Context) (core.Viewer, bool) {
	viewer, found := requestContext.Value(campaignViewerContextKey{}).(core.Viewer)
	return viewer, found
}

// viewerFrom returns the request's campaign Viewer, or the zero Viewer, which core refuses.
func viewerFrom(requestContext context.Context) core.Viewer {
	viewer, _ := campaignViewerFromContext(requestContext)
	return viewer
}

// callerFrom returns the signed-in Caller of an account or owner route, or the zero Caller, which
// core refuses.
func callerFrom(requestContext context.Context) core.Caller {
	caller, _ := auth.CallerFromContext(requestContext)
	return caller
}
