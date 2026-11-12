package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

// The Foundry bridge's HTTP side (docs/design/foundry-bridge.md §3 and §12). Under /api/bridge/ the
// session cookie is never read (auth.Middleware), so CrossOriginProtection is bypassed there and the
// bearer token is the only credential; bridgeGuard resolves it, caps bodies, rate-limits and answers
// CORS for the token's own link origin only.

const (
	bridgePathPrefix     = auth.BridgePathPrefix
	bridgePairPath       = "/api/bridge/pair"
	bridgeMaxBodyBytes   = 256 * 1024
	bridgePreflightAge   = "7200"
	bridgeAllowedHeaders = "Authorization, Content-Type"
	bridgeAllowedMethods = "GET, POST"
	// bridgeLinkRequestsPerMinute and bridgePairRequestsPerMinute are design §3.3's limits.
	bridgeLinkRequestsPerMinute = 120
	bridgePairRequestsPerMinute = 30
)

func (handlers *handlers) registerBridge(api huma.API) {
	Register(api, AccessDm, huma.Operation{
		OperationID: "getBridgeStatus",
		Method:      http.MethodGet,
		Path:        "/api/campaigns/{campaignId}/bridge",
		Summary:     "The Foundry bridge: the connected world, an open pairing code and the DM's inbox",
		Tags:        []string{"bridge"},
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.BridgeStatus], error) {
		status, err := handlers.core.GetBridgeStatus(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BridgeStatus]{Body: status}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "createBridgePairingCode",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/bridge/codes",
		Summary:       "A one-time code for the Foundry module, good for 10 minutes; it replaces an unused one",
		Tags:          []string{"bridge"},
		DefaultStatus: http.StatusCreated,
	}, func(requestContext context.Context, _ *campaignPath) (*bodyOutput[core.BridgePairingCode], error) {
		code, err := handlers.core.CreateBridgePairingCode(requestContext, viewerFrom(requestContext))
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BridgePairingCode]{Body: code}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "revokeBridge",
		Method:        http.MethodDelete,
		Path:          "/api/campaigns/{campaignId}/bridge",
		Summary:       "Disconnect Foundry: revoke the link and its unused code, keeping the rows for the record",
		Tags:          []string{"bridge"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, _ *campaignPath) (*struct{}, error) {
		if err := handlers.core.RevokeBridge(requestContext, viewerFrom(requestContext)); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "clearBridgeFailedRows",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/bridge/failed/clear",
		Summary:       "Forget the changes Foundry couldn't apply",
		Tags:          []string{"bridge"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, _ *campaignPath) (*struct{}, error) {
		if err := handlers.core.ClearBridgeFailedRows(requestContext, viewerFrom(requestContext)); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessDm, huma.Operation{
		OperationID:   "dismissBridgeInboxItem",
		Method:        http.MethodPost,
		Path:          "/api/campaigns/{campaignId}/bridge/inbox/{eventId}/dismiss",
		Summary:       "Dismiss a Foundry inbox item; one already handled is 409",
		Tags:          []string{"bridge"},
		DefaultStatus: http.StatusNoContent,
	}, func(requestContext context.Context, input *bridgeEventPath) (*struct{}, error) {
		if err := handlers.core.DismissBridgeInboxItem(requestContext, viewerFrom(requestContext), input.EventID); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessPublic, huma.Operation{
		OperationID:  "pairBridge",
		Method:       http.MethodPost,
		Path:         bridgePairPath,
		Summary:      "Trade a pairing code for the campaign's bridge token (the Foundry module, from a Forge game's page)",
		Tags:         []string{"bridge"},
		MaxBodyBytes: bridgeMaxBodyBytes,
	}, func(requestContext context.Context, input *bridgePairInput) (*bodyOutput[core.BridgePairing], error) {
		pairing, err := handlers.core.PairBridge(requestContext, input.Origin, input.Body)
		if errors.Is(err, core.ErrBridgeCodeRejected) {
			return nil, huma.Error400BadRequest(core.ErrBridgeCodeRejected.Message)
		}
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BridgePairing]{Body: pairing}, nil
	})

	Register(api, AccessToken, huma.Operation{
		OperationID: "readBridgeOutbox",
		Method:      http.MethodGet,
		Path:        "/api/bridge/outbox",
		Summary:     "Up to 100 unapplied outbox rows after a seq, oldest first (bridge token)",
		Tags:        []string{"bridge"},
	}, func(requestContext context.Context, input *bridgeOutboxQuery) (*bodyOutput[core.BridgeOutboxPage], error) {
		page, err := handlers.core.ReadBridgeOutbox(requestContext, bridgeCallerFrom(requestContext), input.After)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BridgeOutboxPage]{Body: page}, nil
	})

	Register(api, AccessToken, huma.Operation{
		OperationID:   "ackBridgeOutbox",
		Method:        http.MethodPost,
		Path:          "/api/bridge/acks",
		Summary:       "Mark outbox rows applied, or failed with a reason (bridge token)",
		Tags:          []string{"bridge"},
		DefaultStatus: http.StatusNoContent,
		MaxBodyBytes:  bridgeMaxBodyBytes,
	}, func(requestContext context.Context, input *bodyInput[core.BridgeAckInput]) (*struct{}, error) {
		if err := handlers.core.AckBridgeOutbox(requestContext, bridgeCallerFrom(requestContext), input.Body); err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &struct{}{}, nil
	})

	Register(api, AccessToken, huma.Operation{
		OperationID:  "recordBridgeEvents",
		Method:       http.MethodPost,
		Path:         "/api/bridge/events",
		Summary:      "Report 1 to 100 Foundry events; a repeated event ID is ignored (bridge token)",
		Tags:         []string{"bridge"},
		MaxBodyBytes: bridgeMaxBodyBytes,
	}, func(requestContext context.Context, input *bodyInput[core.BridgeEventsInput]) (*bodyOutput[core.BridgeEventsResult], error) {
		result, err := handlers.core.RecordBridgeEvents(requestContext, bridgeCallerFrom(requestContext), input.Body)
		if err != nil {
			return nil, handlers.fail(requestContext, err)
		}
		return &bodyOutput[core.BridgeEventsResult]{Body: result}, nil
	})
}

