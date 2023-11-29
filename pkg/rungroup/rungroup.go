package rungroup

// rungroup expands on oklog/run, adding logs to indicate which actor caused
// the interrupt and which actor, if any, is preventing shutdown. In the
// future, we would like to add the ability to force shutdown before a given
// timeout. See: https://github.com/kolide/launcher/issues/1205

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"golang.org/x/sync/semaphore"
)

type (
	Group struct {
		logger log.Logger
		actors []rungroupActor
	}

	rungroupActor struct {
		name      string // human-readable identifier for the actor
		execute   func() error
		interrupt func(error)
	}

	actorError struct {
		errorSourceName string
		err             error
	}
)

const (
	interruptTimeout     = 5 * time.Second // How long for all actors to return from their `interrupt` function
	executeReturnTimeout = 5 * time.Second // After interrupted, how long for all actors to exit their `execute` functions
)

func NewRunGroup(logger log.Logger) *Group {
	return &Group{
		logger: log.With(logger, "component", "run_group"),
		actors: make([]rungroupActor, 0),
	}
}

func (g *Group) Add(name string, execute func() error, interrupt func(error)) {
	g.actors = append(g.actors, rungroupActor{name, execute, interrupt})
}

func (g *Group) Run() error {
	if len(g.actors) == 0 {
		return nil
	}

	// Run each actor.
	level.Debug(g.logger).Log("msg", "starting all actors", "actor_count", len(g.actors))
	errors := make(chan actorError, len(g.actors))
	for _, a := range g.actors {
		go func(a rungroupActor) {
			level.Debug(g.logger).Log("msg", "starting actor", "actor", a.name)
			err := a.execute()
			errors <- actorError{
				errorSourceName: a.name,
				err:             err,
			}
		}(a)
	}

	// Wait for the first actor to stop.
	initialActorErr := <-errors
	level.Debug(g.logger).Log("msg", "received interrupt error from first actor -- shutting down other actors", "err", initialActorErr)

	// Signal all actors to stop.
	numActors := int64(len(g.actors))
	interruptWait := semaphore.NewWeighted(numActors)
	for _, a := range g.actors {
		interruptWait.Acquire(context.Background(), 1)
		go func(a rungroupActor) {
			defer interruptWait.Release(1)
			level.Debug(g.logger).Log("msg", "interrupting actor", "actor", a.name)
			a.interrupt(initialActorErr.err)
			level.Debug(g.logger).Log("msg", "interrupt complete", "actor", a.name)
		}(a)
	}

	interruptCtx, interruptCancel := context.WithTimeout(context.Background(), interruptTimeout)
	defer interruptCancel()

	// Wait for interrupts to complete, but only until we hit our interruptCtx timeout
	if err := interruptWait.Acquire(interruptCtx, numActors); err != nil {
		level.Debug(g.logger).Log("msg", "timeout waiting for interrupts to complete, proceeding with shutdown", "err", err)
	}

	// Wait for all other actors to stop, but only until we hit our executeReturnTimeout
	timeoutTimer := time.NewTimer(executeReturnTimeout)
	defer timeoutTimer.Stop()
	for i := 1; i < cap(errors); i++ {
		select {
		case <-timeoutTimer.C:
			level.Debug(g.logger).Log("msg", "rungroup shutdown deadline exceeded, not waiting for any more actors to return")

			// Return the original error so we can proceed with shutdown
			return initialActorErr.err
		case e := <-errors:
			level.Debug(g.logger).Log("msg", "execute returned", "actor", e.errorSourceName, "index", i)
		}
	}

	// Return the original error.
	return initialActorErr.err
}

func (a actorError) String() string {
	return fmt.Sprintf("%s returned error: %+v", a.errorSourceName, a.err)
}
