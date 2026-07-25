package bunyan

import (
	"context"
	"errors"
	"sync"
)

// ManagerID is a wrapped string for future portability reasons
type ManagerID string

// manager provides functionality for long-term / multiple zone management.
// This is particularly useful for processes that handle requests, like networked servers.
// In such usages, each request zone has resources to track, which can be rolled up to a more
// complete view (in a manager).
type manager struct {
	// ctx is the root context propagated to all zones
	ctx context.Context

	// id is stores with manager as multiple may be running within a given process
	id ManagerID

	// pointer to zone for association / linking
	zone *map[ZoneID]*zone

	// errorChannel is used for runtime errors in goroutines
	errorChannel        chan error
	errorChannelHandler func(error)

	sync.Mutex
}

func NewManager(ctx context.Context, id ManagerID, errorChannelHandler func(error)) manager {
	m := manager{
		ctx:                 ctx,
		id:                  id,
		zone:                new(map[ZoneID]*zone),
		errorChannel:        make(chan error, 25),
		errorChannelHandler: errorChannelHandler,
		Mutex:               sync.Mutex{},
	}

	mZ := make(map[ZoneID]*zone)
	m.zone = &mZ

	if errorChannelHandler != nil {
		go func(c context.Context) {
			for {
				select {
				case <-c.Done():
					return
				case e := <-m.errorChannel:
					errorChannelHandler(e)
				}
			}
		}(ctx)
	} else {
		go func(c context.Context) {
			// user implicitly requests discard.  perform check once upfront
			// instead of paying for an additional boolean comparison on each potential error
			// either on read (this function) or send
			for {
				select {
				case <-c.Done():
					return
				case <-m.errorChannel:
					// discard
				}
			}
		}(ctx)
	}

	return m
}

func (m *manager) GetID() ManagerID {
	m.Lock()
	defer m.Unlock()
	return m.id
}

// SetID generates a UUID formatted random string
func (m *manager) SetID(id ManagerID) {
	m.Lock()
	defer m.Unlock()
	m.id = id
}

// NewZone initializes a new zone and associates it with a manager
// No goroutines or work is put in the background.  This is simply initialization.
func (m *manager) NewZone(ctx context.Context, zoneID ZoneID) (*zone, error) {
	m.Lock()
	defer m.Unlock()

	if _, ok := (*m.zone)[zoneID]; ok {
		return &zone{}, errors.New("zone id already exists")
	}

	// provided context is wrapped to provide graceful shutdown
	// we wrap this zone in preparation for multiple zones on a given manager
	wrappedCtx, wrappedCtxCancel := context.WithCancel(ctx)

	// track out manager's context to cascade shutdown
	go func(c context.Context) {
		<-c.Done()
		wrappedCtxCancel()
	}(m.ctx)

	zone := &zone{
		// context.Context is thread-safe.  no lock required.
		// managerCtx is passed in to trigger a graceful shutdown
		managerCtx:          m.ctx,
		zoneCtx:             wrappedCtx,
		zoneCtxCancel:       wrappedCtxCancel,
		id:                  zoneID,
		chain:               &chain{},
		errorChannel:        m.errorChannel,
		errorChannelHandler: m.errorChannelHandler,
		Mutex:               sync.Mutex{},
	}

	// associate new zone with manager, returning zone for use
	(*m.zone)[zoneID] = zone
	return zone, nil
}

// Shutdown allows for cleanly shutting down all spans from the top level of a manager
// an error is available
func (m *manager) Shutdown() []error {
	m.Lock()
	defer m.Unlock()

	var errs []error
	// intentionally block on every zone
	//
	// todo: most of this workflow is async.  we may need to wait and
	// 		 gather somehow
	for _, zone := range *m.zone {
		e := zone.Close()
		if e != nil {
			errs = append(errs, e)
		}
	}
	return errs
}
