package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/BBruington/colony-planner/internal/auth"
	"github.com/BBruington/colony-planner/internal/core"
)

const (
	defaultStreamHeartbeat = 20 * time.Second
	// Streams end at about 10 minutes, under the 15-minute proxy cap of the M6 host; EventSource
	// reconnects with Last-Event-ID, which costs nothing when nothing changed.
	defaultStreamLifetime = 10 * time.Minute
	streamWriteTimeout    = 10 * time.Second
	// EventSource waits this long before reconnecting after a stream ends.
	streamRetryMilliseconds = 2000
)

// stream is GET /api/campaigns/{campaignId}/stream (pending level): Server-Sent Events carrying only
// `event: invalidate` messages whose data is a core.InvalidateMessage, with `id: <bootId>-<seq>`
// counting only this campaign's messages (docs/design/multi-campaign.md §7.1). It is a route-table
// route because huma's SSE helper only supports integer IDs. A connect or reconnect runs one read, the
// membership the access gate resolves; the open stream runs none, and heartbeats are comments.
func (handlers *handlers) stream() http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		lastEventID := request.Header.Get("Last-Event-ID")
		if lastEventID == "" {
			// A tab that closed its stream while hidden passes its last ID back here when it reopens.
			lastEventID = request.URL.Query().Get("last")
		}
		subscription, catchUp, err := handlers.core.Subscribe(viewerFrom(request.Context()), lastEventID)
		if err != nil {
			auth.WriteProblem(writer, http.StatusNotFound, core.ErrNoSuchCampaign.Message)
			return
		}
		defer subscription.Close()

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
		opening := fmt.Sprintf("retry: %d\n\n", streamRetryMilliseconds)
		if catchUp != nil {
			opening += formatStreamMessage(*catchUp)
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
			case <-request.Context().Done():
				return
			case <-subscription.Closed():
				return
			case <-rotation.C:
				return
			case <-heartbeat.C:
				if !send(": heartbeat\n\n") {
					return
				}
			case message := <-subscription.Messages():
				if !send(formatStreamMessage(message)) {
					return
				}
			}
		}
	})
}

func formatStreamMessage(message core.StreamMessage) string {
	// InvalidateMessage holds only topic names, so encoding cannot fail.
	data, _ := json.Marshal(message.Data)
	return fmt.Sprintf("id: %s\nevent: invalidate\ndata: %s\n\n", message.ID, data)
}
