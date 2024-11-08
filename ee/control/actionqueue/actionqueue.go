package actionqueue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/kolide/launcher/ee/agent/storage/inmemory"
	"github.com/kolide/launcher/ee/agent/types"
)

const (
	ActionsSubsystem       = "actions"
	defaultCleanupInterval = time.Hour * 12
	// about 6 months, long enough to ensure that K2 no longer has the message
	// and will not send a duplicate
	actionRetentionPeriod = time.Hour * 24 * 30 * 6
)

type actor interface {
	Do(data io.Reader) error
}

type action struct {
	ID          string    `json:"id"`
	ValidUntil  int64     `json:"valid_until"` // timestamp
	Type        string    `json:"type"`
	ProcessedAt time.Time `json:"processed_at,omitempty"`
}

func (a action) String() string {
	return fmt.Sprintf("ID: %s; type: %s; valid until: %d", a.ID, a.Type, a.ValidUntil)
}

type ActionQueue struct {
	ctx                   context.Context // nolint:containedctx
	actors                map[string]actor
	store                 types.KVStore
	oldNotificationsStore types.KVStore
	slogger               *slog.Logger
	actionCleanupInterval time.Duration
	cancel                context.CancelFunc
}

type actionqueueOption func(*ActionQueue)

func WithStore(store types.KVStore) actionqueueOption {
	return func(aq *ActionQueue) {
		aq.store = store
	}
}

func WithOldNotificationsStore(store types.KVStore) actionqueueOption {
	return func(aq *ActionQueue) {
		aq.oldNotificationsStore = store
	}
}

func WithCleanupInterval(cleanupInterval time.Duration) actionqueueOption {
	return func(aq *ActionQueue) {
		aq.actionCleanupInterval = cleanupInterval
	}
}

func WithContext(ctx context.Context) actionqueueOption {
	return func(aq *ActionQueue) {
		aq.ctx = ctx
	}
}

func New(k types.Knapsack, opts ...actionqueueOption) *ActionQueue {
	aq := &ActionQueue{
		ctx:                   context.Background(),
		actors:                make(map[string]actor, 0),
		actionCleanupInterval: defaultCleanupInterval,
		slogger:               k.Slogger().With("component", "actionqueue"),
	}

	for _, opt := range opts {
		opt(aq)
	}

	if aq.store == nil {
		aq.store = inmemory.NewStore()
	}

	return aq
}

func (aq *ActionQueue) Update(data io.Reader) error {
	// We want to unmarshal each action separately, so that we don't fail to send all actions
	// if only some are malformed.
	var rawActionsToProcess []json.RawMessage
	if err := json.NewDecoder(data).Decode(&rawActionsToProcess); err != nil {
		return fmt.Errorf("failed to decode actions data: %w", err)
	}

	var processError error = nil

	for _, rawAction := range rawActionsToProcess {
		var action action
		if err := json.Unmarshal(rawAction, &action); err != nil {
			aq.slogger.Log(context.TODO(), slog.LevelWarn,
				"received action in unexpected format from K2, discarding",
				"err", err,
			)
			continue
		}

		if !aq.isActionValid(action) || !aq.isActionNew(action.ID) {
			continue
		}

		actor, err := aq.actorForAction(action)
		if err != nil {
			aq.slogger.Log(context.TODO(), slog.LevelInfo,
				"getting actor for action",
				"err", err,
			)
			continue
		}

		if err := actor.Do(bytes.NewReader(rawAction)); err != nil {
			aq.slogger.Log(context.TODO(), slog.LevelInfo,
				"failed to do action with action, not marking action complete",
				"err", err,
			)
			processError = fmt.Errorf("actor.Do, action type: %s, failed: %w", action.Type, err)
			continue
		}

		// only mark processed when actor was successful
		action.ProcessedAt = time.Now().UTC()
		aq.storeActionRecord(action)
	}

	return processError
}

func (aq *ActionQueue) RegisterActor(actorType string, actorToRegister actor) {
	aq.actors[actorType] = actorToRegister
}

func (aq *ActionQueue) StartCleanup() error {
	aq.runCleanup()
	return nil
}

func (aq *ActionQueue) runCleanup() {
	ctx, cancel := context.WithCancel(aq.ctx)
	aq.cancel = cancel

	t := time.NewTicker(aq.actionCleanupInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			aq.slogger.Log(context.TODO(), slog.LevelDebug,
				"action cleanup stopped due to context cancel",
			)
			return
		case <-t.C:
			aq.cleanupActions()
		}
	}
}

func (aq *ActionQueue) StopCleanup(err error) {
	aq.cancel()
}

func (aq *ActionQueue) storeActionRecord(actionToStore action) {
	rawAction, err := json.Marshal(actionToStore)
	if err != nil {
		aq.slogger.Log(context.TODO(), slog.LevelError,
			"could not marshal complete action",
			"err", err,
		)
		return
	}

	if err := aq.store.Set([]byte(actionToStore.ID), rawAction); err != nil {
		aq.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not mark action complete",
			"err", err,
		)
	}
}

func (aq *ActionQueue) isActionNew(id string) bool {
	completedActionRaw, err := aq.store.Get([]byte(id))
	if err != nil {
		aq.slogger.Log(context.TODO(), slog.LevelError,
			"could not read action from bucket",
			"err", err,
		)
		return false
	}

	// found previous record, action not new
	if completedActionRaw != nil {
		return false
	}

	// the first "actions" were actually notifications
	// so lets make sure we are not actually getting an
	// old notification

	// 6 months or so after 2023_09_07 we should be able to remove
	// the logic around the "oldNotificationsStore"
	// since we will be sure everything has been removed from k2
	// ~ James Pickett

	// no where else to look, action is new
	if aq.oldNotificationsStore == nil {
		return true
	}

	completedActionRaw, err = aq.oldNotificationsStore.Get([]byte(id))
	if err != nil {
		aq.slogger.Log(context.TODO(), slog.LevelError,
			"could not read action from old notifications store",
			"err", err,
		)
		return false
	}

	// if nil, it's new so return true
	return completedActionRaw == nil
}

func (aq *ActionQueue) isActionValid(a action) bool {
	if a.ID == "" {
		aq.slogger.Log(context.TODO(), slog.LevelWarn,
			"action ID is empty",
			"action", a.String(),
		)
		return false
	}

	if a.ValidUntil <= 0 {
		aq.slogger.Log(context.TODO(), slog.LevelWarn,
			"action valid until is empty",
			"action", a.String(),
		)
		return false
	}

	return a.ValidUntil > time.Now().Unix()
}

func (aq *ActionQueue) actorForAction(a action) (actor, error) {
	if len(aq.actors) == 0 {
		return nil, errors.New("no actor registered")
	}

	if a.Type == "" {
		return nil, errors.New("action does not have type, cannot determine actor")
	}

	actor, ok := aq.actors[a.Type]
	if !ok {
		return nil, fmt.Errorf("actor type %s not found", a.Type)
	}

	return actor, nil
}

func (aq *ActionQueue) cleanupActions() {
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
		aq.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not iterate over bucket items to determine which are expired",
			"err", err,
		)
	}

	// Delete all old keys
	if err := aq.store.Delete(keysToDelete...); err != nil {
		aq.slogger.Log(context.TODO(), slog.LevelWarn,
			"could not delete old actions from bucket",
			"err", err,
		)
	}
}