type bridgeEventPath struct {
	CampaignID uuid.UUID `path:"campaignId" format:"uuid" doc:"The campaign's ID (GET /api/campaigns lists the caller's)"`
	EventID    uuid.UUID `path:"eventId" format:"uuid"`
}

type bridgePairInput struct {
	Origin string `header:"Origin" doc:"Set by the browser; a Forge game's page"`
	Body   core.BridgePairInput
}

type bridgeOutboxQuery struct {
	After int64 `query:"after" doc:"The newest seq the module has; 0 for everything waiting"`
}

type bridgeCallerContextKey struct{}

func bridgeCallerFrom(requestContext context.Context) core.BridgeCaller {
	caller, _ := requestContext.Value(bridgeCallerContextKey{}).(core.BridgeCaller)
	return caller
}

// bridgeGuard runs on every /api/bridge/ request before the route. Preflights go to the route table's
// OPTIONS entry; /pair is public under its own per-IP limit; everything else needs a live token.
func (handlers *handlers) bridgeGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasPrefix(request.URL.Path, bridgePathPrefix) {
			next.ServeHTTP(writer, request)
			return
		}
		writer.Header().Add("Vary", "Origin")
		if request.ContentLength > bridgeMaxBodyBytes {
			auth.WriteProblem(writer, http.StatusRequestEntityTooLarge, "Bridge request bodies are at most 256 KiB.")
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, bridgeMaxBodyBytes)
		origin := request.Header.Get("Origin")
		switch {
		case request.Method == http.MethodOptions:
			next.ServeHTTP(writer, request)
		case request.URL.Path == bridgePairPath:
			if handlers.core.BridgeOriginAllowed(origin) {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
			}
			if !handlers.allowBridgeRequest(writer, handlers.pairLimiter, clientAddress(request)) {
				return
			}
			next.ServeHTTP(writer, request)
		default:
			caller, err := handlers.core.EnterBridge(request.Context(), bearerToken(request))
			if errors.Is(err, core.ErrBridgeUnauthorized) {
				auth.WriteProblem(writer, http.StatusUnauthorized, "")
				return
			}
			if err != nil {
				handlers.logger.ErrorContext(request.Context(), "bridge token lookup failed", slog.Any("error", err))
				auth.WriteProblem(writer, http.StatusServiceUnavailable, "The server can't check the bridge right now. Try again in a moment.")
				return
			}
			if origin != "" && origin == caller.Origin() {
				writer.Header().Set("Access-Control-Allow-Origin", origin)
			}
			if !handlers.allowBridgeRequest(writer, handlers.linkLimiter, caller.LinkID().String()) {
				return
			}
			next.ServeHTTP(writer, request.WithContext(context.WithValue(request.Context(), bridgeCallerContextKey{}, caller)))
		}
	})
}

