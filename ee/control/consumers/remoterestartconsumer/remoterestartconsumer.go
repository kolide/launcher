package remoterestartconsumer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/agent/types"
)

const (
	// Identifier for this consumer.
	RemoteRestartActorType = "remote_restart"

	restartDelay = 15 * time.Second
)

type RemoteRestartConsumer struct {
	knapsack      types.Knapsack
	slogger       *slog.Logger
	signalRestart chan error
	interrupt     chan struct{}
	interrupted   bool
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

func (r *RemoteRestartConsumer) Do(data io.Reader) error {
	var restartAction remoteRestartAction

	if err := json.NewDecoder(data).Decode(&restartAction); err != nil {
		return fmt.Errorf("decoding restart action: %w", err)
	}

	// The action's run ID indicates the current `runLauncher` that should be restarted.
	// If the action's run ID does not match the current run ID, we assume the restart
	// has already happened and does not need to happen again.
	if restartAction.RunID != r.knapsack.GetRunID() {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"received remote restart action for incorrect (assuming past) launcher run ID -- discarding",
			"run_id", restartAction.RunID,
		)
		return nil
	}

	// Perform the restart by signaling actor shutdown, but delay slightly to give
	// the actionqueue a chance to process all actions and store their statuses.
	go func() {
		r.slogger.Log(context.TODO(), slog.LevelInfo,
			"received remote restart action for current launcher run ID -- signaling for restart shortly",
			"run_id", restartAction.RunID,
			"restart_delay", restartDelay.String(),
		)

		time.Sleep(restartDelay)

		r.signalRestart <- NewRemoteRestartRequestedErr()
	}()

	return nil
}

func (r *RemoteRestartConsumer) Execute() (err error) {
	// Wait until we receive a remote restart action, or until we receive a Shutdown request
	select {
	case <-r.interrupt:
		return nil
	case signalRestartErr := <-r.signalRestart:
		return signalRestartErr
	}
}

func (r *RemoteRestartConsumer) Shutdown(_ error) {
	// Only perform shutdown tasks on first call to interrupt -- no need to repeat on potential extra calls.
	if r.interrupted {
		return
	}
	r.interrupted = true

	r.interrupt <- struct{}{}
}
