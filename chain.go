package bunyan

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ChainID is a wrapped string for future portability reasons
type ChainID string

type dataAccessMessageType string

const damRequestCopy dataAccessMessageType = "copy"
const damWrite dataAccessMessageType = "write"
const chainRequestCopy dataAccessMessageType = "chainCopy"
const chainWrite dataAccessMessageType = "chainWrite"

type dataAccessMessage struct {
	Type dataAccessMessageType
	// SpanFields are optional, depending on Type
	SpanFields SpanFields
	// ChainChan is optional, depending on Type
	// todo: require buffered chan to handle the chan being closed?
	ChainChan chan *chain

	// chainFields are optional, depending on Type
	chainFields chainFields
}

func (dam dataAccessMessage) String() string {
	return fmt.Sprintf("<DataAccessMessage Type:%s; Fields: %s>", dam.Type, dam.SpanFields)
}

// chain provides complex span management
// This is useful for tracking all spans for a complex function or chain of functions.
type chain struct {

	//
	// data
	//
	id ChainID
	// root span is useful as a simple span or wrapper around
	// all sub-spans

	// Categories / spans
	// also stores comments for a given chain
	categories category

	//
	// internals
	//

	// dataAccessChan accepts messages for updating spans
	// or access fields inside our maps
	dataAccessChan chan dataAccessMessage
	// requestCopy allows for thread-safe generation of span objects
	//requestCopy chan struct{}

	// copyComplete is used to pass back a copied chain as requested by a message on requestCopy
	copyComplete chan *chain

	// spanUpdateFn is bound to chain for "safe" passing to spans
	// this may be aliased to updateFn
	spanUpdateFn func(SpanFields)

	//
	// cancellation context
	// root -> zone -> chain -> span
	//                 ^^^^^
	chainCtx context.Context
	cancelFn context.CancelFunc

	// life management
	// isReady: denotes if the chain has been initialized
	// preReadyWait: allows for delaying or not sending to channels if !isReady
	// isClosed: denotes if the chain is closed for writing
	//
	// it is possible that isClosed will be set before isReady -- if a context
	// or closure happens before initialization, the chain can be closed without ever being ready

	// isReady is intended to denote if channels will be consumed by receivers.
	// this prevents sending to closed channels, which would deadlock and/or panic if goroutines
	// are sleeping or GC'd.
	//
	// this is the fast path of readiness checking
	isReady *atomic.Bool
	// preReadyWait allows senders to wait on readiness without spin locking, polling, or
	// doing a default case re-queue strategy in a select.
	// logically, isReady should be checked before decaying to checking this conditional variable.
	//
	// a condition variable is required because any number of goroutines could have attempted sending to channels
	preReadyWait *sync.Cond
	// required for condition variable semantics
	preReadyWaitMu *sync.Mutex

	// isClosed denotes if the chain is closed for writing.  messages should not be
	// sent to channels if the chain is closed.
	//
	// once closed, a chain cannot be re-opened.
	isClosed *atomic.Bool

	// readyChan is used to signal when the chain has been initialized and is ready for use.
	readyChan chan struct{}

	// pointer for sake of copy operations
	sync.Mutex
}

//func (c *Chain) getEntry(category string, spanID SpanID) (Span, bool) {
//	cat, ok := c.categories.table[category]
//	if !ok {
//		return Span{}, false
//	}
//	e, ok := cat[spanID]
//	return e, ok
//}

// setEntry sets fields on an entry, creating a category if necessary.
// note that this is not thread safe and expects either locking or sequential writing
func (c *chain) setEntry(entry SpanFields) {

	// check if the category already exists
	// we *must* not write to this hashmap anywhere else
	//
	// uninitialized c.table will throw a panic
	// save a check vs paying for the lookup every time
	cat, categoryExists := c.categories.table[entry.category]
	if !categoryExists {
		// no existing category.  create container for an entry

		// note: an empty string is a valid map key
		c.categories.table[entry.category] = make(entries)

		// if we did not have a matching entry category,
		// it is not possible that the entry already exists.
		newSpanEntry := c.initializeSpan(entry.category, entry.spanID, entry)

		// now apply the entry on the new span.  this could be a start, out order stop, comment, etc.
		// note: this can probably be removed with initializeSpan handling start
		newSpanEntry = mergeMutableSpanFields(newSpanEntry, entry)

		// library error
		if newSpanEntry.updateFn == nil {
			panic("span updateFn must not be nil")
		}

		c.categories.table[entry.category][entry.spanID] = newSpanEntry
		return
	}

	// category exists, does our entry?
	// we *must* not write to this hashmap anywhere else
	//
	// cat is scoped to a category map
	_, ok := cat[entry.spanID]
	if !ok {
		newSpanEntry := c.initializeSpan(entry.category, entry.spanID, entry)
		newSpanEntry = mergeMutableSpanFields(newSpanEntry, entry)
		// library error
		if newSpanEntry.updateFn == nil {
			panic("span updateFn must not be nil")
		}
		cat[entry.spanID] = newSpanEntry
		return
	}

	cat[entry.spanID] = mergeMutableSpanFields(cat[entry.spanID], entry)
}

