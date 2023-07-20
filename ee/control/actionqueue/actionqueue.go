package actionqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/kolide/launcher/pkg/agent/storage/inmemory"
	"github.com/kolide/launcher/pkg/agent/types"
)

const (
	ActionsSubsystem       = "actions"
	defaultCleanupInterval = time.Hour * 12
	// about 6 months, long enough to ensure that K2 no longer has the message
	// and will not send a duplicate
	actionRetentionPeriod = time.Hour * 24 * 30 * 6
)

type updater interface {
	Update(data io.Reader) error
}

type action struct {
	ID          string    `json:"id"`
	ValidUntil  int64     `json:"valid_until"` // timestamp
	Type        string    `json:"type"`
	ProcessedAt time.Time `json:"processed_at,omitempty"`
}

type actionqueue struct {
	ctx                   context.Context
	updaters              map[string]updater
	store                 types.KVStore
	logger                log.Logger
	actionCleanupInterval time.Duration
	cancel                context.CancelFunc
}

type actionqueueOption func(*actionqueue)

func WithLogger(logger log.Logger) actionqueueOption {
	return func(aq *actionqueue) {
		aq.logger = logger
	}
}

func WithStore(store types.KVStore) actionqueueOption {
	return func(aq *actionqueue) {
		aq.store = store
	}
}

func WithCleanupInterval(cleanupInterval time.Duration) actionqueueOption {
	return func(aq *actionqueue) {
		aq.actionCleanupInterval = cleanupInterval
	}
}

func WithContext(ctx context.Context) actionqueueOption {
	return func(aq *actionqueue) {
		aq.ctx = ctx
	}
}

func New(opts ...actionqueueOption) *actionqueue {
	aq := &actionqueue{
		ctx:                   context.Background(),
		updaters:              make(map[string]updater, 0),
		actionCleanupInterval: defaultCleanupInterval,
		logger:                log.NewNopLogger(),
	}

	for _, opt := range opts {
		opt(aq)
	}

	if aq.store == nil {
		aq.store = inmemory.NewStore(aq.logger)
	}

	return aq
}

func (aq *actionqueue) Update(data io.Reader) error {
	// We want to unmarshal each action separately, so that we don't fail to send all actions
	// if only some are malformed.
	var rawActionsToProcess []json.RawMessage
	if err := json.NewDecoder(data).Decode(&rawActionsToProcess); err != nil {
		return fmt.Errorf("failed to decode actions data: %w", err)
	}

	for _, rawAction := range rawActionsToProcess {
		var action action
		if err := json.Unmarshal(rawAction, &action); err != nil {
			level.Debug(aq.logger).Log("msg", "received action in unexpected format from K2, discarding", "err", err)
			continue
		}

		if !aq.isActionValid(action) || !aq.isActionNew(action.ID) {
			continue
		}

		updater, err := aq.updaterForAction(action)
		if err != nil {
			level.Info(aq.logger).Log("msg", "getting updater for action", "error", err)
			continue
		}

		if err := updater.Update(bytes.NewReader(rawAction)); err != nil {
			level.Info(aq.logger).Log("msg", "failed to update with action, not marking action complete", "err", err)
			continue
		}

		// only mark processed when updater was successful
		action.ProcessedAt = time.Now().UTC()
		aq.storeActionRecord(action)
	}

	return nil
}

func (aq *actionqueue) RegisterUpdater(updaterType string, updater updater) {
	aq.updaters[updaterType] = updater
}

func (aq *actionqueue) StartCleanup() error {
	aq.runCleanup()
	return nil
}

func (aq *actionqueue) runCleanup() {
	ctx, cancel := context.WithCancel(aq.ctx)
	aq.cancel = cancel

	t := time.NewTicker(aq.actionCleanupInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			level.Debug(aq.logger).Log("msg", "action cleanup stopped due to context cancel")
			return
		case <-t.C:
			aq.cleanupActions()
		}
	}
}

func (aq *actionqueue) StopCleanup(err error) {
	aq.cancel()
}

func (aq *actionqueue) storeActionRecord(action action) {
	rawAction, err := json.Marshal(action)
	if err != nil {
		level.Error(aq.logger).Log("msg", "could not marshal complete action", "err", err)
		return
	}

	if err := aq.store.Set([]byte(action.ID), rawAction); err != nil {
		level.Debug(aq.logger).Log("msg", "could not mark notification sent", "err", err)
	}
}

func (aq *actionqueue) isActionNew(id string) bool {
	sentNotificationRaw, err := aq.store.Get([]byte(id))
	if err != nil {
		level.Error(aq.logger).Log("msg", "could not read action from bucket", "err", err)
	}

	if sentNotificationRaw == nil {
		// No previous record -- action has not been processed before, it's new
		return true
	}

	return false
}

func (aq *actionqueue) isActionValid(a action) bool {
	if a.ID == "" {
		level.Info(aq.logger).Log("msg", "action ID is empty", "action", a)
		return false
	}

	if a.ValidUntil <= 0 {
		level.Info(aq.logger).Log("msg", "action valid until is empty", "action", a)
		return false
	}

	return a.ValidUntil > time.Now().Unix()
}

func (aq *actionqueue) updaterForAction(a action) (updater, error) {
	if len(aq.updaters) == 0 {
		return nil, errors.New("no updaters registered")
	}

	// more than one updater
	if a.Type == "" {
		return nil, errors.New("have more than 1 updater and action type is empty, cannot determine updater")
	}

	updater, ok := aq.updaters[a.Type]
	if !ok {
		return nil, fmt.Errorf("updater type %s not found", a.Type)
	}

	return updater, nil
}

func (aq *actionqueue) cleanupActions() {
	// Read through all keys in bucket to determine which ones are old enough to be deleted
	keysToDelete := make([][]byte, 0)

	if err := aq.store.ForEach(func(k, v []byte) error {
		var processedAction action
		if err := json.Unmarshal(v, &processedAction); err != nil {
			return fmt.Errorf("error processing %s: %w", string(k), err)
		}

		if processedAction.ProcessedAt.Add(actionRetentionPeriod).Before(time.Now().UTC()) {
			keysToDelete = append(keysToDelete, k)
		}

		return nil
	}); err != nil {
		level.Debug(aq.logger).Log("msg", "could not iterate over bucket items to determine which are expired", "err", err)
	}

	// Delete all old keys
	if err := aq.store.Delete(keysToDelete...); err != nil {
		level.Debug(aq.logger).Log("msg", "could not delete old notifications from bucket", "err", err)
	}
}
