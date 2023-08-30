package rungroup

import (
	"fmt"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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
	err := <-errors
	level.Debug(g.logger).Log("msg", "received interrupt error from first actor -- shutting down other actors", "err", err)

	// Signal all actors to stop.
	for _, a := range g.actors {
		level.Debug(g.logger).Log("msg", "interrupting actor", "actor", a.name)
		a.interrupt(err.err)
	}

	// Wait for all actors to stop.
	for i := 1; i < cap(errors); i++ {
		e := <-errors
		level.Debug(g.logger).Log("msg", "successfully interrupted actor", "actor", e.errorSourceName, "index", i)
	}

	// Return the original error.
	return err.err
}

func (a actorError) String() string {
	return fmt.Sprintf("%s returned error: %+v", a.errorSourceName, a.err)
}