// newChain creates a new chain with a cancellable context
// the context must be wrapped inside of newChain() and bound as the input ctx
// may not have been cancellable.  we use context to handle graceful closing.
// we do this instead of attaching a function and an atomic or an additional close channel.
func newChain(ctx context.Context, id ChainID) *chain {

	// provided context is wrapped to provide graceful shutdown
	wrappedCtx, wrappedCtxCancel := context.WithCancel(ctx)
	simpleMu := &sync.Mutex{}
	c := &chain{
		id:             id,
		categories:     newEmptyCategory(),
		dataAccessChan: make(chan dataAccessMessage),
		copyComplete:   make(chan *chain),

		// assigned after Chain{} is addressable
		spanUpdateFn: nil,

		chainCtx:       wrappedCtx,
		cancelFn:       wrappedCtxCancel,
		isReady:        &atomic.Bool{},
		preReadyWait:   sync.NewCond(simpleMu),
		preReadyWaitMu: simpleMu,
		isClosed:       &atomic.Bool{},
		readyChan:      make(chan struct{}),
		Mutex:          sync.Mutex{},
	}

	c.spanUpdateFn = c.updateSpan

	return c
}

// TimingCopy returns a copy of our timings, which is useful for requesting mid-operation status.
// TimingCopy is responsible for safe operation around chain initialization, locking, etc.
func (c *chain) TimingCopy() (*chain, error) {

	// readiness checking
	// fast path: check atomic load, which ideally just bounces off hardware
	//            instead of waiting on the scheduler
	if !c.isReady.Load() {
		// slow path: take lock, give it back and tell the scheduler to sleep us until
		// 			  a signal or broadcast on the condition variable is sent
		c.preReadyWaitMu.Lock()
		// tf: I'm suspicious that there's still a slight chance of goroutines deadlocking
		// 	   even with a guarded wait
		for !c.isReady.Load() && !c.isClosed.Load() {
			c.preReadyWait.Wait()
		}
		c.preReadyWaitMu.Unlock()
	}

	// chain closed, potentially even closed while waiting for initialization
	// if our chain is closed, we bypass sending to a channel as otherwise we'll deadlock
	var chainCopy *chain
	if c.isClosed.Load() || c.chainCtx.Err() != nil {
		// this *must* match handleLifetime, DAMRequestCopy
		chainCopy = c.getCopy()
	} else {
		// channel not closed,
		// create a channel for our response to be placed onto
		// this is required to have a response correspond to a request
		chainCopyChan := make(chan *chain)

		// message onto a channel to request a copy
		// this will block until picked up.  select{} guarded in case context expires or Chain's channel explicitly closed
		// checking c.IsClosed() is unsafe as we could be closed at time of send
		//
		// todo: add a default case that sends to an overflow queue
		//       with a matching return channel to wait on for the caller
		//       this would best be matched with a max duration wait in our select{} ({ case request copy; case } )
		select {
		case <-c.chainCtx.Done():
			return &chain{}, errors.New("chain context canceled")
		// we provide a chainCopyChan as a target to read (vs. c.copyComplete)
		case c.dataAccessChan <- dataAccessMessage{Type: damRequestCopy, ChainChan: chainCopyChan}:
			select {
			// read from channel, which will return when a copy is ready
			case chainCopy = <-chainCopyChan:
				return chainCopy, nil
			case <-time.After(30 * time.Second):
				// we should never take this long - raise an issue if resource constraints result in this much of a delay
				return &chain{}, errors.New("timeout waiting for chain copy")
			case <-c.chainCtx.Done():
				return &chain{}, errors.New("chain context canceled while waiting for copy")
			}

		}
	}

	return chainCopy, nil
}

func newSpanFlame(span *span, totalDuration time.Duration, relativeStartTime time.Time) spanAnalysis {
	sA := spanAnalysis{
		Span:     span,
		Duration: time.Duration(0),
		FlameCalculations: struct {
			Offset time.Duration
			Width  time.Duration
			Depth  int
		}{},
	}

	// duration of this span
	thisSpanDur := sA.Span.end.Sub(sA.Span.start)

	off := sA.Span.start.Sub(relativeStartTime)

	if totalDuration == 0 {
		sA.Duration = 0
		sA.FlameCalculations.Offset = 0
	} else {
		sA.Duration = (thisSpanDur / totalDuration) * 100
		sA.FlameCalculations.Offset = (off / totalDuration) * 100
	}

	return sA
}

