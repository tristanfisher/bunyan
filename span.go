package bunyan

import (
	"fmt"
	"time"
)

// SpanID is a wrapped string for future portability reasons
type SpanID string

func (s SpanID) String() string {
	return string(s)
}

// span tracks an individual unit of work's start/end time
// A parentID may be provided to track a call structure
type span struct {
	// updateFn is a reference back to the relevant chain for the span
	// it is required as updates to Spans are not performed directly,
	// instead they flow through the chain.
	//
	// see c.updateSpan() and handleLifetime
	updateFn func(SpanFields)

	// spanID is meant for the specific call, e.g. "creditAccount"
	// spanID must not be modified after creation.
	spanID SpanID

	// parentSpanID references a parent entry ID for this span
	// the parentSpanID does not need to exist in any chain.  This is intentional to support cases
	// such as distributed traces, where the ultimate parent span is external to the process.
	parentSpanID SpanID
	// category is useful for groupings, like "billing"
	category string
	start    time.Time
	end      time.Time
	// Comment is useful for additional context for logs or developers
	comment []string

	// see spanAnalysis for reporting/derived values
	// span is kept as minimal as possible for sake of performance, including empty fields
}

func mergeSpanOntoFields(category string, spanID SpanID, u SpanFields) SpanFields {
	if len(u.category) > 0 {
		category = u.category
	}
	if len(u.spanID) > 0 {
		spanID = u.spanID
	}

	return SpanFields{
		category:      category,
		spanID:        spanID,
		ParentSpanID:  u.ParentSpanID,
		Comment:       u.Comment,
		kind:          u.kind,
		effectiveTime: u.effectiveTime,
	}
}

func mergeMutableSpanFields(s *span, u SpanFields) *span {
	// library error
	if s.updateFn == nil {
		panic("span updateFn must be set, returned merged span would result in panic")
	}

	// implicit UpdateKindStart
	if u.kind == updateKindStart {
		s.start = u.effectiveTime
	}
	if u.kind == updateKindStop {
		s.end = u.effectiveTime
	}

	if len(u.Comment) > 0 {
		s.comment = append(s.comment, u.Comment...)
	}

	if len(u.ParentSpanID) > 0 {
		s.parentSpanID = u.ParentSpanID
	}

	return s
}

func (s span) Update(update SpanFields) {
	// spanID is not available to the caller, but it's necessary
	// for the update function to target the correct span
	update.spanID = s.spanID
	// category necessary for routing the update to the correct entry
	update.category = s.category

	// preserve a time closer to call time in case we have calls stacked up
	update.effectiveTime = time.Now()

	// note that spanID is on Span, not necessarily update
	go func(up SpanFields) {
		// SpanFields is a delta; not all the fields on Span are on SpanFields.  updateFn == *chain.updateSpan
		s.updateFn(up)
	}(update)
}

// Note on updates: be sure to set `category: s.category` or otherwise the update will not dispatch correctly

// todo: GetSpanID
// todo: GetParentID
// these two must run through lifecycle to be safe

func (s span) String() string {
	return fmt.Sprintf("<SpanID: %s; ParentSpanID: %s; Category: %s; Start: %s; End: %s; Comment: %s>",
		s.spanID, s.parentSpanID, s.category, s.start, s.end, s.comment)
}

func (s span) SetParentID(parentID SpanID) {
	s.Update(SpanFields{
		category:     s.category,
		ParentSpanID: parentID,
	})
}

func (s span) Start() {
	s.StartAt(time.Now())
}

func (s span) StartAt(t time.Time) {
	s.Update(SpanFields{
		category:      s.category,
		kind:          updateKindStart,
		effectiveTime: t,
	})
}

func (s span) End() {
	s.EndAt(time.Now())
}

func (s span) EndAt(t time.Time) {
	s.Update(SpanFields{
		category:      s.category,
		kind:          updateKindStop,
		effectiveTime: t,
	})
}

func (s span) AddComment(comment string) {
	s.Update(SpanFields{
		category: s.category,
		Comment:  []string{comment},
	})
}

// note: flame calculations are performed separately and are not attached as method
// 		 receivers to not waste multiple bounds checks

// in order to get our span fields, we need to consult our chain
//
// updates are not performed on individual span structs as it could be desynced
// from when chain updates are performed.
//
// todo: getter function for a specific span that copies out of the chain
