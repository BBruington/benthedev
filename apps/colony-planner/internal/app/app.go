// Package app is the HTTP transport and nothing else: huma operations that decode, call one
// internal/core function and encode; the route table for everything that isn't a huma operation;
// access enforcement; and the redaction Transformer. Swapping the API toolchain rewrites only this
// package (spec §8.6).
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

type Options struct {
	Core   *core.Service
	Auth   *auth.Service
	Logger *slog.Logger
	// StreamHeartbeat and StreamLifetime default to 20 seconds and 10 minutes; tests shorten them.
	StreamHeartbeat time.Duration
	StreamLifetime  time.Duration
	// ImageUploadReadTimeout bounds reading one image upload; it defaults to 2 minutes.
	ImageUploadReadTimeout time.Duration
	// RequestReadTimeout bounds reading any other request, body included; it defaults to 30 seconds.
	RequestReadTimeout time.Duration
}

// Route is one entry of the route table: every non-huma route, each with its access level.
type Route struct {
	Pattern string
	Access  AccessLevel
	Handler http.Handler
}

type Server struct {
	api     huma.API
	handler http.Handler
	// routes is the route table, kept so the secrecy test can call every entry.
	routes             []Route
	requestReadTimeout time.Duration
}

// defaultRequestReadTimeout is far longer than any request but the two that set their own deadline:
// the image upload extends it and the event stream clears it.
const defaultRequestReadTimeout = 30 * time.Second

// handlers carries the dependencies the per-feature handler files use.
type handlers struct {
	core            *core.Service
	auth            *auth.Service
	logger          *slog.Logger
	streamHeartbeat time.Duration
	streamLifetime  time.Duration
	// imageUploadReadTimeout bounds reading one image upload's body.
	imageUploadReadTimeout time.Duration
}

func New(options Options) (*Server, error) {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	serveMux := http.NewServeMux()
	api := humago.New(serveMux, humaConfig())
	gate := accessGate{core: options.Core, logger: logger}
	api.UseMiddleware(operationAccessMiddleware(api, gate))
	registerSchemas(api)

	featureHandlers := &handlers{
		core:                   options.Core,
		auth:                   options.Auth,
		logger:                 logger,
		streamHeartbeat:        options.StreamHeartbeat,
		streamLifetime:         options.StreamLifetime,
		imageUploadReadTimeout: options.ImageUploadReadTimeout,
	}
	if featureHandlers.imageUploadReadTimeout <= 0 {
		featureHandlers.imageUploadReadTimeout = defaultImageUploadReadTimeout
	}
	if featureHandlers.streamHeartbeat <= 0 {
		featureHandlers.streamHeartbeat = defaultStreamHeartbeat
	}
	if featureHandlers.streamLifetime <= 0 {
		featureHandlers.streamLifetime = defaultStreamLifetime
	}
	featureHandlers.registerLogin(api)
	featureHandlers.registerAccount(api)
	featureHandlers.registerAdmin(api)
	featureHandlers.registerCampaign(api)
	featureHandlers.registerDiscord(api)
	featureHandlers.registerMe(api)
	featureHandlers.registerMembers(api)
	featureHandlers.registerSettings(api)
	featureHandlers.registerColony(api)
	featureHandlers.registerTimeline(api)
	featureHandlers.registerResources(api)
	featureHandlers.registerPeople(api)
	featureHandlers.registerNpcs(api)
	featureHandlers.registerPlaces(api)
	featureHandlers.registerQuests(api)
	featureHandlers.registerPatrons(api)
	featureHandlers.registerRequests(api)
	featureHandlers.registerImages(api)
	featureHandlers.registerMaps(api)
	featureHandlers.registerHexes(api)
	featureHandlers.registerGameSessions(api)
	nullableEnumFields(api.OpenAPI())

	openAPIJSON, err := json.Marshal(api.OpenAPI())
	if err != nil {
		return nil, fmt.Errorf("app: encode the OpenAPI document: %w", err)
	}

	routes := []Route{
		{Pattern: "GET /healthz", Access: AccessPublic, Handler: http.HandlerFunc(serveHealth)},
		{Pattern: "GET /auth/discord/login", Access: AccessPublic, Handler: options.Auth.DiscordLogin()},
		{Pattern: "GET /auth/discord/callback", Access: AccessPublic, Handler: options.Auth.DiscordCallback()},
		{Pattern: "POST /auth/logout", Access: AccessAccount, Handler: options.Auth.Logout()},
		{Pattern: "POST /auth/logout-everywhere", Access: AccessAccount, Handler: options.Auth.LogoutEverywhere()},
		{Pattern: "GET /api/campaigns/{campaignId}/stream", Access: AccessPending, Handler: featureHandlers.stream()},
		{Pattern: "GET /calendar/{" + calendarFileParameter + "}", Access: AccessToken, Handler: featureHandlers.calendarFeed()},
		{Pattern: "GET /api/openapi.json", Access: AccessOwner, Handler: serveOpenAPI(openAPIJSON)},
		{Pattern: "GET /api/docs", Access: AccessOwner, Handler: http.HandlerFunc(serveDocs)},
		{Pattern: "GET /", Access: AccessPublic, Handler: spaHandler()},
	}
	if devSignIn := options.Auth.DevSignIn(); devSignIn != nil {
		routes = append(routes, Route{Pattern: "GET /auth/dev", Access: AccessPublic, Handler: devSignIn})
	}

	if err := checkAccessDeclarations(api, routes); err != nil {
		return nil, err
	}
	for _, route := range routes {
		serveMux.Handle(route.Pattern, guardRoute(route, gate))
	}

	var handler http.Handler = serveMux
	handler = requireJSONBodies(handler)
	handler = options.Auth.Middleware(handler)
	handler = http.NewCrossOriginProtection().Handler(handler)
	handler = options.Auth.RequireKnownHost(handler)
	handler = securityHeaders(handler)
	handler = logRequests(handler, logger)
	requestReadTimeout := options.RequestReadTimeout
	if requestReadTimeout <= 0 {
		requestReadTimeout = defaultRequestReadTimeout
	}
	return &Server{api: api, handler: handler, routes: routes, requestReadTimeout: requestReadTimeout}, nil
}