// TODO: status() should process an internal state for reporting or writing to structured log
func (c *chain) status() (*ChainReport, error) {

	// get a copy of our chain for potential modification and lock-free/no-concurrency-concern operations
	chainCopy, err := c.TimingCopy()
	if err != nil {
		return &ChainReport{}, err
	}

	// "infinite start time" to prevent only triggering an early date if start is before epoch start
	// and to avoid having to check an .IsZero() on every iteration
	//
	// 1<<62 is used as a crude, safe max approximation.  2^63 - 1 would overflow when
	// the time package converts to year 1.  go operates on nanoseconds, so the real value is something
	// approximating 1<<63- 1 minute (to nano) + 9999.... nanosec
	//
	// start/end is interesting on a statistical level and required to create a flamegraph
	totalStart := time.Unix(1<<62, 0)
	totalEnd := time.Time{}
	// Ordered from longest to shortest
	var spanAnalysisTimings []spanAnalysis

	// save allocation per iteration
	dummyInsert := spanAnalysis{}

	categories := make([]string, len(chainCopy.categories.table))

	// keep all expensive processing ien this loop
	categoryNames := make([]string, len(chainCopy.categories.table))
	var warnings []string
	i := 0

	// nest spans inside each other based on timing
	// mimic a flamegraph

	for categoryName, categorySpans := range chainCopy.categories.table {
		categories[i] = categoryName

		for spanName, spanValue := range categorySpans {
			// time settings
			spanTimeDisqualified := false

			if spanValue.start.IsZero() && spanValue.end.IsZero() {
				warnings = append(warnings, fmt.Sprintf("%s missing start or end time.  unix time start: %d end: %d", spanName, spanValue.start.Unix(), spanValue.end.Unix()))
				spanTimeDisqualified = true
			}

			if !spanTimeDisqualified {
				spanDuration := spanValue.end.Sub(spanValue.start)

				// add to ordered array of long to short spans (high to low)
				idx := sort.Search(len(spanAnalysisTimings), func(i int) bool {
					return spanAnalysisTimings[i].Duration <= spanAnalysisTimings[i].Duration
				})

				// insert dummy value to grow slice
				spanAnalysisTimings = append(spanAnalysisTimings, dummyInsert)

				// shift left, right around idx for insertion
				copy(spanAnalysisTimings[idx+1:], spanAnalysisTimings[idx:])

				spanAnalysisTimings[idx] = spanAnalysis{
					Span:     spanValue,
					Duration: spanDuration,
				}

				// calculate start and end of chain based on spans
				if spanValue.start.Before(totalStart) {
					totalStart = spanValue.start
				}

				if spanValue.end.After(totalEnd) {
					totalEnd = spanValue.end
				}

			} else {
				// simply append the span without proper start/stop to the end of span timings (ordered high to low)
				spanAnalysisTimings = append(spanAnalysisTimings, spanAnalysis{
					Span:     spanValue,
					Duration: -1, // -1 to denote invalid start/end
				})
			}

		}
		categoryNames[i] = categoryName
		i++
	}

	// sort category names lexicographically
	slices.Sort(categoryNames)

	// if no spans were recorded, pay for initialization as otherwise we can return garbage or negative max
	if totalEnd.IsZero() || totalStart.Equal(time.Unix(1<<62, 0)) {
		totalStart = time.Time{}
		totalEnd = time.Time{}
	}

	// span calculation
	// totalDuration gets us our width
	totalDuration := totalEnd.Sub(totalStart)

	// spanTimings is longest to shortest span durations for listing, which we want to preserve/not resort.
	// make a copy of spanTimings for spans with valid starts and ends for our flame graph
	adjacencyList := make(map[SpanID][]spanAnalysis)
	allFlameIDs := make(map[SpanID]struct{})

	// iterative approach of building hierarchal structure instead of recursion to try to avoid a stack overflow
	// spanAnalysisTimings have offsets and duration
	for _, s := range spanAnalysisTimings {
		if s.Duration < 0 {
			// warning already expected via earlier loop over spans.
			// we don't want to build a hierarchy with invalid times
			continue
		}

		// todo: we must do this to get values for drawing our flame graph
		newSpanFlame(s.Span, totalDuration, totalStart)

		// no parent span will have a "" value
		adjacencyList[s.Span.parentSpanID] = append(adjacencyList[s.Span.parentSpanID], s)
		// note spanID and not parentSpanID
		allFlameIDs[s.Span.spanID] = struct{}{}
	}

	// identify root spans
	var rootSpans []spanAnalysis
	for _, s := range spanAnalysisTimings {
		_, ok := allFlameIDs[s.Span.parentSpanID]
		if !ok {
			rootSpans = append(rootSpans, s)
		}
	}

	// sort by start time for alignment
	// then sort spans by start time for alignment
	sort.Slice(rootSpans, func(i, j int) bool {
		// if there's no duration, sort to start.  this is expected to be filtered out, but defensively as zero
		// or negative width display can result in a strange display
		if rootSpans[i].Duration < 0 {
			return true
		}
		return rootSpans[i].Span.start.Before(rootSpans[j].Span.start)
	})
	// assign to nil values to let the GC clean up ASAP
	allFlameIDs = nil
	adjacencyList = nil

	// iterate using a stack instead of recursion
	//
	// todo: we can probably iterate directly inside the sort as this is just a reversal
	stack := make([]spanAnalysis, 0)
	for i := len(rootSpans) - 1; i >= 0; i-- {
		stack = append(stack, rootSpans[i])
	}

	var flameBuf strings.Builder
	flameBuf.WriteString(fmt.Sprintf("%-20s | %-6s | %-10s | %-10s\n", "Span ID", "Depth", "Offset %", "Width %"))
	flameBuf.WriteString("---------------------------------------------------------------\n")

	for len(stack) > 0 {
		// pop off our stack
		curr := stack[len(stack)-1]
		// remove top element in preparation of next loop
		stack = stack[:len(stack)-1]

		var widthPercent, offsetPercent float64
		if totalDuration > 0 {
			widthPercent = (curr.Duration / totalDuration).Seconds() * 100
			offsetPercent = (curr.FlameCalculations.Offset / totalDuration).Seconds() * 100
		}

		flameBuf.WriteString(fmt.Sprintf("%-20s | %-6d | %-10.2f | %-10.2f\n",
			curr.Span.spanID,
			curr.FlameCalculations.Depth,
			offsetPercent,
			widthPercent))

		// sort children
		children := adjacencyList[curr.Span.spanID]
		sort.Slice(children, func(i, j int) bool {
			return children[i].Span.start.Before(children[j].Span.start)
		})

		for i := len(children) - 1; i >= 0; i-- {
			children[i].FlameCalculations.Depth = curr.FlameCalculations.Depth + 1
			stack = append(stack, children[i])
		}
	}

	report := &ChainReport{
		ID:            chainCopy.id,
		TotalDuration: totalDuration,
		Start:         totalStart,
		End:           totalEnd,
		Categories: CategoriesReport{
			CategoryTable: chainCopy.categories.table,
			Comment:       *chainCopy.categories.comment,
		},
		CategoryNames: categoryNames,
		LongestSpan:   spanAnalysis{},
		SpanTimings:   spanAnalysisTimings,
		Warnings:      warnings,
		FlameText:     flameBuf.String(),
	}

	return report, nil
}

