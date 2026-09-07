package bunyan

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// getCopy copies the table and entries
func (c *chain) getCopy() *chain {

	// a new background context is created as the copy is not related to the
	// operational chain (no connection to the manager exists - (manager->zone->chain->....))
	ctx2Bg := context.Background()
	ctx2, ctx2Cancel := context.WithCancel(ctx2Bg)

	// make a copy under lock as we're copying fields on Chain
	//c.Lock() // todo: no need? all operations happen in lifetime
	ch := newChain(ctx2, c.id)
	// copy() is expected to be the bulk of operational time
	catPtr := c.categories.copy()
	ch.categories = *catPtr

	// unlock as soon as possible
	//c.Unlock() // todo: no need?

	ch.chainCtx = ctx2
	ch.cancelFn = ctx2Cancel
	// by definition, copies are default closed
	// as they do not have a handleLifetime() call attached (pre-ready will never move this copy to a ready state!).
	// if you need the functionality to work off a "fork" of *chain,
	// open a PR with different semantics for chain vs. fork.
	ch.isClosed.Store(true)
	return ch
}

func (c *chain) handleLifetime(ctx context.Context) error {

	// if channels are sent messages before handleLifetime is ready, we can have unexpected
	// behaviors such as select cases hitting their defaults or deadlocks on less guarded code
	//
	// this happens if ctx.Done() is read before any calls
	markInitialized := make(chan struct{})
	go func(ctx2 context.Context) {
		select {
		// context closed before initialization could fire
		case <-ctx2.Done():
			// wake up any pending initialization, which should be smart enough
			// to check closed status.
			// potentially redundant storage of closure to avoid fringe cases
			// where this goroutine catches the context closure before our main loop
			// and also wakes up all waiting routines with a Broadcast
			c.isClosed.Store(true)
			c.preReadyWait.Broadcast()
			return
		case markInitialized <- struct{}{}:
		}
	}(ctx)

	// this serialized handling eases flow and reduces the need for locking
	// while a non-shutdown case is being handled, we're thread safe for modifications.
	// the shutdown/context-closure case will return with messages left in channels

	for {
		select {

		case <-markInitialized:
			c.isReady.Store(true)
			// wake all goroutines waiting on ready.  calls will serialize to this function
			c.preReadyWait.Broadcast()
			close(c.readyChan) // Signal that the chain is ready

		case <-ctx.Done():
			c.isClosed.Store(true)
			c.preReadyWait.Broadcast()

			dacLen := len(c.dataAccessChan)
			if dacLen > 0 {
				return fmt.Errorf("chain closed with message count in queue: %d", dacLen)
			}

			if len(c.copyComplete) > 0 {
				return errors.New("chain closed with a pending copy operation")
			}

			return nil

		// if entries are modified while a copy is in operation, we break thread safety.
		// this *must* be the only place where we write to our underlying map(s) as concurrent map writes will cause a panic.
		//
		// note that we expect callers to get to this channel via updateSpan()
		case ent := <-c.dataAccessChan:

			switch ent.Type {
			case damRequestCopy:
				// damRequestCopy behavior is simply routing based on the status of ent channels

				// if we have a target channel on the entry, use it.
				// see below for default if not provided
				if ent.ChainChan != nil {
					select {
					case ent.ChainChan <- c.getCopy():
						if c.chainCtx.Err() != nil {
							return errors.New("chain closed with a pending copy operation")
						}

						// todo: handle case of ent.ChainChan being closed
					}
					continue // don't fall through to default
				}

				// else, send to a default copy channel, which exists
				// to support functionality of regular polling (todo)
				select {
				case c.copyComplete <- c.getCopy():
					if c.chainCtx.Err() != nil {
						return errors.New("chain closed with a pending copy operation")
					}
					// success
				default:
					//expected to be closed
				}

			case chainWrite:
				if ent.chainFields.kind == updateKindDelete {
					// removes all instances of target comment
					*c.categories.comment = slices.DeleteFunc(*c.categories.comment, func(c string) bool {
						return c == ent.chainFields.comment
					})
					continue
				}

				// otherwise assume updateKindWrite
				*c.categories.comment = append(*c.categories.comment, ent.chainFields.comment)

			case damWrite:
				// todo: add a not-after. has the message expired? if so, just drop it
				//
				// todo: consider intelligent pre-chewing for a stop or edit
				//       operation that will error if addressing an unknown key.
				//       this would prevent spans that have ends, but no starts
				//
				c.setEntry(ent.SpanFields)
			}

		}

	}
}
