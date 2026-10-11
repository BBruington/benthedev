package game

import (
	"strconv"
	"strings"
)

// The canonical texts the rules return. Handlers pass them through unchanged.
const (
	MessageTooManyDays      = "That many days can't be stored"
	MessageAdvanceAtLeast   = "Advance at least 1 day"
	MessageIntervalTooSmall = "The shipment interval must be at least 1 day"
	MessageNegativeDay      = "The day can't be negative"
	MessageNegativeColonist = "Colonists can't be negative"
	MessageTooManyColonists = "That many colonists can't be stored"
)

// InvalidError is input a rule refuses. Core maps it to core.Invalid (422), using Field as the
// field error's JSON name when it is set.
type InvalidError struct {
	Field   string
	Message string
}

func (invalidError *InvalidError) Error() string {
	return invalidError.Message
}

func invalid(field string, message string) error {
	return &InvalidError{Field: field, Message: message}
}

// Shortfall is one thing a STRICT spend could not pay for (spec §5.4).
type Shortfall struct {
	// ResourceKey is the resource that falls short, or "" when the shortfall is favor.
	ResourceKey string
	// Holder names who is short: the patron for favor, "the colony" for a resource.
	Holder string
	// What is the lowercased thing that is short: "favor", or the resource's label.
	What  string
	Has   int64
	Needs int64
}

// Message renders the canonical text, e.g. "Not enough favor: Lady Vex has 2, needs 3".
func (shortfall Shortfall) Message() string {
	var builder strings.Builder
	builder.WriteString("Not enough ")
	builder.WriteString(shortfall.What)
	builder.WriteString(": ")
	builder.WriteString(shortfall.Holder)
	builder.WriteString(" has ")
	builder.WriteString(strconv.FormatInt(shortfall.Has, 10))
	builder.WriteString(", needs ")
	builder.WriteString(strconv.FormatInt(shortfall.Needs, 10))
	return builder.String()
}

// ShortfallError rejects a whole STRICT operation and names every shortfall, favor first and then
// resources by key. Core maps it to 422 when a request is made and to 409 when it is approved.
type ShortfallError struct {
	Shortfalls []Shortfall
}

func (shortfallError *ShortfallError) Error() string {
	messages := make([]string, 0, len(shortfallError.Shortfalls))
	for _, shortfall := range shortfallError.Shortfalls {
		messages = append(messages, shortfall.Message())
	}
	return strings.Join(messages, ". ")
}