func (handlers *handlers) allowBridgeRequest(writer http.ResponseWriter, limiter *rateLimiter, key string) bool {
	allowed, retryAfter := limiter.allow(key)
	if !allowed {
		writer.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
		auth.WriteProblem(writer, http.StatusTooManyRequests, "Too many bridge requests. Try again in a moment.")
	}
	return allowed
}

func bearerToken(request *http.Request) string {
	scheme, token, found := strings.Cut(request.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

// clientAddress is the address the per-IP limit counts. Behind Railway's proxy the last
// X-Forwarded-For entry is the one the proxy itself appended, so a client can't choose it.
func clientAddress(request *http.Request) string {
	if forwarded := request.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
		entries := strings.Split(forwarded[len(forwarded)-1], ",")
		return strings.TrimSpace(entries[len(entries)-1])
	}
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		return request.RemoteAddr
	}
	return host
}

// bridgePreflight is OPTIONS /api/bridge/ (public). A preflight carries no token, so it is answered for
// an origin with an active link, and on /pair for any origin a module may pair from.
func (handlers *handlers) bridgePreflight() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		allowed := false
		if request.URL.Path == bridgePairPath {
			allowed = handlers.core.BridgeOriginAllowed(origin)
		} else {
			linked, err := handlers.core.BridgeOriginLinked(request.Context(), origin)
			if err != nil {
				handlers.logger.ErrorContext(request.Context(), "bridge preflight failed", slog.Any("error", err))
				auth.WriteProblem(writer, http.StatusServiceUnavailable, "The server can't check the bridge right now. Try again in a moment.")
				return
			}
			allowed = linked
		}
		if !allowed {
			auth.WriteProblem(writer, http.StatusForbidden, "")
			return
		}
		header := writer.Header()
		header.Set("Access-Control-Allow-Origin", origin)
		header.Set("Access-Control-Allow-Methods", bridgeAllowedMethods)
		header.Set("Access-Control-Allow-Headers", bridgeAllowedHeaders)
		header.Set("Access-Control-Max-Age", bridgePreflightAge)
		writer.WriteHeader(http.StatusNoContent)
	})
}