// Report flattens out the internal shape.
// This may be called at any point.
//
// This is a minimal set of fields
// and all derived information (e.g. timelines)
// should be constructed outside the lifecycle action.
func (c *chain) Report() (*ChainReport, error) {
	report, err := c.status()
	if err != nil {
		return &ChainReport{}, err
	}
	return report, nil
}

// Close irreversibly terminates a chain
// This can be called explicitly by our zone or user at any time.
func (c *chain) Close() {
	c.cancelFn()
}

func (c *chain) IsClosed() bool {
	return c.isClosed.Load()
}

// updateSpan is our entrypoint to updating our internal state.
// this may be aliased to spanUpdateFn and called via updateFn and handleLifetime
func (c *chain) updateSpan(update SpanFields) {

	// todo: handle c.isReady and check context

	// intentionally block on sending.
	// calls must be serialized to prevent concurrent map writes
	c.dataAccessChan <- dataAccessMessage{Type: damWrite, SpanFields: update}
}

func (c *chain) updateCategory(update chainFields) {

	// todo: handle c.isReady and check context

	// intentionally block on sending.
	// calls must be serialized to prevent concurrent writes
	c.dataAccessChan <- dataAccessMessage{Type: chainWrite, chainFields: update}
}

func (c *chain) AddComment(comment string) {
	c.updateCategory(chainFields{
		comment: comment,
		kind:    updateKindWrite,
	})
}

func (c *chain) RemoveComment(comment string) {
	c.updateCategory(chainFields{
		comment: comment,
		kind:    updateKindDelete,
	})
}
