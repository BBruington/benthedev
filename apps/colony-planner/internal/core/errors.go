package core

import (
	"errors"
	"fmt"
)

// ErrorKind is the closed set of failures an operation can report. The transport maps each kind to
// one status code (internal/app/errors.go); anything else is an internal error.
type ErrorKind int

const (
	KindInvalid ErrorKind = iota + 1
	KindForbidden
	KindNotFound
	KindConflict
	KindNotImplemented
	// KindSessionEnded means the caller's session no longer leads to a member: sign in again.
	KindSessionEnded
)

type FieldError struct {
	// Field is the JSON name of the input field, e.g. "characterName".
	Field   string
	Message string
}

type Error struct {
	Kind    ErrorKind
	Message string
	Fields  []FieldError
}

func (operationError *Error) Error() string {
	return operationError.Message
}

func Invalid(message string, fields ...FieldError) error {
	return &Error{Kind: KindInvalid, Message: message, Fields: fields}
}

func Forbidden(message string) error {
	return &Error{Kind: KindForbidden, Message: message}
}

func NotFound(message string) error {
	return &Error{Kind: KindNotFound, Message: message}
}

func Conflict(message string) error {
	return &Error{Kind: KindConflict, Message: message}
}

// ErrSessionEnded answers a caller whose session or member row is gone.
var ErrSessionEnded = &Error{Kind: KindSessionEnded, Message: "Your session has ended. Sign in again."}

// NotImplemented marks a contract stub whose operation a later builder fills in.
func NotImplemented(operation string) error {
	return &Error{Kind: KindNotImplemented, Message: fmt.Sprintf("%s is not implemented yet", operation)}
}

// AsError reports whether err carries a typed operation error.
func AsError(err error) (*Error, bool) {
	var operationError *Error
	if errors.As(err, &operationError) {
		return operationError, true
	}
	return nil, false
}
