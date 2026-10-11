package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/BBruington/colony-planner/internal/core"
)

// fail maps core's typed errors to HTTP statuses. Anything untyped is a bug or an outage: it is
// logged and the client sees a generic 500.
func (handlers *handlers) fail(requestContext context.Context, err error) error {
	operationError, ok := core.AsError(err)
	if !ok {
		handlers.logger.ErrorContext(requestContext, "operation failed", slog.Any("error", err))
		return huma.Error500InternalServerError("Something went wrong.")
	}
	switch operationError.Kind {
	case core.KindInvalid:
		details := make([]error, 0, len(operationError.Fields))
		for _, field := range operationError.Fields {
			details = append(details, &huma.ErrorDetail{Location: "body." + field.Field, Message: field.Message})
		}
		return huma.Error422UnprocessableEntity(operationError.Message, details...)
	case core.KindForbidden:
		return huma.Error403Forbidden(operationError.Message)
	case core.KindNotFound:
		return huma.Error404NotFound(operationError.Message)
	case core.KindConflict:
		return huma.Error409Conflict(operationError.Message)
	case core.KindNotImplemented:
		return huma.NewError(http.StatusNotImplemented, operationError.Message)
	case core.KindSessionEnded:
		return huma.Error401Unauthorized(operationError.Message)
	default:
		handlers.logger.ErrorContext(requestContext, "operation failed with an unknown error kind", slog.Any("error", err))
		return huma.Error500InternalServerError("Something went wrong.")
	}
}
