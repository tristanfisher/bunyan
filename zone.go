package bunyan

import (
	"context"
	"sync"
)

// ZoneID is a wrapped string for future portability reasons
type ZoneID string

// zone is a container for a period of measurement, contrasted with manager.
type zone struct {
	managerCtx context.Context

	zoneCtx       context.Context
	zoneCtxCancel context.CancelFunc

	// Container has awareness of its own id for logging purposes
	id ZoneID
	// todo: multi-chain
	// pointer to chain for association / linking
	chain *chain

	errorChannel        chan error
	errorChannelHandler func(error)

	sync.Mutex
}

// SetID allows for overriding the initial spanID of a session
func (z *zone) SetID(id ZoneID) {
	z.Lock()
	defer z.Unlock()
	z.id = id
}

func (z *zone) GetID() ZoneID {
	z.Lock()
	defer z.Unlock()
	return z.id
}

// NewChain creates a new chain (domain of related spans)
//
// # Chain is bound to Zone, Zone is bound to Manager
//
// NewChain start a background workers to handle spans, returning synchronously
// for usage.  Chains can be added to Zones, Spans can be added to Zones
// at any point from this point forward.
func (z *zone) NewChain(ctx context.Context, id ChainID) chain {

	c := newChain(ctx, id)
	z.chain = c
	if z.chain == nil {
		panic("nil chain for zone")
	}

	if z.chain.chainCtx == nil || z.chain.cancelFn == nil {
		// library error
		// pull forward the panic instead of letting it occur at the time of context closure
		panic("nil chain context")
	}

	// both goroutines must control flow via z.chain.chainCtx

	// this goroutine collapses both z.chain.chainCtx and the user-provided
	// context into the z.chain.chainCtx lifetime
	go func(currentChain *chain, argCtx context.Context) {
		// to gracefully shut down, we must track either input context to chain or our
		// wrapped cancel-context, which was created to allow zone to Shutdown() explicitly
		//
		// we do not need to loop over cases as we ultimately control execution through currentChain.chainCtx
		select {
		case <-z.zoneCtx.Done():
			// manager->zone->chain->span
			//          ^^^^
			// zone has canceled context, so try to clean up
			currentChain.Close()

		// NewChain() provided "external" context
		case <-argCtx.Done():
			// close *only* this chain.  do not shut down the zone.
			// close via the context cancel mechanism for a unified workflow
			currentChain.Close()

		// our "internal" context was called directly
		case <-currentChain.chainCtx.Done():
			// no work required, clean up goroutine
			return
		}
	}(c, ctx)

	go func(currentChain *chain, chainCtx context.Context) {
		// for managed chains, we use the chain context.
		// on supporting multiple lifetime cycles, handleLifetime() should
		// support a separate context from chain context
		//
		// handleLifetime blocks
		//
		// this *must* be run only once
		err := currentChain.handleLifetime(chainCtx)
		if err != nil {
			z.errorChannel <- err
		}
		// Removed: currentChain.isClosed.Store(true) as handleLifetime already handles this
	}(c, z.chain.chainCtx) // pick our cancellable context for management

	// wait for the chain to be ready via handleLifetime before returning
	<-c.readyChan

	return *z.chain
}

// Close irreversibly terminates a zone, as well as all underlying chains
func (z *zone) Close() error {
	// tell the chain to close, which will shut down the spans
	z.chain.Close()

	// cancel the zone context
	z.zoneCtxCancel()

	// todo: intentionally block on chain shutdown
	// check each all chain.IsClosed()
	// this is in place for when we have multiple chains in a zone
	return nil
}

func (z *zone) Report() ZoneReport {
	return ZoneReport{}
}
