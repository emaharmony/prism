// Package projection provides state projections over Prism event streams.
package projection

// V10 Projection Event Types
// These events are emitted when projections are computed.
const (
	EventTypeProjectionStarted   = "prism.projection.started"
	EventTypeProjectionCompleted = "prism.projection.completed"
	EventTypeProjectionFailed    = "prism.projection.failed"
)