func (server *Server) Handler() http.Handler {
	return server.handler
}

// HTTPServer serves Handler with read limits. ReadTimeout covers the body too, so a body that stalls
// frees its connection even when it is answered unread (net/http drains up to 256 KB of it first).
// There is no WriteTimeout, which would end every event stream.
func (server *Server) HTTPServer(baseContext func(net.Listener) context.Context) *http.Server {
	return &http.Server{
		Handler:           server.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       server.requestReadTimeout,
		IdleTimeout:       2 * time.Minute,
		BaseContext:       baseContext,
	}
}

// OpenAPI returns the API description that `colony openapi` writes to web/src/api/openapi.json.
func (server *Server) OpenAPI() *huma.OpenAPI {
	return server.api.OpenAPI()
}

func init() {
	// Lists are always JSON arrays, never null; core returns empty slices instead of nil ones.
	huma.DefaultArrayNullable = false
}

func humaConfig() huma.Config {
	config := huma.DefaultConfig("Colony Planner", "1.0.0")
	// huma's built-in /openapi, /docs and /schemas routes would bypass the access check, so they are
	// off; the document and docs page are served again as owner routes.
	config.OpenAPIPath = ""
	config.DocsPath = ""
	config.SchemasPath = ""
	// The default create hook adds `$schema` links to the disabled /schemas route.
	config.CreateHooks = nil
	config.Transformers = []huma.Transformer{redactDmFields}
	return config
}

func serveHealth(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	_, _ = writer.Write([]byte("ok\n"))
}

// requireJSONBodies rejects any request body that isn't JSON, so an HTML form can never drive a
// mutation (spec §2.2, item 7). The image upload alone takes multipart, still behind
// CrossOriginProtection, and its size is capped here before anything reads the body.
func requireJSONBodies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.ContentLength != 0 && request.Method != http.MethodGet && request.Method != http.MethodHead {
			mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
			switch {
			case isImageUpload(request):
				if err != nil || mediaType != "multipart/form-data" {
					auth.WriteProblem(writer, http.StatusUnsupportedMediaType, "Upload images as multipart/form-data.")
					return
				}
				if request.ContentLength > imageUploadMaxBodyBytes {
					auth.WriteProblem(writer, http.StatusRequestEntityTooLarge, imageTooLargeMessage)
					return
				}
				request.Body = http.MaxBytesReader(writer, request.Body, imageUploadMaxBodyBytes)
			case err != nil || mediaType != "application/json":
				auth.WriteProblem(writer, http.StatusUnsupportedMediaType, "Request bodies must be application/json.")
				return
			}
		}
		next.ServeHTTP(writer, request)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "same-origin")
		writer.Header().Set("X-Frame-Options", "DENY")
		// API and sign-in answers are per viewer, so no browser or proxy may keep one: a stored DM
		// projection could be served to a player who asks for the same URL. Set before the handler runs,
		// so huma's own error responses carry it and a handler may still choose another policy.
		if strings.HasPrefix(request.URL.Path, "/api/") || strings.HasPrefix(request.URL.Path, "/auth/") {
			writer.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(writer, request)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the underlying writer, which SSE needs for flushing.
func (recorder *statusRecorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

func logRequests(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)
		logger.LogAttrs(request.Context(), slog.LevelInfo, "request",
			slog.String("method", request.Method),
			slog.String("path", loggedPath(request.URL.Path)),
			slog.Int("status", recorder.status),
			slog.Duration("duration", time.Since(started).Round(time.Microsecond)),
		)
	})
}

// secretPathSegments are the paths whose next segment is a secret: the API's invite routes and the
// SPA's join page carry an invite code, and the ICS feed carries its token.
var secretPathSegments = []struct{ prefix, placeholder string }{
	{prefix: "/api/invites/", placeholder: "{inviteCode}"},
	{prefix: "/join/", placeholder: "{inviteCode}"},
	{prefix: "/calendar/", placeholder: "{icsToken}"},
}

// loggedPath keeps invite codes and ICS tokens out of the log (Invariant 14): a code lets anyone ask
// to join a campaign and a token reads its schedule, while the log is the operator's. The path is
// cleaned first, as ServeMux does before redirecting, so "//calendar/…" or "/x/../calendar/…" is
// redacted too.
func loggedPath(rawPath string) string {
	cleanPath := path.Clean("/" + rawPath)
	if strings.HasSuffix(rawPath, "/") && cleanPath != "/" {
		cleanPath += "/"
	}
	for _, secretPath := range secretPathSegments {
		rest, found := strings.CutPrefix(cleanPath, secretPath.prefix)
		if !found || rest == "" {
			continue
		}
		if _, suffix, hasSuffix := strings.Cut(rest, "/"); hasSuffix {
			return secretPath.prefix + secretPath.placeholder + "/" + suffix
		}
		return secretPath.prefix + secretPath.placeholder
	}
	return cleanPath
}
