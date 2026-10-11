package app

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// AccessLevel says who may call a route. Every huma operation and every route-table entry declares
// one, and New refuses to build a server if any is missing (spec §2.2).
type AccessLevel string

const (
	// AccessPublic routes return nothing a visitor may not see: sign-in, /healthz, the SPA shell.
	AccessPublic AccessLevel = "public"
	// AccessToken routes authorize themselves with a secret in the URL (the ICS feed) and serve only
	// data players can already see.
	AccessToken AccessLevel = "token"
	// AccessPending admits any signed-in member, whatever their status. Only GET /api/me,
	// GET /api/stream and POST /auth/logout use it.
	AccessPending AccessLevel = "pending"
	AccessMember  AccessLevel = "member"
	// AccessDm admits the DM, and not while view as player is on.
	AccessDm AccessLevel = "dm"
)

const accessExtension = "x-access"

var accessLevels = []AccessLevel{AccessPublic, AccessToken, AccessPending, AccessMember, AccessDm}

func (level AccessLevel) valid() bool {
	return slices.Contains(accessLevels, level)
}

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

// accessDecision is the one place access levels are interpreted. A zero status means allowed.
func accessDecision(level AccessLevel, viewer core.Viewer, signedIn bool) (int, string) {
	switch level {
	case AccessPublic, AccessToken:
		return 0, ""
	case AccessPending:
		if !signedIn {
			return http.StatusUnauthorized, "Sign in first."
		}
		return 0, ""
	case AccessMember:
		if !signedIn {
			return http.StatusUnauthorized, "Sign in first."
		}
		if !viewer.IsActive() {
			return http.StatusForbidden, "The DM hasn't let you in."
		}
		return 0, ""
	case AccessDm:
		if !signedIn {
			return http.StatusUnauthorized, "Sign in first."
		}
		if !viewer.IsDM {
			return http.StatusForbidden, "Only the DM can do that."
		}
		if viewer.ViewAsPlayer {
			return http.StatusForbidden, "View as player is on."
		}
		return 0, ""
	default:
		return http.StatusInternalServerError, "This route declares no access level."
	}
}

func operationAccessMiddleware(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(humaContext huma.Context, next func(huma.Context)) {
		viewer, signedIn := auth.ViewerFromContext(humaContext.Context())
		status, message := accessDecision(operationAccess(humaContext.Operation()), viewer, signedIn)
		if status != 0 {
			_ = huma.WriteErr(api, humaContext, status, message)
			return
		}
		next(humaContext)
	}
}

func guardRoute(route Route) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		viewer, signedIn := auth.ViewerFromContext(request.Context())
		status, message := accessDecision(route.Access, viewer, signedIn)
		if status != 0 {
			auth.WriteProblem(writer, status, message)
			return
		}
		route.Handler.ServeHTTP(writer, request)
	})
}

// checkAccessDeclarations walks the OpenAPI document and the route table and lists every route
// without a valid access level, including huma operations registered around Register.
func checkAccessDeclarations(api huma.API, routes []Route) error {
	var undeclared []string
	for path, pathItem := range api.OpenAPI().Paths {
		for method, operation := range pathOperations(pathItem) {
			if !operationAccess(operation).valid() {
				undeclared = append(undeclared, method+" "+path)
			}
		}
	}
	for _, route := range routes {
		if !route.Access.valid() || route.Handler == nil {
			undeclared = append(undeclared, route.Pattern)
		}
	}
	if len(undeclared) > 0 {
		slices.Sort(undeclared)
		return fmt.Errorf("refusing to start: these routes declare no access level: %s", strings.Join(undeclared, ", "))
	}
	return nil
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

// viewerFrom returns the request's Viewer. Handlers run only after the access middleware has
// admitted the request, so a missing Viewer means a public route.
func viewerFrom(requestContext context.Context) core.Viewer {
	viewer, _ := auth.ViewerFromContext(requestContext)
	return viewer
}
