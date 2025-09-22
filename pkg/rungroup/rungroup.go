package rungroup

// rungroup expands on oklog/run, adding logs to indicate which actor caused
// the interrupt and which actor, if any, is preventing shutdown. In the
// future, we would like to add the ability to force shutdown before a given
// timeout. See: https://github.com/kolide/launcher/issues/1205

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/gowrapper"
	"github.com/kolide/launcher/pkg/log/multislogger"
	"golang.org/x/sync/semaphore"
)

type (
	Group struct {
		slogger *slog.Logger
		actors  []rungroupActor
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
	InterruptTimeout     = 10 * time.Second // How long for all actors to return from their `interrupt` function
	executeReturnTimeout = 5 * time.Second  // After interrupted, how long for all actors to exit their `execute` functions
)

func NewRunGroup() *Group {
	return &Group{
		slogger: multislogger.NewNopLogger(),
		actors:  make([]rungroupActor, 0),
	}
}

func (g *Group) Add(name string, execute func() error, interrupt func(error)) {
	g.actors = append(g.actors, rungroupActor{name, execute, interrupt})
}

func (g *Group) SetSlogger(slogger *slog.Logger) {
	g.slogger = slogger.With("component", "run_group")
}

func (g *Group) Run() error {
	if len(g.actors) == 0 {
		return nil
	}

	// Run each actor.
	g.slogger.Log(context.TODO(), slog.LevelDebug,
		"starting all actors",
		"actor_count", len(g.actors),
	)

	actorErrors := make(chan actorError, len(g.actors))
	for _, a := range g.actors {
		a := a
		gowrapper.GoWithRecoveryAction(context.TODO(), g.slogger, func() {
			g.slogger.Log(context.TODO(), slog.LevelDebug,
				"starting actor",
				"actor", a.name,
			)
			err := a.execute()
			actorErrors <- actorError{
				errorSourceName: a.name,
				err:             err,
			}
		}, func(r any) {
			g.slogger.Log(context.TODO(), slog.LevelInfo,
				"shutting down after actor panic",
				"actor", a.name,
			)

			// Since execute panicked, the actor error won't get sent to our channel below --
			// add it now.
			actorErrors <- actorError{
				errorSourceName: a.name,
				err:             fmt.Errorf("executing rungroup actor %s panicked: %+v", a.name, r),
			}
		})
	}

	// Wait for the first actor to stop.
	initialActorErr := <-actorErrors

	g.slogger.Log(context.TODO(), slog.LevelInfo,
		"received interrupt error from first actor -- shutting down other actors",
		"err", initialActorErr.err,
		"error_source", initialActorErr.errorSourceName,
	)

	defer g.slogger.Log(context.TODO(), slog.LevelDebug,
		"done shutting down actors",
		"actor_count", len(g.actors),
		"initial_err", initialActorErr,
	)

	// Signal all actors to stop.
	numActors := int64(len(g.actors))
	interruptWait := semaphore.NewWeighted(numActors)
	for _, a := range g.actors {
		interruptWait.Acquire(context.Background(), 1)
		gowrapper.Go(context.TODO(), g.slogger, func() {
			defer interruptWait.Release(1)
			g.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupting actor",
				"actor", a.name,
			)
			a.interrupt(initialActorErr.err)
			g.slogger.Log(context.TODO(), slog.LevelDebug,
				"interrupt complete",
				"actor", a.name,
			)
		})
	}

	interruptCtx, interruptCancel := context.WithTimeout(context.Background(), InterruptTimeout)
	defer interruptCancel()

	// Wait for interrupts to complete, but only until we hit our interruptCtx timeout
	if err := interruptWait.Acquire(interruptCtx, numActors); err != nil {
		g.slogger.Log(context.TODO(), slog.LevelDebug,
			"timeout waiting for interrupts to complete, proceeding with shutdown",
			"err", err,
		)
	}

	// Wait for all other actors to stop, but only until we hit our executeReturnTimeout
	timeoutTimer := time.NewTimer(executeReturnTimeout)
	defer timeoutTimer.Stop()
	for i := 1; i < cap(actorErrors); i++ {
		select {
		case <-timeoutTimer.C:
			g.slogger.Log(context.TODO(), slog.LevelDebug,
				"rungroup shutdown deadline exceeded, not waiting for any more actors to return",
			)

			// Return the original error so we can proceed with shutdown
			return initialActorErr.err
		case e := <-actorErrors:
			g.slogger.Log(context.TODO(), slog.LevelDebug,
				"received error from actor",
				"actor", e.errorSourceName,
				"err", e.err,
				"index", i,
			)
		}
	}

	// Return the original error.
	return initialActorErr.err
}

func (a actorError) String() string {
	return fmt.Sprintf("%s returned error: %+v", a.errorSourceName, a.err)
}
