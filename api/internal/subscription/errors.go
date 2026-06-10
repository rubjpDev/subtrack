package subscription

import (
	"errors"
	"strings"
)

// ErrNotFound is returned by the service (and expected from the repository)
// when a subscription does not exist. Handlers map it to HTTP 404.
var ErrNotFound = errors.New("subscription: not found")

// ValidationError carries one or more per-field validation failures so the
// HTTP edge can render a structured 400 response with field-level detail.
type ValidationError struct {
	// Fields maps a field name to a human-readable validation message.
	Fields map[string]string
}

// Error satisfies the error interface with a concise summary; callers that
// need per-field detail should use Fields directly (e.g. the HTTP handler).
func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "subscription: validation failed"
	}

	parts := make([]string, 0, len(e.Fields))
	for field, msg := range e.Fields {
		parts = append(parts, field+": "+msg)
	}
	return "subscription: validation failed: " + strings.Join(parts, "; ")
}

// addField records a validation failure for the given field. It is a small
// helper used while building up a ValidationError across several checks.
func (e *ValidationError) addField(field, msg string) {
	if e.Fields == nil {
		e.Fields = make(map[string]string)
	}
	e.Fields[field] = msg
}

// hasErrors reports whether any field failures have been recorded.
func (e *ValidationError) hasErrors() bool {
	return len(e.Fields) > 0
}
