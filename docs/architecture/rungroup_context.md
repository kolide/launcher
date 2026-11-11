# Context-based Rungroup

## Background
Launcher uses [rungroup](https://github.com/kolide/launcher/blob/85b325963fbb9032121d4cdabc6207d635241643/pkg/rungroup/rungroup.go#L3-L6)
for top-level execution of [the primary launcher process](https://github.com/kolide/launcher/blob/85b325963fbb9032121d4cdabc6207d635241643/cmd/launcher/launcher.go#L281-L282)
and its [child desktop processes](https://github.com/kolide/launcher/blob/85b325963fbb9032121d4cdabc6207d635241643/cmd/launcher/desktop.go#L145).
Actors are registered with a rungroup by name, passing a long-lived execution function and a short-lived interrupt
function to stop it.
Primary features include:
  - manual batched start and stop of all actors
  - any actor exiting results in termination of all actors (errgroup-like)
  - interruption timeout: 10 second wait for actors interrupt calls to end
  - drain timeout: up to 5 more seconds for actors post-interruption to return
  - panic tolerance throughout
  - logging of exit attribution, interruption results, and general lifecycle

The key API at hand here is to register actors:
```golang
func (g *Group) Add(name string, execute func() error, interrupt func(error))
```

When a rungroup exits, `Add` inherently requires that actors communicate from the goroutine running their `interrupt`
to stop the long-lived goroutine running `execute`. A common pattern for a minimal and safe actor implementation can be simplified to:
```golang
type sample struct {
    interrupted atomic.Bool
    interrupt   chan struct{} // buffer of 1
}

func (s *sample) Execute() error {
    if s.interrupted.Load() {
        return
    }

    for {
        // [...] do work [...]
        select {
            case <-someTicker.C:
            case <-s.interrupt:
                return nil
        }
    }

    return nil
}

func (s *sample) Interrupt(_ error) {
    if s.interrupted.Swap(true) {
        return
    }

    s.interrupt <- struct{}{}
}
```

The development to arrive at this near-standard seems long, with [#1205](https://github.com/kolide/launcher/issues/1205) tracking one line of discovery. Nevertheless, a few bugs remain in bespoke actor implementations.

rungroup actors generally have issues in that:
  - actors whose `execute` end must still accept interrupts, or rungroup shutdown times out waiting
  - `interrupt` doesn't interrupt per se: even a robust `execute` ignores it during its "work"
  - timing challenges: rungroups call `execute` before `interrupt`, but on a fast rungroup shutdown `execute` may be very early on when `interrupt` signals

## Proposed Changes
Why: make using run group correctly a dead simple default. It's too hard to hold rungroup's API properly, and even when we do there's more effort to handle work interruption and avoid races. For recent examples of pain resolved: [#2783](https://github.com/kolide/launcher/pull/2783), [#2772](https://github.com/kolide/launcher/pull/2772), [#2803](https://github.com/kolide/launcher/pull/2803/changes).


Context-aware actors provide an idiomatic way to signal actors to stop work. The above example is greatly simplified with a context and potentially solves all three issues,
depending on the work:
```golang
func sampleExecute(ctx context.Context) error {
    for {
        // [...] do work and pass in ctx [...]
        select {
            case <-someTicker.C:
            case <-ctx.Done():
                return nil
        }
    }

    return nil
}
```

Addition of an `AddContext` API is unsurprising:
```golang
// Add accepts the legacy rungroup actor interface, where execute ran until a separate interrupt
// told it to stop.
//
// Deprecated: use AddContext and honor the context in the execute function.
func (g *Group) Add(name string, execute func() error, interrupt func(error))

// AddContext registers an actor by a human-readable name and the function to run it. The context is
// done when the actor should stop. If the actor returns before the passed ctx ends, the rungroup will
// shut down.
func (g *Group) AddContext(name string, execute func(ctx context.Context) error)
```

`Add` can be written as a specialized and deprecated `AddContext`:
```golang
func (g *Group) Add(name string, execute func() error, interrupt func(error)) {
    g.AddContext(name, func(ctx context.Context) error {
        gowrapper.Go(ctx, g.slogger, func() {
            <-ctx.Done() // wait to end, like we expect modern actors to
            g.slogger.Log(ctx, slog.LevelDebug,
                "interrupting legacy actor",
                "actor", name,
            )
            interrupt(ctx.Err())
            g.slogger.Log(ctx, slog.LevelDebug,
                "interrupt complete",
                "actor", name,
            )
        })
        return execute()
    })
}
```

The deprecation allows simplification of rungroup.Run. Without first class interrupts, context cancellation is the sole signaling
mechanism and it's shared by all its actors. The bulk of rungroup becomes logging, error reporting, and handling draining timeouts.
Timeouts become a shared 15 second drain period, which is a functional change since interrupts used to get their own window.

## Evaluation
This repo is a bit short of thirty production calls to `rungroup.Group.Add`. The evaluation below serves as a check that the proposed
design does not paint us into a corner. Anything that is a clear fit or already context-based is excluded.

### Listener-based Actors
Painful. Listener-based actors include:
  - [ee/desktop/user/server's UserServer.Serve()](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/ee/desktop/user/server/server.go#L110-L112)
  - [ee/localserver's localServer.Start()](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/ee/localserver/server.go#L255)
  - [ee/agent/listener's listener.Execute()](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/ee/agent/listener/listener.go#L96)

A few of these drain before shutdown, but all of them have a primary loop that waits on a `net.Listen.Accept()`.
Accept does not honor a context outside of dialing.

Each needs modification to use the provided context to trigger a listener.Close(). Afterward, a separate context
can implement any specific draining logic.

### logShipper
[logShipper.Run](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/pkg/log/logshipper/logshipper.go#L130) is an odd duck that likely belongs outside of runGroup. That way, it can be one of the final things to exit and
flush logs.

It already operates on a context. What changes is that now when context is cancelled, it will need to manually create
another to drain.

### ControlService and ActionsQueue
Separate projects to do right. Both [ControlService](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/cmd/launcher/launcher.go#L468) and [ActionsQueue](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/cmd/launcher/launcher.go#L522-L528) already use contexts to signal themselves to stop.

ControlService behaves well, shutting down its main loop, **except** when actively fetching and notifying. Interrupting a fetch is easily fixed, but consumer notification behavior [requires updating an interface](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/ee/control/control.go#L434-L437) since it is not context aware.

ActionsQueue itself largely an edge-triggered consumer from ControlService, and its `Update` implementation is context-unaware due to the interface. It further goes on to update more actors which have any actions queued.

In summary, these interfaces should be context-aware. That is a separate evaluation and not in scope.

ControlService's [consumer](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/ee/control/control.go#L40-L44):
```go
// consumer is an interface for something that consumes control server data updates. The
// control server supports at most one consumer per subsystem.
type consumer interface {
    Update(data io.Reader) error
}
```

ControlService's [subscriber](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/ee/control/control.go#L46-L50) is similar, though I'd recommend deprecation on of this pattern. Instead, let [subscribers subscribe](https://github.com/kolide/launcher/compare/main...brhoades:launcher:billy/deprecate-controlserver-ping). Today:
```golang
// subscriber is an interface for something that wants to be notified when a subsystem has been updated.
// Subscribers do not receive data -- they are expected to read the data from where consumers write it.
type subscriber interface {
    Ping()
}
```

ActionsQueue's [actor](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/ee/control/actionqueue/actionqueue.go#L28-L30) too, though I'm partial to inverting
this interface to be a datastore-based API:
```golang
type actor interface {
	Do(data io.Reader) error
}
```

We are no worse off today by converting ControlService and ActionsQueue's rungroup actors to use contexts-- they already do in their own way-- however we cannot go the full mile without further effort.

### osqueryRunner's Actor
[pkg/osquery/runtime's Runner](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/pkg/osquery/runtime/runner.go#L67) and its children [osqueryinstance](https://github.com/kolide/launcher/blob/b663550d2b6ae0a969776d457c062ce97c5a3740/pkg/osquery/runtime/runner.go#L107)s

The runner abstraction had a sweeping draft PR out, fixing [a few holes where an instance can slip through](https://github.com/kolide/launcher/pull/2813) with a context. A lesser version of that PR which can be used to plumb context through and remove some tricky shutdown backflips.

Unlike most other actors, Runner still needs to allow outside shutdown and restarts independent of its context.