// bridgeStream is GET /api/bridge/stream (token): `event: outbox` with the newest unapplied seq on
// connect when rows wait and after each bridge publish that raised it, `: ping` every 20 seconds, and
// an end at about 10 minutes. It reads the database only on connect and on a publish (Invariant 9).
func (handlers *handlers) bridgeStream() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestContext := request.Context()
		caller := bridgeCallerFrom(requestContext)
		subscription, err := handlers.core.SubscribeBridge(requestContext, caller)
		if errors.Is(err, core.ErrBridgeUnauthorized) {
			auth.WriteProblem(writer, http.StatusUnauthorized, "")
			return
		}
		if err != nil {
			handlers.logger.ErrorContext(requestContext, "bridge stream read failed", slog.String("link_id", caller.LinkID().String()), slog.Any("error", err))
			auth.WriteProblem(writer, http.StatusServiceUnavailable, "The server can't read the bridge right now. Try again in a moment.")
			return
		}
		defer subscription.Close()
		newest, err := handlers.core.NewestBridgeSeq(requestContext, caller)
		if err != nil {
			handlers.logger.ErrorContext(requestContext, "bridge stream read failed", slog.String("link_id", caller.LinkID().String()), slog.Any("error", err))
			auth.WriteProblem(writer, http.StatusServiceUnavailable, "The server can't read the bridge right now. Try again in a moment.")
			return
		}

		controller := http.NewResponseController(writer)
		header := writer.Header()
		header.Set("Content-Type", "text/event-stream")
		header.Set("Cache-Control", "no-store")
		header.Set("X-Accel-Buffering", "no")
		writer.WriteHeader(http.StatusOK)
		send := func(chunk string) bool {
			if err := controller.SetWriteDeadline(time.Now().Add(streamWriteTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				return false
			}
			if _, err := fmt.Fprint(writer, chunk); err != nil {
				return false
			}
			return controller.Flush() == nil
		}
		var sentSeq int64
		opening := ": ping\n\n"
		if newest > 0 {
			opening = formatBridgeOutboxMessage(newest)
			sentSeq = newest
		}
		if !send(opening) {
			return
		}

		heartbeat := time.NewTicker(handlers.streamHeartbeat)
		defer heartbeat.Stop()
		rotation := time.NewTimer(handlers.streamLifetime)
		defer rotation.Stop()
		for {
			select {
			case <-requestContext.Done():
				return
			case <-subscription.Closed():
				return
			case <-rotation.C:
				return
			case <-heartbeat.C:
				if !send(": ping\n\n") {
					return
				}
			case <-subscription.Messages():
				newest, err := handlers.core.NewestBridgeSeq(requestContext, caller)
				if err != nil {
					handlers.logger.WarnContext(requestContext, "bridge stream read failed", slog.String("link_id", caller.LinkID().String()), slog.Any("error", err))
					return
				}
				if newest <= sentSeq {
					continue
				}
				sentSeq = newest
				if !send(formatBridgeOutboxMessage(newest)) {
					return
				}
			}
		}
	})
}

func formatBridgeOutboxMessage(seq int64) string {
	data, _ := json.Marshal(map[string]int64{"seq": seq})
	return fmt.Sprintf("event: outbox\ndata: %s\n\n", data)
}

// rateLimiter counts requests per key in fixed one-minute windows, in memory (single replica,
// Invariant 8).
type rateLimiter struct {
	mutex   sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	windows map[string]rateWindow
}

type rateWindow struct {
	start time.Time
	count int
}

// rateLimiterKeysBeforeSweep is when allow drops ended windows, so idle keys don't pile up.
const rateLimiterKeysBeforeSweep = 1024

func newRateLimiter(limit int) *rateLimiter {
	return &rateLimiter{limit: limit, window: time.Minute, now: time.Now, windows: map[string]rateWindow{}}
}

// allow counts one request for key and reports whether it is within the limit, and if not how long
// until the window ends.
func (limiter *rateLimiter) allow(key string) (bool, time.Duration) {
	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()
	now := limiter.now()
	if len(limiter.windows) >= rateLimiterKeysBeforeSweep {
		for existing, window := range limiter.windows {
			if now.Sub(window.start) >= limiter.window {
				delete(limiter.windows, existing)
			}
		}
	}
	window := limiter.windows[key]
	if now.Sub(window.start) >= limiter.window {
		window = rateWindow{start: now}
	}
	if window.count >= limiter.limit {
		return false, limiter.window - now.Sub(window.start)
	}
	window.count++
	limiter.windows[key] = window
	return true, 0
}
