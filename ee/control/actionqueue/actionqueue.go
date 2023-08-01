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

type actioner interface {
	DoAction(data io.Reader) error
}

type action struct {
	ID          string    `json:"id"`
	ValidUntil  int64     `json:"valid_until"` // timestamp
	Type        string    `json:"type"`
	ProcessedAt time.Time `json:"processed_at,omitempty"`
}

type actionqueue struct {
	ctx                   context.Context
	actioners             map[string]actioner
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
		actioners:             make(map[string]actioner, 0),
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

		actioner, err := aq.actionerForAction(action)
		if err != nil {
			level.Info(aq.logger).Log("msg", "getting actioner for action", "error", err)
			continue
		}

		if err := actioner.DoAction(bytes.NewReader(rawAction)); err != nil {
			level.Info(aq.logger).Log("msg", "failed to do action with action, not marking action complete", "err", err)
			continue
		}

		// only mark processed when actioner was successful
		action.ProcessedAt = time.Now().UTC()
		aq.storeActionRecord(action)
	}

	return nil
}

func (aq *actionqueue) RegisterActioner(actionerType string, actionToRegister actioner) {
	aq.actioners[actionerType] = actionToRegister
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

func (aq *actionqueue) storeActionRecord(actionToStore action) {
	rawAction, err := json.Marshal(actionToStore)
	if err != nil {
		level.Error(aq.logger).Log("msg", "could not marshal complete action", "err", err)
		return
	}

	if err := aq.store.Set([]byte(actionToStore.ID), rawAction); err != nil {
		level.Debug(aq.logger).Log("msg", "could not mark action complete", "err", err)
	}
}

func (aq *actionqueue) isActionNew(id string) bool {
	completedActionRaw, err := aq.store.Get([]byte(id))
	if err != nil {
		level.Error(aq.logger).Log("msg", "could not read action from bucket", "err", err)
		return false
	}

	if completedActionRaw == nil {
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

func (aq *actionqueue) actionerForAction(a action) (actioner, error) {
	if len(aq.actioners) == 0 {
		return nil, errors.New("no actioners registered")
	}

	// more than one actioner
	if a.Type == "" {
		return nil, errors.New("have more than 1 actioner and action type is empty, cannot determine actioner")
	}

	actioner, ok := aq.actioners[a.Type]
	if !ok {
		return nil, fmt.Errorf("actioner type %s not found", a.Type)
	}

	return actioner, nil
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
		level.Debug(aq.logger).Log("msg", "could not delete old actions from bucket", "err", err)
	}
}
