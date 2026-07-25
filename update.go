package bunyan

import (
	"fmt"
	"time"
)

const (
	updateKindField  updateKind = iota
	updateKindStop   updateKind = 1
	updateKindStart  updateKind = 2
	updateKindDelete updateKind = 3
	updateKindWrite  updateKind = 4
)

// updateKind needs to be a type with a value representing null.
// If the update does not have anything to do start/stop operations.
type updateKind int

func (u updateKind) String() string {
	switch u {
	case updateKindField:
		return "UpdateKindField"
	case updateKindStop:
		return "UpdateKindStop"
	case updateKindStart:
		return "UpdateKindStart"
	default:
		return "unknown"
	}
}

type chainFields struct {
	comment string
	kind    updateKind
}

// SpanFields exposes non-internal fields that may be modified
type SpanFields struct {
	// [BUN-1] - category is a string that is closed to modification
	// after span creation.
	//
	// do not expose category to updating unless logic exists
	// to move spans from one category to another category on the chain
	//
	// category is duplicated from the chain onto the span so the span can
	// have its own context in isolation
	category string

	// spanID is bound
	spanID       SpanID
	ParentSpanID SpanID
	Comment      []string

	// kind determines the action of update for the span
	// This is a constrained set of options, hence the type as a hint.
	//
	// this is intentionally not exposed and is meant to be controlled via functions
	kind updateKind

	// effectiveTime is used for time start/stop
	effectiveTime time.Time
}

func (sf SpanFields) String() string {
	return fmt.Sprintf(
		"<SpanFields category:%s; spanID: %s; ParentSpanID: %s; Comment: %s; kind: %s; effectiveTime: %s>",
		sf.category,
		sf.spanID,
		sf.ParentSpanID,
		sf.Comment,
		sf.kind,
		sf.effectiveTime)
}

// Update presents a synchronous, procedural interface around underlying asynchronous updates that
// are performed with minimal locking.  This asynchronous behavior with an immediate return is for
// ease of code for the user, not for performance considerations.
//
// This is our entrypoint to updating spans across multiple fields.

//func (c *Chain) update(update SpanFields) {
//	// preserve a time closer to call time in case we have calls stacked up
//	update.effectiveTime = time.Now()
//	go func(up SpanFields) {
//		c.dataAccessChan <- up
//	}(update)
//}

// initializeSpan creates an initialized span, but does not update the chain
// this is required when code is executing as a result of lifetime handling
//
// a pointer is returned to prevent initialization of fields on return
func (c *chain) initializeSpan(category string, spanId SpanID, fields SpanFields) *span {
	s := &span{
		updateFn:     c.spanUpdateFn,
		parentSpanID: fields.ParentSpanID,
		spanID:       spanId,
		category:     category,
		// start lazily delays until the first tracking call
		// do not set this on instantiation.  this is not a bug.
		//
		// start, end are controlled by methods
		start:   time.Time{},
		end:     time.Time{},
		comment: fields.Comment,
	}

	return s
}

// NewSpan initializes a Span.  The pointer received can be considered to be a proxy for the span
// as fields are not updated on update (instead, the central registry tracks changes).
//
// Note that updateFn is called - this must not be called via updateFn
func (c *chain) NewSpan(category string, spanId SpanID, fields SpanFields) *span {
	// note: a span can be set to the parent span.  nothing prevents this.
	// consider requiring a unique span ID.  handle uniqueness by re-using our existing pattern
	// of serializing to a channel and checking a map (vs hoping a UUID is truly unique)
	s := c.initializeSpan(category, spanId, fields)
	s.updateFn(mergeSpanOntoFields(category, spanId, fields))
	return s
}
