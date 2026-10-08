package playbackv1

import "errors"

// FieldError names the request field that was refused (the API error's path).
type FieldError struct{ Path string }

func (e *FieldError) Error() string { return "invalid field " + e.Path }

// Domain errors. The HTTP layer maps each to a registered API error code.
var (
	ErrNotFound            = errors.New("not found")
	ErrEnded               = errors.New("session ended")
	ErrRevision            = errors.New("revision mismatch")
	ErrRevisionRequired    = errors.New("revision required")
	ErrIdempotencyMismatch = errors.New("idempotency key reused with a different request")
	ErrKeyRequired         = errors.New("idempotency key required")
	ErrQueueEnded          = errors.New("queue ended")
	ErrUnsupportedSelector = errors.New("unsupported selector")
	// ErrQueueTooLarge is a selection past a segment's or a queue's key bound.
	ErrQueueTooLarge   = errors.New("queue too large")
	ErrNotControllable = errors.New("device not controllable")
	ErrStreamExists    = errors.New("stream exists")
)
