package remoterestartconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
	"github.com/kolide/launcher/ee/gowrapper"
)

const (
	// RemoteRestartActorType identifies this action/actor type, which performs
	// a launcher restart when requested by the control server. This actor type
	// belongs to the action subsystem.
	RemoteRestartActorType = "remote_restart"

	// restartDelay is the delay after receiving action before triggering the restart.
	// We have a delay to allow the actionqueue.
	restartDelay = 15 * time.Second
)

var (
	ErrRemoteRestartRequested = errors.New("need to reload launcher: remote restart requested")
)

type RemoteRestartConsumer struct {
	knapsack      types.Knapsack
	slogger       *slog.Logger
	signalRestart chan error
	interrupt     chan struct{}
	interrupted   atomic.Bool
}

type remoteRestartAction struct {
	RunID string `json:"run_id"` // the run ID for the launcher run to restart
}

func New(knapsack types.Knapsack) *RemoteRestartConsumer {
	return &RemoteRestartConsumer{
		knapsack:      knapsack,
		slogger:       knapsack.Slogger().With("component", "remote_restart_consumer"),
		signalRestart: make(chan error, 1),
		interrupt:     make(chan struct{}, 1),
	}
}

// Do implements the `actionqueue.actor` interface, and allows the actionqueue
// to pass `remote_restart` type actions to this consumer. The actionqueue validates
// that this action has not already been performed and that this action is still
// valid (i.e. not expired). `Do` additionally validates that the `run_id` given in
// the action matches the current launcher run ID.
func (r *RemoteRestartConsumer) Do(data io.Reader) error {
	var restartAction remoteRestartAction

	if err := json.NewDecoder(data).Decode(&restartAction); err != nil {
		return fmt.Errorf("decoding restart action: %w", err)
	}

	// The action's run ID indicates the current `runLauncher` that should be restarted.
	// If the action's run ID does not match the current run ID, we assume the restart
	// has already happened and does not need to happen again.
	if restartAction.RunID == "" {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"received remote restart action with blank launcher run ID -- discarding",
		)
		return nil
	}
	if restartAction.RunID != r.knapsack.GetRunID() {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"received remote restart action for incorrect (assuming past) launcher run ID -- discarding",
			"action_run_id", restartAction.RunID,
		)
		return nil
	}

	// Perform the restart by signaling actor shutdown, but delay slightly to give
	// the actionqueue a chance to process all actions and store their statuses.
	gowrapper.Go(context.TODO(), r.slogger, func() {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"received remote restart action for current launcher run ID -- signaling for restart shortly",
			"action_run_id", restartAction.RunID,
			"restart_delay", restartDelay.String(),
		)

		select {
		case <-r.interrupt:
			r.slogger.Log(context.TODO(), slog.LevelDebug,
				"received external interrupt before remote restart could be performed",
			)
			return
		case <-time.After(restartDelay):
			r.signalRestart <- ErrRemoteRestartRequested
			r.slogger.Log(context.TODO(), slog.LevelInfo,
				"signaled for restart after delay",
				"action_run_id", restartAction.RunID,
			)
			return
		}
	})

	return nil
}

// Execute allows the remote restart consumer to run in the main launcher rungroup.
// It waits until it receives a remote restart action from `Do`, or until it receives
// a `Interrupt` request.
func (r *RemoteRestartConsumer) Execute() (err error) {
	select {
	case <-r.interrupt:
		return nil
	case signalRestartErr := <-r.signalRestart:
		return signalRestartErr
	}
}

// Interrupt allows the remote restart consumer to run in the main launcher rungroup
// and be shut down when the rungroup shuts down.
func (r *RemoteRestartConsumer) Interrupt(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if r.interrupted.Swap(true) {
		return
	}

	r.interrupt <- struct{}{}
}
